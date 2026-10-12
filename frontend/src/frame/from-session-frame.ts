// Mint the local pinned view from one accepted backend frame. This path reads
// the immutable cell model, never PTY bytes or a second terminal parser.
import type { ScreenSnapshot } from '../cell-model'
import type { TerminalSnapshot } from '../scrollback/serializer'
import { cellAttrsOf } from '../painter/style'
import type { CapturedFrame, FrameRow } from './types'

export interface AbsoluteRowRange {
  readonly start: number
  readonly end: number
}

const RETAINED_SCREEN_ROWS = 10000
const MAX_CAPTURE_ROWS = 10000
const MAX_JSON_SAFE_INTEGER = Number.MAX_SAFE_INTEGER

/** Capture a range from the accepted viewport. Rows and cursor use the
 * backend's shared absolute active-buffer coordinate. */
export function captureAcceptedScreen(
  snapshot: ScreenSnapshot,
  palette: TerminalSnapshot,
  requested?: AbsoluteRowRange,
): CapturedFrame {
  const baseRow = snapshot.state.baseRow
  const viewportEnd = baseRow + snapshot.geometry.rows
  if (
    !Number.isSafeInteger(baseRow) ||
    baseRow < 0 ||
    !Number.isSafeInteger(snapshot.geometry.rows) ||
    snapshot.geometry.rows < 0 ||
    !Number.isSafeInteger(viewportEnd) ||
    viewportEnd > MAX_JSON_SAFE_INTEGER
  ) {
    throw new RangeError('accepted screen rows exceed the JSON safe-integer range')
  }
  const range = requested ?? { start: baseRow, end: viewportEnd }
  if (
    !Number.isSafeInteger(range.start) ||
    !Number.isSafeInteger(range.end) ||
    range.start < baseRow ||
    range.end < range.start ||
    range.end > viewportEnd ||
    range.end > MAX_JSON_SAFE_INTEGER ||
    range.end - range.start > MAX_CAPTURE_ROWS
  ) {
    throw new RangeError(
      `screen range [${range.start}, ${range.end}) is outside accepted viewport [${baseRow}, ${viewportEnd}) or exceeds the ${MAX_CAPTURE_ROWS}-row/JSON-safe limit`,
    )
  }

  const rows: FrameRow[] = []
  for (let absoluteRow = range.start; absoluteRow < range.end; absoluteRow++) {
    const row = snapshot.rowAt(absoluteRow - baseRow)
    if (row === null) {
      throw new RangeError(`accepted screen row ${absoluteRow} is not present in its frame`)
    }
    rows.push({
      kind: 'cells',
      cells: row.cells.map((cell) => ({
        char: cell.grapheme || ' ',
        attrs: cellAttrsOf(cell.style, palette),
      })),
    })
  }

  const buffer =
    snapshot.state.bufferKind === 'alternate'
      ? { kind: 'alternate' as const, altSession: snapshot.state.bufferInstance }
      : { kind: 'normal' as const, instance: snapshot.state.bufferInstance }
  const cursorRow = baseRow + snapshot.cursor.y
  return {
    rows,
    cursor: {
      line: cursorRow,
      col: Math.min(Math.max(snapshot.cursor.x, 0), Math.max(snapshot.geometry.cols - 1, 0)),
    },
    provenance: {
      source: 'live',
      identity: {
        buffer,
        cols: snapshot.geometry.cols,
        rows: snapshot.geometry.rows,
        generation: snapshot.revision,
      },
      range: { ...range },
      scrollbackCapLines: RETAINED_SCREEN_ROWS,
    },
  }
}
