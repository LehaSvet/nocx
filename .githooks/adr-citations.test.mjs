/**
 * Every `ADR-NNNN` citation still resolves, because nothing was renumbered
 * (nocx-hyh2p).
 *
 * This is the assertion the change had to be safe against. Retiring the counter
 * for NEW records was only acceptable on the promise that no existing record is
 * renamed: a citation is a one-way pointer, and 278 of them were already
 * deciding by coin flip before the four pairs of 2026-08-28 were repaired. So
 * this reads every tracked document, collects every citation, and demands that
 * each one names a file in `docs/decisions/`.
 *
 * It is written to run unchanged on the tree BEFORE the draw as well: it reads
 * the citations that exist, not a list of identifiers the scheme produces. A
 * drawn citation is checked the same way, which is what keeps the new spelling
 * honest — `ADR-zv62fyqg` has to name a record the same as `ADR-0105` does.
 *
 * The volume assertion is not decoration. A glob that matched twelve files, or
 * a regex that stopped matching, would pass every "resolves" check by having
 * nothing to check.
 */
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { readdirSync, readFileSync } from 'node:fs'
import { basename, join } from 'node:path'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

import { IDENTIFIER_PATTERN } from './adr-identifier.mjs'

const root = fileURLToPath(new URL('..', import.meta.url))
const decisions = join(root, 'docs/decisions')

/** The eight symbols the generator draws, as a character class. The test below
 * asserts it IS the generator's alphabet, so a citation is read with one
 * vocabulary rather than a second, looser copy of it. */
const CROCKFORD_EIGHT = '[0123456789abcdefghjkmnpqrstvwxyz]{8}'

/** A four-digit citation, and a drawn one. A placeholder such as `ADR-NNNN` or
 * `ADR-00NN` matches neither, deliberately — those are shapes in prose, not
 * pointers. So does a word that happens to follow `ADR-`: `i`, `l`, `o` and `u`
 * are not in the alphabet, which is what `ADR-required` fails on. */
const CITATION = new RegExp(`ADR-(?:\\d{4}|${CROCKFORD_EIGHT})(?![\\w-])`, 'g')

/** Every tracked markdown document, from git rather than from a walk: a walk
 * reads a worktree that a run has not committed yet, and CI has no such thing
 * to read. */
function trackedDocuments() {
  const listed = execFileSync('git', ['ls-files', '-z', '--', '*.md'], {
    cwd: root,
    encoding: 'utf8',
  })
  return listed.split('\0').filter((path) => path !== '')
}

test("the citation reader draws on the generator's own alphabet", () => {
  assert.equal(IDENTIFIER_PATTERN.source, `^${CROCKFORD_EIGHT}$`)
})

test('every ADR citation in the tracked documents names a record file', () => {
  const records = new Set(
    readdirSync(decisions)
      .filter((name) => name !== 'INDEX.md' && name.endsWith('.md'))
      .map((name) => name.split('-')[0]),
  )

  const documents = trackedDocuments()
  assert.ok(documents.length > 200, `only ${documents.length} documents were read`)

  const citations = []
  for (const path of documents) {
    const text = readFileSync(join(root, path), 'utf8')
    for (const match of text.matchAll(CITATION)) {
      citations.push({ identifier: match[0].slice('ADR-'.length), path })
    }
  }

  assert.ok(citations.length > 500, `only ${citations.length} citations were read`)

  const unresolved = citations
    .filter(({ identifier }) => !records.has(identifier))
    .map(({ identifier, path }) => `${path} cites ADR-${identifier}, which names no record`)

  assert.deepEqual(unresolved, [])

  // A citation from outside docs/decisions, so this is not only the records
  // citing themselves.
  assert.ok(
    citations.some(({ path }) => !path.startsWith('docs/decisions/')),
    'every citation found was inside docs/decisions',
  )
})

test('the citations the drawn scheme added resolve like any other', () => {
  const identifiers = new Set(
    readdirSync(decisions)
      .filter((name) => name.endsWith('.md') && name !== 'INDEX.md')
      .map((name) => name.split('-')[0])
      .filter((identifier) => !/^\d{4}$/.test(identifier)),
  )

  assert.ok(identifiers.size > 0, 'no drawn identifier is in docs/decisions')

  for (const identifier of identifiers) {
    const cited = trackedDocuments().filter((path) =>
      readFileSync(join(root, path), 'utf8').includes(`ADR-${identifier}`),
    )
    assert.ok(cited.length > 0, `ADR-${identifier} is in the tree and cited by no document`)
    assert.ok(
      cited.some((path) => !basename(path).startsWith(identifier)),
      `ADR-${identifier} is cited only by its own record`,
    )
  }
})
