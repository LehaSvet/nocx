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

/** What a name must look like, for a message that tells a person what to do. */
export const NAME_RULE =
  '<identifier>-<slug>.md, where the identifier is eight Crockford base32 symbols (node .githooks/adr-identifier.mjs prints one) or, for a record written before the draw, a four-digit number'
