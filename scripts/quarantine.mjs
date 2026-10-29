// One owner for the test quarantine: the file, the log parse, and the verdict.
//
// Two callers ask the same question of the same data — the promotion
// (scripts/release/promotion.mjs), which reads a merged pull request's job logs
// through the GitHub API, and a pull request's own CI
// (scripts/quarantine-gate.mjs), which reads the job's own output. A second
// parser or a second verdict would be a regression with a delay fuse: the two
// agree everywhere anyone looks and disagree somewhere nobody does.
//
// ADR-xf4p9fzd is the decision (a flake is quarantined by its open bead);
// ADR-c7fbhtkt widened a key to a whole job; ADR-0bawnmfg is CI honouring it.

const requireThat = (condition, message) => {
  if (!condition) throw new Error(message)
}

// A key is the name the suite prints on failure, without line numbers, which
// move: `go:<TestName>`, `e2e:<spec path> › <title path>`, or — for a known
// defect of a check rather than of a test — `job:<job name>#<pull number>`.
const keyPattern = /^(go:Test\w+|e2e:e2e\/\S+ › .+|job:[^#\n]+#[1-9]\d*)$/

// A quarantined test is one a flake bead owns (ADR "a flaky test is quarantined
// by its bead"). Its failure stops blocking only while that bead is open:
// closing the bead ends the protection without anyone remembering to edit this
// file.
export function readQuarantine(json, issuesJsonl) {
  return readQuarantineState(json, issuesJsonl).open
}

// The same read, keeping the entries whose bead is closed. A closed bead
// protects nothing, and the caller that reports a red job needs the difference:
// "no bead quarantines this" sends the reader to file one, while "the file
// holds it under a closed bead" sends them to reopen the bead or fix the test.
export function readQuarantineState(json, issuesJsonl) {
  if (json === undefined) return { open: {}, closed: {} }
  const doc = JSON.parse(json)
  requireThat(doc?.schema === 1 && Array.isArray(doc.tests), 'invalid test quarantine')
  const status = new Map()
  for (const line of (issuesJsonl ?? '').split('\n')) {
    if (!line.trim()) continue
    const row = JSON.parse(line)
    status.set(row.id, row.status)
  }
  const open = {}
  const closed = {}
  for (const entry of doc.tests) {
    requireThat(
      typeof entry?.test === 'string' &&
        keyPattern.test(entry.test) &&
        typeof entry.bead === 'string',
      `invalid test quarantine entry ${JSON.stringify(entry)}`,
    )
    const state = status.get(entry.bead)
    requireThat(state !== undefined, `quarantine ${entry.test}: bead ${entry.bead} does not exist`)
    if (['closed', 'tombstone'].includes(state)) closed[entry.test] = entry.bead
    else open[entry.test] = entry.bead
  }
  return { open, closed }
}

// The tests a failed job's log names, in the quarantine's key form. A failure
// the log does not name (a build error, a timeout panic, the job's own setup)
// yields nothing, and nothing cannot be quarantined.
//
// ONE parser for both callers, which is why the CI job keeps its own output in
// this exact shape: scripts/ci-run-logged.sh tees a step's stdout and stderr to
// a file, and that file is what GET /repos/{repo}/actions/jobs/{id}/logs would
// return for the same step. The timestamp prefix is the API's, and the CI-side
// file has none — the strip below is why one function reads both.
export function failedTests(log) {
  const keys = new Set()
  for (const raw of log.split(/\r?\n/)) {
    const line = raw.replace(/^\S+Z /, '')
    const go = /^\s*--- FAIL: (Test\w+)/.exec(line)
    if (go) keys.add(`go:${go[1]}`)
    const e2e = /^\s*\d+\) \[[^\]]+\] › (e2e\/[^:\s]+):\d+:\d+ › (.+?)[\s─]*$/.exec(line)
    if (e2e) keys.add(`e2e:${e2e[1]} › ${e2e[2]}`)
  }
  return [...keys].sort()
}

// The verdict on one job's failures, in one place for both callers.
//
// Every failure must be held by an open bead. A failure that names no test is
// not a flake anyone can quarantine by test name: it passes only when the same
// job passed in another attempt (the promotion's reading of a retry that
// classified an infrastructure flake) or when a bead quarantines exactly this
// job of exactly this pull request (`job:<name>#<PR>`) — a known defect of the
// check itself, never a class. A pull request's own CI has no other attempt, so
// only the job entry can answer there.
//
// `reason` is phrased for the caller's line prefix, so the promotion's
// `requireThat` and the CI gate's report refuse in the same words.
export function judgeFailures({
  failures,
  quarantine,
  closed = {},
  jobKey,
  passedInAnotherAttempt = false,
}) {
  const quarantined = []
  const unquarantined = []
  for (const test of failures) {
    const bead = quarantine[test]
    if (bead === undefined) unquarantined.push(test)
    else quarantined.push({ test, bead })
  }
  if (failures.length === 0) {
    const held =
      passedInAnotherAttempt || (jobKey !== undefined && quarantine[jobKey] !== undefined)
    if (!passedInAnotherAttempt && held)
      quarantined.push({ test: jobKey, bead: quarantine[jobKey] })
    return held
      ? { accepted: true, quarantined, unquarantined, reason: '' }
      : {
          accepted: false,
          quarantined,
          unquarantined,
          reason: 'failed without naming a test and never passed',
        }
  }
  if (unquarantined.length === 0) return { accepted: true, quarantined, unquarantined, reason: '' }
  const test = unquarantined[0]
  const bead = closed[test]
  return {
    accepted: false,
    quarantined,
    unquarantined,
    reason: bead
      ? `failed ${test}, which docs/testing/quarantine.json holds under ${bead} — a closed bead protects nothing`
      : `failed ${test}, which no open bead quarantines`,
  }
}
