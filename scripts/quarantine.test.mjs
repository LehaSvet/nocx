import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'

import { gate } from './quarantine-gate.mjs'
import { failedTests, judgeFailures, readQuarantine, readQuarantineState } from './quarantine.mjs'

// Real log lines (CI runs 40866954718 and 40893969284, 2026-10-29), with the
// timestamp prefix the jobs/logs endpoint adds and Playwright's trailing rule.
const goLog =
  '2026-10-29T08:51:12.1Z === RUN   TestZshAgentWrapperCancelsTheLaunchOnCtrlC\n' +
  '2026-10-29T08:51:23.8Z --- FAIL: TestZshAgentWrapperCancelsTheLaunchOnCtrlC (11.67s)\n' +
  '2026-10-29T08:51:23.8Z     --- FAIL: TestZshAgentWrapperCancelsTheLaunchOnCtrlC/sub (0.1s)\n' +
  '2026-10-29T08:51:24.0Z FAIL\tgithub.com/shady2k/nocx/internal/app\t300.1s\n'
const e2eLog =
  '2026-10-29T13:01:02.0Z   1) [webkit] › e2e/ask-attaches-the-frozen-screen.spec.ts:218:5 › the assistant is told the frozen screen came with the question (nocx-hp8p2.4) › summoning over a full-screen program on the alternate screen attaches its screen to the ask ─────────\n' +
  '2026-10-29T13:01:02.1Z     Error: fake-openai: timed out waiting for 2 request(s)\n'
const askKey =
  'e2e:e2e/ask-attaches-the-frozen-screen.spec.ts › the assistant is told the frozen screen came with the question (nocx-hp8p2.4) › summoning over a full-screen program on the alternate screen attaches its screen to the ask'

test('a failed job log names its failed tests in the quarantine key form, and nothing else', () => {
  assert.deepEqual(failedTests(goLog), ['go:TestZshAgentWrapperCancelsTheLaunchOnCtrlC'])
  assert.deepEqual(failedTests(e2eLog), [askKey])
  assert.deepEqual(failedTests('2026-10-29T00:00:00Z panic: test timed out after 10m0s\n'), [])
})

test('only an open bead quarantines; a closed one protects nothing; an unknown one refuses', () => {
  const issues = [
    { id: 'nocx-a', status: 'open' },
    { id: 'nocx-b', status: 'submitted' },
    { id: 'nocx-c', status: 'closed' },
  ]
    .map((row) => JSON.stringify(row))
    .join('\n')
  const doc = (tests) => JSON.stringify({ schema: 1, tests })
  assert.deepEqual(readQuarantine(undefined, issues), {})
  assert.deepEqual(
    readQuarantine(
      doc([
        { test: 'go:TestA', bead: 'nocx-a' },
        { test: askKey, bead: 'nocx-b' },
        { test: 'go:TestC', bead: 'nocx-c' },
      ]),
      issues,
    ),
    { 'go:TestA': 'nocx-a', [askKey]: 'nocx-b' },
  )
  // The closed entry is kept, not dropped: a red job must be able to say that
  // the file holds the test and its bead is closed, rather than that no bead
  // quarantines it.
  assert.deepEqual(
    readQuarantineState(doc([{ test: 'go:TestC', bead: 'nocx-c' }]), issues).closed,
    { 'go:TestC': 'nocx-c' },
  )
  assert.throws(() => readQuarantine(doc([{ test: 'go:TestA', bead: 'nocx-z' }]), issues), /exist/)
  assert.throws(() => readQuarantine(doc([{ test: 'TestA', bead: 'nocx-a' }]), issues), /entry/)
  assert.deepEqual(readQuarantine(doc([{ test: 'job:ci-backlog#557', bead: 'nocx-a' }]), issues), {
    'job:ci-backlog#557': 'nocx-a',
  })
  assert.throws(
    () => readQuarantine(doc([{ test: 'job:ci-backlog', bead: 'nocx-a' }]), issues),
    /entry/,
  )
  assert.throws(() => readQuarantine('{"schema":2,"tests":[]}', issues), /quarantine/)
})

test('the verdict holds a failure only while an open bead owns it, in both callers', () => {
  const quarantine = { 'go:TestA': 'nocx-a', 'job:ci-backend (no-secret-service)#12': 'nocx-b' }
  const held = judgeFailures({ failures: ['go:TestA'], quarantine })
  assert.deepEqual(held.quarantined, [{ test: 'go:TestA', bead: 'nocx-a' }])
  assert.equal(held.accepted, true)
  const other = judgeFailures({ failures: ['go:TestA', 'go:TestB'], quarantine })
  assert.equal(other.accepted, false)
  assert.match(other.reason, /failed go:TestB, which no open bead quarantines/)
  // A failure that names no test is never held by a test key, only by a retry
  // that classified it or by an entry for exactly this job of this PR.
  assert.equal(judgeFailures({ failures: [], quarantine }).accepted, false)
  assert.match(judgeFailures({ failures: [], quarantine }).reason, /never passed/)
  assert.equal(
    judgeFailures({ failures: [], quarantine, passedInAnotherAttempt: true }).accepted,
    true,
  )
  const jobKey = 'job:ci-backend (no-secret-service)#12'
  assert.equal(judgeFailures({ failures: [], quarantine, jobKey }).accepted, true)
  assert.equal(
    judgeFailures({ failures: [], quarantine, jobKey: 'job:ci-backend (no-secret-service)#13' })
      .accepted,
    false,
  )
})

