/**
 * The uniqueness check refuses a collision and names both sides of it
 * (nocx-hyh2p).
 *
 * Every test here drives the real program over a real directory, because the
 * property is about a SET of files: a unit test of a function that happens to
 * group a list proves the grouping and says nothing about whether the commit
 * that adds a second record is refused.
 *
 * The legacy cases are the owner's decision of 2026-10-26, and they are paired
 * on purpose: a unique `0106-…` record is ACCEPTED, because the numbers are
 * retired rather than banned and the updater pull request adds exactly that
 * file; two `0106-…` records are refused like any other pair, because a legacy
 * number is not a licence to collide.
 */
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { existsSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

import { readIdentifier } from './adr-records.mjs'

const check = fileURLToPath(new URL('./check-adr-identifiers.mjs', import.meta.url))
const decisions = fileURLToPath(new URL('../docs/decisions', import.meta.url))

/** A directory of records, as a branch would leave it. */
function fixture(files) {
  const dir = mkdtempSync(join(tmpdir(), 'nocx-adr-identifiers-'))
  for (const [name, text] of Object.entries(files)) writeFileSync(join(dir, name), text)
  return dir
}

function run(dir) {
  const result = spawnSync(process.execPath, [check, dir], { encoding: 'utf8' })
  return { status: result.status, stdout: result.stdout, stderr: result.stderr }
}

const record = (identifier) => `# ADR-${identifier} — a decision\n\n- **Date:** 2026-10-26\n`

test('two records with one identifier are refused, and both files are named', () => {
  const dir = fixture({
    'INDEX.md': '# Architecture decision records\n',
    'k3f9a2bc-the-first-decision.md': record('k3f9a2bc'),
    'k3f9a2bc-the-second-decision.md': record('k3f9a2bc'),
  })
  try {
    const { status, stderr } = run(dir)
    assert.equal(status, 1, `the check accepted a collision:\n${stderr}`)
    assert.match(stderr, /k3f9a2bc-the-first-decision\.md/)
    assert.match(stderr, /k3f9a2bc-the-second-decision\.md/)
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

test('two distinct identifiers pass', () => {
  const dir = fixture({
    'INDEX.md': '# Architecture decision records\n',
    'k3f9a2bc-the-first-decision.md': record('k3f9a2bc'),
    'm7p1q4tz-the-second-decision.md': record('m7p1q4tz'),
  })
  try {
    const { status, stderr } = run(dir)
    assert.equal(status, 0, stderr)
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

test('a unique legacy number is accepted: the numbers are retired, not banned', () => {
  const dir = fixture({
    'INDEX.md': '# Architecture decision records\n',
    '0105-a-record-written-before-the-draw.md': record('0105'),
    '0106-the-updaters-record.md': record('0106'),
    'k3f9a2bc-the-first-drawn-record.md': record('k3f9a2bc'),
  })
  try {
    const { status, stderr } = run(dir)
    assert.equal(status, 0, stderr)
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

test('two records with one legacy number are refused like any other pair', () => {
  const dir = fixture({
    'INDEX.md': '# Architecture decision records\n',
    '0106-the-updaters-record.md': record('0106'),
    '0106-a-second-record.md': record('0106'),
  })
  try {
    const { status, stderr } = run(dir)
    assert.equal(status, 1, `a number was read as a licence to collide:\n${stderr}`)
    assert.match(stderr, /0106-the-updaters-record\.md/)
    assert.match(stderr, /0106-a-second-record\.md/)
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

test('a file no citation could name is refused, and the name it needs is printed', () => {
  const dir = fixture({
    'INDEX.md': '# Architecture decision records\n',
    'notes-on-the-scheme.md': record('k3f9a2bc'),
  })
  try {
    const { status, stderr } = run(dir)
    assert.equal(status, 1, `a nameless record was accepted:\n${stderr}`)
    assert.match(stderr, /notes-on-the-scheme\.md/)
    assert.match(stderr, /eight Crockford base32 symbols/)
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

test('the untouched tree passes, legacy numbers and all', () => {
  const { status, stderr } = run(decisions)
  assert.equal(status, 0, stderr)
})

test('the record that changed the scheme is accepted by the scheme it checks', () => {
  // Named rather than discovered: a test that accepts whatever file happens to
  // be in the directory passes on the day the record is deleted.
  const changed = 'zv62fyqg-a-new-records-identifier-is-drawn-never-counted.md'
  assert.ok(existsSync(join(decisions, changed)), `${changed} is missing`)
  assert.equal(readIdentifier(changed)?.legacy, false, `${changed} carries a number, not a draw`)

  // Beside a legacy record in the same directory, because the pair is the
  // property: one directory holds both shapes and the check accepts both.
  const legacy = '0001-xterm-js-as-vt-frontend.md'
  assert.ok(existsSync(join(decisions, legacy)), `${legacy} is missing`)
  assert.equal(readIdentifier(legacy)?.legacy, true)

  const { status, stderr } = run(decisions)
  assert.equal(status, 0, stderr)
})
