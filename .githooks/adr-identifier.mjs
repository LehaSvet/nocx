/**
 * The identifier of a new decision record is drawn, never counted (nocx-hyh2p).
 *
 * A central counter cannot survive two branches adding a record to the same
 * base: each picks the next number, both are the next number, and the merge has
 * to renumber one of them — which breaks every citation written against it in
 * the meantime. A draw needs nothing from the base, so two branches collide with
 * probability 2^-40 instead of with certainty.
 *
 * Eight symbols of Crockford base32, lower case: 40 bits, which puts the chance
 * of any two of ten thousand records sharing an identifier at about 5 in 10^5.
 * The alphabet leaves out `i`, `l`, `o` and `u`, so an identifier read off a
 * screen and typed back is the identifier that was meant.
 *
 * `node .githooks/adr-identifier.mjs` prints one, which is what a person adding
 * a record does. The file is then `<identifier>-<slug>.md`.
 */
import { randomInt } from 'node:crypto'
import { pathToFileURL } from 'node:url'

const ALPHABET = '0123456789abcdefghjkmnpqrstvwxyz'

export const IDENTIFIER_LENGTH = 8

/** What a drawn identifier looks like. A legacy numeric identifier (`0105`) is
 * also a valid identifier in a filename; this pattern is about what WE draw. */
export const IDENTIFIER_PATTERN = new RegExp(`^[${ALPHABET}]{${IDENTIFIER_LENGTH}}$`)

/** One identifier, uniform over the alphabet, from the OS entropy pool. */
export function drawIdentifier() {
  let id = ''
  for (let i = 0; i < IDENTIFIER_LENGTH; i += 1) id += ALPHABET[randomInt(ALPHABET.length)]
  return id
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  process.stdout.write(`${drawIdentifier()}\n`)
}
