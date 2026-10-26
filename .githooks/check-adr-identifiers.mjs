#!/usr/bin/env node
/**
 * No two decision records carry one identifier (nocx-hyh2p).
 *
 * A number was chosen by whoever wrote the record, from a counter only their
 * own branch could see, so two branches open at once chose the same number and
 * the merge renumbered one of them — after citations to it had been written.
 * Four such pairs were repaired in nocx-yjvg5 and three more in nocx-jp0h3.
 * A draw removes the coordination; this check removes the silence. It cannot
 * stop a collision (nothing can, short of a lock) but it makes one refuse the
 * commit and the pull request instead of reaching `main` and being found by the
 * reader of a citation.
 *
 * It refuses:
 *   - two records with the same identifier, naming EVERY file that holds it, so
 *     the reader sees both sides of the collision and not the one they staged;
 *   - a file in the directory whose name is neither a drawn identifier nor a
 *     legacy number, which is a record no citation can name.
 *
 * It accepts a unique legacy number, deliberately. The numbers are retired, not
 * banned: `0106` is a record the updater pull request adds, and it is a record
 * the same way `0001` is.
 *
 *   node .githooks/check-adr-identifiers.mjs [directory]
 *
 * The directory is a parameter because the tests drive the check over a
 * fixture; every caller in the repository omits it.
 */
import { readdirSync } from 'node:fs'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

import { DECISIONS_DIR, INDEX_FILE, NAME_RULE, readIdentifier } from './adr-records.mjs'

/** Every problem in one directory, each message naming the files it is about. */
export function findProblems(dir) {
  const problems = []
  const holders = new Map()

  for (const name of readdirSync(dir).sort()) {
    if (!name.endsWith('.md') || name === INDEX_FILE) continue
    const path = join(dir, name)
    const read = readIdentifier(name)
    if (read === null) {
      problems.push(`${path} does not name one record: a record's file is ${NAME_RULE}`)
      continue
    }
    const files = holders.get(read.identifier) ?? []
    files.push(path)
    holders.set(read.identifier, files)
  }

  for (const [identifier, files] of holders) {
    if (files.length < 2) continue
    problems.push(
      `identifier ${identifier} names ${files.length} records: ${files.join(' and ')} — one identifier, one decision`,
    )
  }

  return problems
}

function main(argv) {
  const dir = argv[2] ?? DECISIONS_DIR
  let problems
  try {
    problems = findProblems(dir)
  } catch (error) {
    process.stderr.write(`FAIL: ${dir} cannot be read: ${error.message}\n`)
    return 2
  }

  for (const problem of problems) process.stderr.write(`FAIL: ${problem}\n`)
  if (problems.length > 0) {
    process.stderr.write(
      '\nA decision record takes an identifier no other record has: `node .githooks/adr-identifier.mjs` prints one, and the file becomes <identifier>-<slug>.md.\n',
    )
    return 1
  }

  process.stdout.write(`OK: ${dir} holds no identifier that names two records\n`)
  return 0
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  process.exit(main(process.argv))
}
