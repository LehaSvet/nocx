import test from 'node:test'
import assert from 'node:assert/strict'
import { summarizeGoTestEvents } from './summarize-go-test-json.mjs'

test('reports the 20 slowest test outcomes separately for each package', () => {
  const events = [
    { Action: 'pass', Package: 'example/a', Test: 'fast', Elapsed: 0.25 },
    { Action: 'fail', Package: 'example/a', Test: 'TestFailing', Elapsed: 4 },
    ...Array.from({ length: 22 }, (_, i) => ({
      Action: 'pass',
      Package: 'example/a',
      Test: `Test${i}`,
      Elapsed: i + 1,
    })),
    { Action: 'pass', Package: 'example/b', Test: 'only-test', Elapsed: 2.5 },
    { Action: 'output', Package: 'example/a', Test: 'not-a-result', Elapsed: 1000 },
  ]

  const summary = summarizeGoTestEvents(events)
  assert.deepEqual(
    summary.map((row) => row.package),
    ['example/a', 'example/b'],
  )
  assert.equal(summary[0].tests.length, 20)
  assert.equal(summary[0].tests[0].name, 'Test21')
  assert.equal(
    summary[0].tests.some(({ name }) => name === 'TestFailing'),
    true,
  )
  assert.equal(summary[0].tests[0].seconds, 22)
  assert.equal(summary[0].tests.at(-1).name, 'TestFailing')
  assert.deepEqual(summary[1].tests, [{ name: 'only-test', seconds: 2.5 }])
})

test('ignores package-only results and orders ties by test name', () => {
  const summary = summarizeGoTestEvents([
    { Action: 'pass', Package: 'example/a', Elapsed: 1 },
    { Action: 'pass', Package: 'example/a', Test: 'z', Elapsed: 1 },
    { Action: 'pass', Package: 'example/a', Test: 'a', Elapsed: 1 },
  ])
  assert.deepEqual(
    summary[0].tests.map(({ name }) => name),
    ['a', 'z'],
  )
})

test('workflow pipeline prints Go output verbatim and preserves go test exit statuses, including signal-style', async () => {
  const { mkdtemp, writeFile, chmod, rm } = await import('node:fs/promises')
  const { tmpdir } = await import('node:os')
  const { join } = await import('node:path')
  const { spawnSync } = await import('node:child_process')
  const dir = await mkdtemp(join(tmpdir(), 'go-test-json-'))
  try {
    const fakeGo = join(dir, 'go')
    await writeFile(
      fakeGo,
      `#!/bin/sh
printf '%s\\n' '{"Action":"pass","Package":"example/app","Test":"TestFailurePath","Elapsed":9}'
printf '%s\\n' '{"Action":"output","Package":"example/app","Test":"TestFailurePath","Output":"FAIL diagnostic\\npanic: test timed out\\n goroutine 1 [running]:\\n"}'
exit "${'${FAKE_GO_STATUS:-23}'}"
`,
    )
    await chmod(fakeGo, 0o755)
    const script = `set +e
go test -json -race -tags gtk3,nocx_framecheck -count=1 ./internal/app/... | node scripts/summarize-go-test-json.mjs
pipeline_status=("${'${PIPESTATUS[@]}'}")
exit "${'${pipeline_status[0]}'}"`
    for (const exitStatus of [0, 1, 23, 130]) {
      const result = spawnSync('bash', ['-c', script], {
        cwd: new URL('..', import.meta.url),
        env: {
          ...process.env,
          PATH: `${dir}:${process.env.PATH}`,
          FAKE_GO_STATUS: String(exitStatus),
        },
        encoding: 'utf8',
      })
      assert.equal(result.status, exitStatus)
      assert.match(
        result.stdout,
        /FAIL diagnostic\npanic: test timed out\n goroutine 1 \[running\]:\n/,
      )
      assert.doesNotMatch(result.stdout, /"Action":"output"/)
      assert.match(result.stderr, /Slowest tests in example\/app/)
    }
  } finally {
    await rm(dir, { recursive: true, force: true })
  }
})
