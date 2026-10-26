/**
 * What a decision record's file name is (nocx-hyh2p).
 *
 * A record is `docs/decisions/<identifier>-<slug>.md`. Two shapes of identifier
 * exist, and there will never be a third:
 *
 *   - a LEGACY identifier is the four-digit number of a record written while
 *     records were numbered. `0001`–`0105` are in the tree, and `0106` is a
 *     record on its way in. An accepted record is never edited, and every
 *     citation written against these must keep resolving, so they are read
 *     here and not rewritten.
 *   - a DRAWN identifier is what a new record takes: eight Crockford base32
 *     symbols, from `adr-identifier.mjs`. No counter takes part, so two
 *     branches adding a record to the same base cannot take one identifier
 *     between them.
 *
 * `INDEX.md` is the one file in the directory that is not a record. Anything
 * else whose name is not one of these two shapes is a name no reader can
 * resolve, which is why `readIdentifier` answers null for it rather than
 * guessing.
 */
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

import { IDENTIFIER_PATTERN, LEGACY_PATTERN } from './adr-identifier.mjs'

export const DECISIONS_DIR = 'docs/decisions'

/** The index is the directory's one non-record file. */
export const INDEX_FILE = 'INDEX.md'

/** `<identifier>-<slug>.md`, with the slug required: an identifier alone names
 * no decision. */
const RECORD_FILE = /^([^-]+)-(.+)\.md$/

/** The identifier a record's file name carries, and whether it is a number from
 * the numbered era; null when the name is not a record's name at all. */
export function readIdentifier(filename) {
  const match = RECORD_FILE.exec(filename)
  if (match === null) return null
  const identifier = match[1]
  if (LEGACY_PATTERN.test(identifier)) return { identifier, legacy: true }
  if (IDENTIFIER_PATTERN.test(identifier)) return { identifier, legacy: false }
  return null
}

/** Everything before the record's first level-2 heading: its header block. A
 * date below it is prose about the decision, not the record's own date. */
function header(text) {
  const match = /\n## /.exec(text)
  return match === null ? text : text.slice(0, match.index)
}

/** The date a record gives for itself, as `YYYY-MM-DD`, or null.
 *
 * There is no single shape: 94 records carry a `- **Date:** YYYY-MM-DD` field,
 * two a plain `Date: YYYY-MM-DD` line, one only the acceptance inside its
 * status, and eight an `accepted (YYYY-MM-DD)` clause. A parser written for the
 * first shape alone silently loses fourteen records, so each shape is read in
 * turn and the field wins over the prose. The accepted clause takes its LAST
 * match, because a superseded record's status names the date it was superseded
 * before the date it was accepted.
 */
export function recordDate(text) {
  const head = header(text)
  const field = /^\s*[-*]?\s*\*\*Date:\*\*\s*(\d{4}-\d{2}-\d{2})/m.exec(head)
  if (field !== null) return field[1]
  const plain = /^\s*Date:\s*(\d{4}-\d{2}-\d{2})/m.exec(head)
  if (plain !== null) return plain[1]
  const accepted = [...head.matchAll(/[Aa]ccepted\s*(\d{4}-\d{2}-\d{2})/g)]
  if (accepted.length > 0) return accepted[accepted.length - 1][1]
  const loose = /(\d{4}-\d{2}-\d{2})/.exec(head)
  return loose === null ? null : loose[1]
}

/** The identifier a record's heading names, e.g. `ADR-0105` or `ADR-zv62fyqg`.
 * Null when the heading names none. */
export function readHeadingIdentifier(text) {
  const match = /^#\s*ADR[- ]?([0-9a-z]+)\b/m.exec(text)
  return match === null ? null : match[1]
}

/** Every record in a directory, with the date it gives for itself. */
export function readRecords(dir) {
  const records = []
  for (const name of readdirSync(dir).sort()) {
    if (!name.endsWith('.md') || name === INDEX_FILE) continue
    const read = readIdentifier(name)
    if (read === null) continue
    const text = readFileSync(join(dir, name), 'utf8')
    records.push({ file: name, ...read, date: recordDate(text), text })
  }
  return records
}

/** One row of `INDEX.md`: `| id | date | [title](file) | status |`. */
const INDEX_ROW =
  /^\|\s*([^|]+?)\s*\|\s*(\d{4}-\d{2}-\d{2})\s*\|\s*\[(.*?)\]\(([^)]+)\)\s*\|\s*(.*?)\s*\|$/

/** The rows of an index table, in the order they are written. */
export function indexRows(text) {
  const rows = []
  for (const line of text.split('\n')) {
    const match = INDEX_ROW.exec(line)
    if (match === null) continue
    rows.push({
      identifier: match[1],
      date: match[2],
      title: match[3],
      file: match[4],
      status: match[5],
    })
  }
  return rows
}

/** What a name must look like, for a message that tells a person what to do. */
export const NAME_RULE =
  '<identifier>-<slug>.md, where the identifier is eight Crockford base32 symbols (node .githooks/adr-identifier.mjs prints one) or, for a record written before the draw, a four-digit number'
