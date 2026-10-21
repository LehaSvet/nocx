// Procedural ink at the paint seam (nocx-1ebrs.15): what paintRow builds for
// box drawing and block elements, what it refuses to measure, and the two
// degrades back to the font glyph. The geometry itself is asserted in
// cell-ink.test.ts; this file asserts the painted DOM that geometry lands in —
// including the bead's regression, two vertically adjacent U+2502 rows whose
// painted bands must cover the shared edge.

// @vitest-environment jsdom

import { describe, it, expect } from 'vitest'
import { DEFAULT_SNAPSHOT, type TerminalSnapshot } from '../scrollback/serializer'
import type { RunMetric } from '../scrollback/run-geometry'
import { frameOf, snapshotOf, type CellSpec } from './fixtures'
import { fitCandidatesOf, paintRow } from './paint-row'
import { proceduralSvgOf } from './cell-ink'

const palette: TerminalSnapshot = DEFAULT_SNAPSHOT

/** A metric with the published pitch: cellWidth 8, row pitch 20. */
const metric: RunMetric = {
  cellWidth: 8,
  defaultSpacing: 0,
  pitch: 20,
  advanceOf: () => null,
}

const rowOf = (cells: CellSpec[]) => snapshotOf(frameOf(1, [cells])).rows[0]

const proceduralCellsOf = (row: HTMLElement): HTMLElement[] =>
  Array.from(row.querySelectorAll<HTMLElement>('.term-cell[data-procedural]'))

describe('paintRow procedural ink (nocx-1ebrs.15)', () => {
  it('paints a run of U+2502 cells as ONE procedural box spanning the run', () => {
    const el = paintRow(
      rowOf([
        ['│', 1, true],
        ['│', 1, true],
      ]),
      { metric, palette },
    )
    const boxes = proceduralCellsOf(el)
    expect(boxes).toHaveLength(1)
    const box = boxes[0]
    expect(box.dataset.cols).toBe('2')
    // Sized by the published vars, so a re-pitch moves the box with the
    // text around it and the SVG stretches with it.
    expect(box.style.width).toBe('calc(var(--term-cell-width, 1ch) * 2)')
    expect(box.style.height).toBe('var(--term-cell-height, 1.2em)')
    // The character stays reachable for assistive tech.
    expect(box.getAttribute('aria-label')).toBe('││')
    // No font glyph is composited: the run paints no text of its own.
    expect(el.textContent).not.toContain('│')
    const svg = box.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(svg?.getAttribute('viewBox')).toBe('0 0 16 20')
    expect(svg?.getAttribute('aria-hidden')).toBe('true')
    expect(svg?.querySelectorAll('rect').length).toBeGreaterThanOrEqual(1)
  })

  it('paints two vertically adjacent U+2502 rows whose bands cover the shared edge — no gap pixels', () => {
    // The bead's regression test, on the painted result: each row's band
    // reaches past the shared edge on the same columns, so the union covers
    // the boundary and the line reads continuous. The font glyph cannot pass
    // this: it is shorter than the row and leaves the edge unpainted.
    const first = paintRow(rowOf([['│', 1, true]]), { metric, palette })
    const second = paintRow(rowOf([['│', 1, true]]), { metric, palette })
    const bandOf = (row: HTMLElement): { x: string; y: number; h: number } | null => {
      const rects = Array.from(row.querySelectorAll('rect'))
      const vertical = rects.find((r) => {
        const y = Number(r.getAttribute('y'))
        const h = Number(r.getAttribute('height'))
        return y <= 0 && y + h >= 20
      })
      return vertical
        ? {
            x: vertical.getAttribute('x') ?? '',
            y: Number(vertical.getAttribute('y')),
            h: Number(vertical.getAttribute('height')),
          }
        : null
    }
    const top = bandOf(first)
    const bottom = bandOf(second)
    expect(top).not.toBeNull()
    expect(bottom).not.toBeNull()
    expect(top?.x).toBe(bottom?.x)
  })

  it('keeps ordinary text beside a procedural cell, and boxes only the procedural cell', () => {
    const el = paintRow(
      rowOf([
        ['a', 1, true],
        ['│', 1, true],
        ['b', 1, true],
      ]),
      { metric, palette },
    )
    expect(proceduralCellsOf(el)).toHaveLength(1)
    expect(el.textContent).toContain('a')
    expect(el.textContent).toContain('b')
    expect(el.textContent).not.toContain('│')
  })

  it('never candidates a procedural cell for font measurement', () => {
    const rows = snapshotOf(
      frameOf(1, [
        [
          ['│', 1, true],
          ['█', 1, true],
          ['a', 1, true],
        ],
      ]),
    ).rows
    expect(fitCandidatesOf(rows)).toEqual([
      { chars: 'a', width: 1, face: { bold: false, italic: false } },
    ])
  })

  it('degrades to the font glyph with no metric', () => {
    const el = paintRow(rowOf([['│', 1, true]]), { metric: null, palette })
    expect(proceduralCellsOf(el)).toHaveLength(0)
    expect(el.textContent).toContain('│')
  })

  it('degrades to the font glyph with no published pitch', () => {
    const el = paintRow(rowOf([['│', 1, true]]), {
      metric: { cellWidth: 8, defaultSpacing: 0, advanceOf: () => null },
      palette,
    })
    expect(proceduralCellsOf(el)).toHaveLength(0)
    expect(el.textContent).toContain('│')
  })
})

describe('proceduralSvgOf', () => {
  it('draws each cell of the run at its own offset in one coordinate space', () => {
    const svg = proceduralSvgOf('││', [1, 1], 8, 20)
    expect(svg.getAttribute('viewBox')).toBe('0 0 16 20')
    const xs = Array.from(svg.querySelectorAll('rect')).map((r) => r.getAttribute('x'))
    expect(xs).toEqual(['4', '12'])
  })

  it('paints shapes with the run’s own colour via currentColor', () => {
    const svg = proceduralSvgOf('█', [1], 8, 20)
    expect(svg.querySelector('rect')?.getAttribute('fill')).toBe('currentColor')
  })
})
