// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { DEFAULT_SNAPSHOT } from '../scrollback/serializer'
import { captureAcceptedScreen } from './from-session-frame'
import { createCapturedFrameView } from './display'
import { frameOf, snapshotOf, styleOf } from '../painter/fixtures'
import type { CellSpec } from '../painter/fixtures'
import type { CapturedFrame } from './types'

function liveProvenance(frame: CapturedFrame) {
  if (frame.provenance.source !== 'live') throw new Error('expected live screen capture')
  return frame.provenance
}

const blankRows = (first: CellSpec[]): CellSpec[][] => [first]

describe('captureAcceptedScreen', () => {
  it('uses absolute rows and the backend buffer identity from the accepted frame', () => {
    const frame = frameOf(
      7,
      blankRows([
        [
          'A',
          1,
          true,
          styleOf({
            foreground: { kind: 2, palette: 0, rgb: { r: 255, g: 0, b: 0 } },
            attributes: 1,
          }),
        ],
      ]),
      { x: 0, y: 0, visible: true },
      {
        cols: 4,
        rows: 1,
      },
      {
        bufferKind: 'normal',
        bufferInstance: '18446744073709551615',
        baseRow: 12,
        bracketedPaste: false,
        mouseTracking: false,
        title: null,
        cwd: null,
      },
    )

    const captured = captureAcceptedScreen(snapshotOf(frame), DEFAULT_SNAPSHOT)

    const provenance = liveProvenance(captured)
    expect(provenance.range).toEqual({ start: 12, end: 13 })
    expect(provenance.identity.buffer).toEqual({
      kind: 'normal',
      instance: '18446744073709551615',
    })
    expect(provenance.identity.generation).toBe(7)
    expect(captured.cursor).toEqual({ line: 12, col: 0 })
    expect(captured.rows).toHaveLength(1)
    const capturedRow = captured.rows[0]
    if (capturedRow.kind !== 'cells') throw new Error('expected cell row')
    expect(capturedRow.cells[0]).toMatchObject({
      char: 'A',
      attrs: { fg: 'rgb(255, 0, 0)', bold: true },
    })
    const view = createCapturedFrameView(captured)
    expect(view.className).toBe('nocx-freeze-frame')
    expect(view.dataset.source).toBe('live')
    expect(view.textContent).toContain('A')
    expect(view.querySelectorAll('.nocx-freeze-frame__row')).toHaveLength(1)
    expect(view.querySelectorAll('.nocx-freeze-frame__cell')).toHaveLength(4)
  })

  it('rejects ranges that exceed the JSON-safe absolute row boundary', () => {
    const frame = frameOf(9, blankRows([['T', 1, true]]), { x: 0, y: 0, visible: false })
    const snapshot = snapshotOf(frame)
    const unsafeSnapshot = {
      ...snapshot,
      state: { ...snapshot.state, baseRow: Number.MAX_SAFE_INTEGER },
    }
    expect(() => captureAcceptedScreen(unsafeSnapshot, DEFAULT_SNAPSHOT)).toThrow(
      'JSON safe-integer range',
    )
  })

  it('bounds a capture to the backend retained-screen row limit', () => {
    const frame = frameOf(10, blankRows([['T', 1, true]]), { x: 0, y: 0, visible: false })
    const snapshot = snapshotOf(frame)
    const oversizedViewport = {
      ...snapshot,
      geometry: { ...snapshot.geometry, rows: 10001 },
    }
    expect(() =>
      captureAcceptedScreen(oversizedViewport, DEFAULT_SNAPSHOT, { start: 0, end: 10001 }),
    ).toThrow('10000-row/JSON-safe limit')
  })

  it('keeps alternate-buffer row zero and accepts only rows in the frame', () => {
    const frame = frameOf(
      8,
      blankRows([['T', 1, true]]),
      { x: 1, y: 0, visible: false },
      {
        cols: 2,
        rows: 1,
      },
      {
        bufferKind: 'alternate',
        bufferInstance: '9007199254740993',
        baseRow: 0,
        bracketedPaste: false,
        mouseTracking: false,
        title: null,
        cwd: null,
      },
    )
    const snapshot = snapshotOf(frame)

    const captured = captureAcceptedScreen(snapshot, DEFAULT_SNAPSHOT, { start: 0, end: 1 })
    const provenance = liveProvenance(captured)
    expect(provenance.range).toEqual({ start: 0, end: 1 })
    expect(provenance.identity.buffer).toEqual({
      kind: 'alternate',
      altSession: '9007199254740993',
    })
    expect(captured.cursor).toEqual({ line: 0, col: 1 })
    expect(() => captureAcceptedScreen(snapshot, DEFAULT_SNAPSHOT, { start: 1, end: 2 })).toThrow(
      'outside accepted viewport [0, 1)',
    )
  })
})
