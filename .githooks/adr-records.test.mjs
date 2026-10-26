/**
 * `INDEX.md` lists the records by date, and every record is in it once
 * (nocx-hyh2p).
 *
 * The claim this file checks is the one a number used to make silently: a
 * reader scanning the list is reading it in order. With drawn identifiers there
 * is no order in the identifier at all, so the order comes from the date each
 * record gives for itself, and a row's date has to BE that date rather than a
 * copy that agreed on the day it was typed.
 *
 * So the assertions are: one row per record and one record per row, each row's
 * date equal to the date in the record it points at, and the rows' own dates in
 * non-decreasing order. Prose saying "newest last" is what was there before and
 * it was false nine times.
 */
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

import { indexRows, recordDate, readRecords } from './adr-records.mjs'

const decisions = fileURLToPath(new URL('../docs/decisions', import.meta.url))
const index = readFileSync(join(decisions, 'INDEX.md'), 'utf8')

test('every record has one row, and every row names a record', () => {
  const records = readRecords(decisions)
  const rows = indexRows(index)

  assert.ok(records.length > 0, 'no records were read from docs/decisions')
  assert.deepEqual(
    rows.map((row) => row.file).sort(),
    records.map((record) => record.file).sort(),
    'INDEX and docs/decisions disagree about which records exist',
  )
  assert.equal(new Set(rows.map((row) => row.file)).size, rows.length, 'a record has two rows')
  assert.equal(
    new Set(rows.map((row) => row.identifier)).size,
    rows.length,
    'two rows carry one identifier',
  )
})

test('the rows are in the order of the dates the records give', () => {
  const rows = indexRows(index)
  const dates = rows.map((row) => row.date)

  const outOfOrder = dates
    .map((date, i) => ({ date, previous: dates[i - 1], file: rows[i].file }))
    .filter(({ date, previous }) => previous !== undefined && date < previous)
    .map(({ previous, date, file }) => `${file} is ${date} after ${previous}`)

  assert.deepEqual(outOfOrder, [], 'INDEX is not in date order')
})

test('each row dates its record as the record dates itself', () => {
  const byFile = new Map(readRecords(decisions).map((record) => [record.file, record]))
  const disagreeing = indexRows(index)
    .filter((row) => byFile.get(row.file)?.date !== row.date)
    .map(
      (row) => `${row.file}: INDEX says ${row.date}, the record says ${byFile.get(row.file)?.date}`,
    )

  assert.deepEqual(disagreeing, [])
})

test('a record date is read from the record itself, in each shape its header takes', () => {
  assert.equal(recordDate('# ADR-0001 — x\n\n- **Date:** 2026-07-21\n'), '2026-07-21')
  assert.equal(recordDate('# ADR-0084 — x\n\nDate: 2026-10-12\n'), '2026-10-12')
  assert.equal(
    recordDate(
      '# ADR-0022 — x\n\n- **Status:** Superseded by ADR-0049 (accepted 2026-08-20).\n  Accepted 2026-08-05.\n',
    ),
    '2026-08-05',
    'the acceptance was read as the supersession date',
  )
  assert.equal(recordDate('# ADR-0026 — x\n\n- **Status:** accepted (2026-08-08)\n'), '2026-08-08')
  assert.equal(recordDate('# ADR-0001 — x\n\nNo date anywhere.\n'), null)
  assert.equal(
    recordDate('# ADR-0001 — x\n\n- **Date:** 2026-07-23\n\n## Context\n\nMeasured 2026-09-01.\n'),
    '2026-07-23',
    'a date in the body was read as the record date',
  )
})
