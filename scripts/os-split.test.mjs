import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { test } from 'node:test'

const makefile = readFileSync(new URL('../Makefile', import.meta.url), 'utf8')

function fixture(run) {
  const root = mkdtempSync(join(tmpdir(), 'nocx-os-split-'))
  try {
    writeFileSync(join(root, 'Makefile'), makefile)
    return run(root)
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
}

function make(root, args) {
  return execFileSync(
    'make',
    ['--no-print-directory', '-s', 'HOST_GOOS=linux', 'GO=true', ...args],
    {
      cwd: root,
      encoding: 'utf8',
    },
  ).trim()
}

function variable(root, name, args = []) {
  return make(root, [
    ...args,
    '--eval',
    `print-variable:;@printf '%s' '$(${name})'`,
    'print-variable',
  ])
}

test('desktopidentity belongs to the real OS-split package set', () => {
  fixture((root) => {
    assert.ok(variable(root, 'OS_PKG_DIRS').split(/\s+/).includes('internal/desktopidentity'))
  })
})

test('OS regex is derived from the package list, including later additions', () => {
  fixture((root) => {
    assert.equal(
      variable(root, 'OS_PKG_RE', ['OS_PKG_DIRS=internal/example internal/another']),
      '(internal/example|internal/another)',
    )
  })
})

test('OS constraint derivation excludes testdata, like go list ./...', () => {
  fixture((root) => {
    for (const file of [
      'internal/example/platform.go',
      'internal/example/testdata/probe/main.go',
    ]) {
      const path = join(root, file)
      mkdirSync(dirname(path), { recursive: true })
      writeFileSync(path, '//go:build linux\n\npackage example\n')
    }
    assert.match(make(root, ['OS_PKG_DIRS=internal/example', 'OS_EXEMPT=', 'ci-os-split']), /ok$/)
  })
})
