#!/usr/bin/env node
/**
 * A pull request's own CI honours the test quarantine.
 *
 * ADR-xf4p9fzd made a red job pass when every test it failed is held by an open
 * bead, and ADR-0bawnmfg is this side of it: the same policy, asked of the job
 * that is running rather than of a merged pull request's stored logs. A job that
 * failed only on quarantined tests passes and PRINTS every one of them with its
 * bead; a test no open bead holds, a bead that has been closed, and a failure
 * that names no test all keep the job red.
 *
 * HOW THE FAILURE LIST REACHES THIS CHECK. A gated step runs its command through
 * scripts/ci-run-logged.sh, which tees the step's own stdout and stderr to
 * `$NOCX_TEST_LOG_DIR/<name>.log` and writes the command's exit status to
 * `<name>.status`. That file holds exactly what
 * `GET /repos/{repo}/actions/jobs/{id}/logs` would return for the same step, so
 * scripts/quarantine.mjs's `failedTests` parses both — no second parser, no
 * token, no API call, and no log downloaded by hand.
 *
 * The STATUS decides whether a step is judged, and the log only names what it
 * failed on. Both halves are needed and neither is enough: a failure that names
 * no test (a build error, a setup step) leaves nothing in the text to find, and
 * a passing step can print a test failure's line into a pane —
 * e2e/visual-board.spec.ts:170 does exactly that — so text alone would fail a
 * green job.
 *
 * The gate runs as the job's last step (`if: always()`), so a red step has
 * already written its evidence by the time the decision is taken. A gated step
 * that never ran writes nothing and is not judged here; the job fails on it in
 * the ordinary way.
 *
 *   NOCX_TEST_LOG_DIR     the directory the gated steps wrote (required)
 *   NOCX_QUARANTINE_JOB   this job's name as GitHub prints it, e.g.
 *                         `ci-backend (no-secret-service)` — the same name the
 *                         promotion reads from the API, so one entry serves both
 *   NOCX_QUARANTINE_PULL  the pull request number; empty off a pull request,
 *                         where a `job:<name>#<PR>` entry cannot match
 *   NOCX_QUARANTINE_FILE  default docs/testing/quarantine.json
 *   NOCX_ISSUES_FILE      default .beads/issues.jsonl
 */
import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

import { failedTests, judgeFailures, readQuarantineState } from './quarantine.mjs'

const prefix = 'quarantine:'

/** The evidence one gated step left: its name, its command's status, its output. */
export function readStepLogs(dir) {
  return readdirSync(dir)
    .filter((name) => name.endsWith('.status'))
    .map((name) => name.slice(0, -'.status'.length))
    .sort()
    .map((name) => ({
      name,
      status: Number(readFileSync(join(dir, `${name}.status`), 'utf8').trim()),
      log: readFileSync(join(dir, `${name}.log`), 'utf8'),
    }))
}

/**
 * The decision, in one place and without a process: 0 when this job may pass, 1
 * when it may not. Everything it says goes through `write`, so a test reads the
 * report a job's log would carry.
 */
export function gate({
  logs,
  job = 'unknown job',
  pull,
  quarantineJson,
  issuesJsonl,
  write = (line) => process.stdout.write(`${line}\n`),
}) {
  const { open, closed } = readQuarantineState(quarantineJson, issuesJsonl)
  const steps = readStepLogs(logs)
  const jobKey = pull ? `job:${job}#${pull}` : undefined
  const failed = []
  for (const step of steps) {
    // ONLY A STEP THAT FAILED IS JUDGED, and the log only names what it failed
    // on. That is not a weakening: e2e/visual-board.spec.ts:170 runs a shell
    // fixture that prints `--- FAIL: TestSessionReconnect (0.08s)` into a pane
    // during a PASSING spec, so a log that names a failure is not evidence the
    // step failed — the status is.
    if (step.status === 0) continue
    const named = failedTests(step.log)
    failed.push({
      step,
      verdict: judgeFailures({ failures: named, quarantine: open, closed, jobKey }),
    })
  }
  if (failed.length === 0) {
    write(`${prefix} ${job}: none of its ${steps.length} recorded test step(s) failed`)
    return 0
  }
  for (const { step, verdict } of failed)
    for (const { test, bead } of verdict.quarantined)
      write(`${prefix} ${job}: ${step.name} failed ${test} — quarantined by ${bead} (open)`)
  const refused = failed.filter(({ verdict }) => !verdict.accepted)
  for (const { step, verdict } of refused) write(`${prefix} ${job}: ${step.name} ${verdict.reason}`)
  const quarantined = failed.reduce((count, { verdict }) => count + verdict.quarantined.length, 0)
  if (refused.length > 0) {
    const unquarantined = refused.reduce(
      (count, { verdict }) => count + verdict.unquarantined.length,
      0,
    )
    const unnamed = refused.filter(({ verdict }) => verdict.unquarantined.length === 0).length
    write(
      `${prefix} FAILED: ${refused.length} test step(s) of ${job} reported ${unquarantined} named failure(s) no open bead holds and ${unnamed} failure(s) that named none, beside ${quarantined} quarantined one(s). Fix them, or file each one's bead and add it to docs/testing/quarantine.json.`,
    )
    write(
      `${prefix} a failure that names no test — a build error, a setup step — is never quarantined by a test name: only a job:<name>#<PR> entry holds one, for one pull request.`,
    )
    return 1
  }
  write(
    `${prefix} passed with ${quarantined} quarantined failure(s): every failure ${job} reported is held by an open bead, listed above.`,
  )
  return 0
}

export function main(env = process.env) {
  const logs = env.NOCX_TEST_LOG_DIR
  const pull = env.NOCX_QUARANTINE_PULL?.trim()
  try {
    if (!logs) throw new Error('NOCX_TEST_LOG_DIR is not set: nothing to judge')
    return gate({
      logs,
      job: env.NOCX_QUARANTINE_JOB?.trim() || 'unknown job',
      pull: pull || undefined,
      quarantineJson: readFileSync(
        env.NOCX_QUARANTINE_FILE || 'docs/testing/quarantine.json',
        'utf8',
      ),
      issuesJsonl: readFileSync(env.NOCX_ISSUES_FILE || '.beads/issues.jsonl', 'utf8'),
    })
  } catch (error) {
    // Fail closed: a check that cannot read its own evidence must never report
    // a pass.
    process.stderr.write(`${prefix} FAILED: ${error.message}\n`)
    return 1
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  process.exit(main())
}
