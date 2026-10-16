import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'

test('pre-push status check does not consume Git ref input', () => {
  const bin = mkdtempSync(join(tmpdir(), 'nocx-pre-push-'))
  try {
    const br = join(bin, 'br')
    writeFileSync(
      br,
      `#!/bin/sh\nif [ "$3" = where ]; then exit 0; fi\ncat >/dev/null\nprintf "Dirty issues: 0\\n"\n`,
    )
    // The status command is allowed to read its stdin; the hook must not hand it Git ref input.
    const chmod = spawnSync('chmod', ['+x', br])
    assert.equal(chmod.status, 0)
    const zero = '0'.repeat(40)
    const input = `(delete) ${zero} refs/heads/gone ${zero}\n`
    const result = spawnSync(
      'sh',
      ['.githooks/pre-push', 'origin', 'https://example.invalid/repo.git'],
      {
        encoding: 'utf8',
        input,
        env: { ...process.env, PATH: `${bin}:${process.env.PATH}` },
      },
    )
    assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`)
    assert.doesNotMatch(result.stderr, /pre-push input is empty|stdin contains no ref lines/)
  } finally {
    rmSync(bin, { recursive: true, force: true })
  }
})