// The gate's own fixtures: the files a gated step leaves behind.
function evidence(t, steps, { tests } = {}) {
  const root = mkdtempSync(join(tmpdir(), 'nocx-quarantine-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const logs = join(root, 'logs')
  mkdirSync(logs)
  for (const { name, status, log } of steps) {
    writeFileSync(join(logs, `${name}.log`), log)
    writeFileSync(join(logs, `${name}.status`), `${status}\n`)
  }
  const files = {
    quarantineJson: JSON.stringify({
      schema: 1,
      tests: tests ?? [
        { test: 'go:TestA', bead: 'nocx-a' },
        { test: askKey, bead: 'nocx-b' },
      ],
    }),
    issuesJsonl: [
      { id: 'nocx-a', status: 'open' },
      { id: 'nocx-b', status: 'open' },
      { id: 'nocx-c', status: 'closed' },
    ]
      .map((row) => JSON.stringify(row))
      .join('\n'),
  }
  const run = ({ job = 'ci-backend (no-secret-service)', pull = '12' } = {}) => {
    const report = []
    const code = gate({ logs, job, pull, ...files, write: (line) => report.push(line) })
    return { code, report, text: report.join('\n') }
  }
  return { run, logs }
}

const failure = (test) =>
  `--- FAIL: ${test} (1.0s)\nFAIL\tgithub.com/shady2k/nocx/internal/x\t1.0s\n`

test('a job that failed only on quarantined tests passes, and every one of them is printed with its bead', (t) => {
  const { run } = evidence(t, [
    { name: 'go-backend', status: 1, log: failure('TestA') },
    { name: 'go-backend-local-ssh', status: 0, log: 'ok  \tgithub.com/shady2k/nocx/x\t1.0s\n' },
    { name: 'e2e-chromium-1', status: 1, log: e2eLog },
  ])
  const { code, text } = run()
  assert.equal(code, 0, text)
  assert.match(text, /go-backend failed go:TestA — quarantined by nocx-a \(open\)/)
  assert.ok(text.includes(`${askKey} — quarantined by nocx-b (open)`), text)
  assert.match(text, /passed with 2 quarantined failure\(s\)/)
})

test('one failure no open bead holds keeps the job red, beside the quarantined ones', (t) => {
  const { run } = evidence(t, [
    {
      name: 'go-backend',
      status: 1,
      log: failure('TestA') + failure('TestUnowned'),
    },
  ])
  const { code, text } = run()
  assert.equal(code, 1, text)
  assert.match(text, /go:TestA — quarantined by nocx-a \(open\)/)
  assert.match(text, /go-backend failed go:TestUnowned, which no open bead quarantines/)
  assert.match(text, /FAILED: 1 test step\(s\)/)
})

test('a closed bead protects nothing, and the report says so instead of asking for a new entry', (t) => {
  const { run } = evidence(t, [{ name: 'go-backend', status: 1, log: failure('TestC') }], {
    tests: [{ test: 'go:TestC', bead: 'nocx-c' }],
  })
  const { code, text } = run()
  assert.equal(code, 1, text)
  assert.match(text, /holds under nocx-c — a closed bead protects nothing/)
})

test('a step that failed without naming a test stays red, and a job entry holds exactly one pull request', (t) => {
  const build = [{ name: 'go-backend', status: 2, log: 'x.go:3:2: undefined: thing\n' }]
  const unnamed = evidence(t, build).run()
  assert.equal(unnamed.code, 1, unnamed.text)
  assert.match(unnamed.text, /failed without naming a test and never passed/)
  assert.match(unnamed.text, /only a job:<name>#<PR> entry holds one/)

  const held = evidence(t, build, {
    tests: [{ test: 'job:ci-backend (no-secret-service)#12', bead: 'nocx-a' }],
  })
  assert.equal(held.run().code, 0, held.run().text)
  const otherPR = held.run({ pull: '13' })
  assert.equal(otherPR.code, 1, otherPR.text)
  // Off a pull request the workflow passes an empty number, and there is then no
  // key an entry could name.
  assert.equal(held.run({ pull: '' }).code, 1)
})

test('a job whose every test step passed says so, and an unreadable evidence set is refused', (t) => {
  const { run } = evidence(t, [{ name: 'go-backend', status: 0, log: 'ok\n' }])
  const { code, text } = run()
  assert.equal(code, 0, text)
  assert.match(text, /none of its 1 recorded test step\(s\) failed/)
  assert.throws(
    () =>
      gate({
        logs: join(tmpdir(), 'nocx-quarantine-does-not-exist'),
        quarantineJson: '{"schema":1,"tests":[]}',
        issuesJsonl: '',
        write: () => {},
      }),
    /ENOENT/,
  )
})
