// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { createCellModel } from '../cell-model'
import { DEFAULT_SNAPSHOT } from '../scrollback/serializer'
import { frameOf, snapshotOf, type CellSpec } from './fixtures'
import { createCellPainter } from './painter'
import { installLiveSelectionGesture } from './selection-gesture'
import { paintStoredRows, type StoredBlockRows } from '../scrollback/block-rows'
import { wireRowOf } from './fixtures'

describe('live selection gesture', () => {
  it('copies cells from the pointerdown revision when frames advance during a drag', () => {
    const model = createCellModel()
    const root = document.createElement('div')
    const surface = document.createElement('div')
    root.append(surface)
    document.body.append(root)
    const first = frameOf(
      1,
      [
        [
          ['a', 1, true],
          ['漢', 2, true],
          ['', 3, false],
          ['b', 1, true],
        ],
      ],
      { x: 0, y: 0, visible: false },
      { cols: 4, rows: 1, cellWidthPx: 8, cellHeightPx: 20 },
    )
    const firstSnapshot = snapshotOf(first)
    model.apply(first)
    const painter = createCellPainter({ surface, palette: () => DEFAULT_SNAPSHOT })
    painter.apply(firstSnapshot)
    vi.spyOn(surface, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      right: 32,
      bottom: 20,
      width: 32,
      height: 20,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    const copy = vi.fn()
    const gesture = installLiveSelectionGesture({
      model,
      painter,
      surface,
      gestureRoot: root,
      surfaceId: 'live',
      copy,
    })
    const event = (type: string, x: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: x, clientY: 5 })
    root.dispatchEvent(event('pointerdown', 8))
    const next = frameOf(
      2,
      [
        [
          ['z', 1, true],
          ['z', 1, true],
          ['z', 1, true],
          ['z', 1, true],
        ],
      ],
      { x: 0, y: 0, visible: false },
      { cols: 4, rows: 1, cellWidthPx: 8, cellHeightPx: 20 },
    )
    model.apply(next)
    painter.apply(snapshotOf(next))
    window.dispatchEvent(event('pointermove', 24))
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()
    window.dispatchEvent(event('pointerup', 24))
    expect(copy).toHaveBeenCalledWith('漢b')
    gesture.dispose()
    painter.dispose()
    root.remove()
  })

  it('a click without a drag selects nothing and leaves the clipboard alone', () => {
    const model = createCellModel()
    const root = document.createElement('div')
    const surface = document.createElement('div')
    root.append(surface)
    document.body.append(root)
    const frame = frameOf(
      1,
      [
        [
          ['a', 1, true],
          ['b', 1, true],
          ['c', 1, true],
          ['d', 1, true],
        ],
      ],
      { x: 0, y: 0, visible: false },
      { cols: 4, rows: 1, cellWidthPx: 8, cellHeightPx: 20 },
    )
    model.apply(frame)
    const painter = createCellPainter({ surface, palette: () => DEFAULT_SNAPSHOT })
    painter.apply(snapshotOf(frame))
    vi.spyOn(surface, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      right: 32,
      bottom: 20,
      width: 32,
      height: 20,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    const copy = vi.fn()
    const gesture = installLiveSelectionGesture({
      model,
      painter,
      surface,
      gestureRoot: root,
      surfaceId: 'live',
      copy,
    })
    const event = (type: string, x: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: x, clientY: 5 })
    // Focusing the terminal is a click on whatever cell is under the pointer.
    root.dispatchEvent(event('pointerdown', 12))
    window.dispatchEvent(event('pointerup', 12))
    expect(copy).not.toHaveBeenCalled()
    expect(surface.querySelector('.term-grid-selection')).toBeNull()
    gesture.dispose()
    painter.dispose()
    root.remove()
  })

  it('a double click selects the word under the pointer and copies it', () => {
    const model = createCellModel()
    const root = document.createElement('div')
    const surface = document.createElement('div')
    root.append(surface)
    document.body.append(root)
    const text = 'ls ~/a-b.txt (x)'
    const frame = frameOf(
      1,
      [[...text].map((ch) => (ch === ' ' ? ['', 1, false] : [ch, 1, true]) as CellSpec)],
      { x: 0, y: 0, visible: false },
      { cols: text.length, rows: 1, cellWidthPx: 8, cellHeightPx: 20 },
    )
    model.apply(frame)
    const painter = createCellPainter({ surface, palette: () => DEFAULT_SNAPSHOT })
    painter.apply(snapshotOf(frame))
    vi.spyOn(surface, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      right: text.length * 8,
      bottom: 20,
      width: text.length * 8,
      height: 20,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    const copy = vi.fn()
    const gesture = installLiveSelectionGesture({
      model,
      painter,
      surface,
      gestureRoot: root,
      surfaceId: 'live',
      copy,
    })
    // Column 6 is inside "~/a-b.txt": a path is one word, as in the frozen
    // block's rule (word-selection.ts) — '-', '.', '/' do not separate.
    const at = (type: string, detail: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: 6 * 8 + 2, clientY: 5, detail })
    root.dispatchEvent(at('pointerdown', 1))
    window.dispatchEvent(at('pointerup', 1))
    root.dispatchEvent(at('pointerdown', 2))
    window.dispatchEvent(at('pointerup', 2))
    root.dispatchEvent(at('dblclick', 2))
    expect(copy).toHaveBeenCalledWith('~/a-b.txt')
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()
    gesture.dispose()
    painter.dispose()
    root.remove()
  })

  it("the grid keeps its own rows while they are a running block's body (ADR-0091)", () => {
    // ADR-0091 seats the live region INSIDE the running block, so a live row
    // has a `.cmd-block` ancestor — and this gesture refused anything with one,
    // as "a plain command block's selectable text, never a guessed live-grid
    // cell". A row of the grid is a row of the grid wherever it is seated: the
    // press, the drag and the word all resolve through the cell snapshot. With
    // the refusal standing, a double click in a running program copied one DOM
    // run of cells — `dbl` — instead of the word
    // (e2e/clipboard.spec.ts).
    const model = createCellModel()
    const root = document.createElement('div')
    const surface = document.createElement('div')
    root.append(surface)
    // The arrangement ADR-0091 creates: block > live region > grid surface.
    const block = document.createElement('div')
    block.className = 'cmd-block'
    const region = document.createElement('div')
    region.className = 'terminal-live-container'
    block.appendChild(region)
    region.appendChild(root)
    document.body.append(block)
    const text = 'ls ~/a-b.txt (x)'
    const frame = frameOf(
      1,
      [[...text].map((ch) => (ch === ' ' ? ['', 1, false] : [ch, 1, true]) as CellSpec)],
      { x: 0, y: 0, visible: false },
      { cols: text.length, rows: 1, cellWidthPx: 8, cellHeightPx: 20 },
    )
    model.apply(frame)
    const painter = createCellPainter({ surface, palette: () => DEFAULT_SNAPSHOT })
    painter.apply(snapshotOf(frame))
    vi.spyOn(surface, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      right: text.length * 8,
      bottom: 20,
      width: text.length * 8,
      height: 20,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    const copy = vi.fn()
    const gesture = installLiveSelectionGesture({
      model,
      painter,
      surface,
      gestureRoot: root,
      surfaceId: 'live',
      copy,
    })
    // Dispatched ON the surface: the target the handlers see is a node inside
    // the live region, which is the whole question.
    const at = (type: string, detail: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: 6 * 8 + 2, clientY: 5, detail })
    surface.dispatchEvent(at('pointerdown', 1))
    window.dispatchEvent(at('pointerup', 1))
    surface.dispatchEvent(at('pointerdown', 2))
    window.dispatchEvent(at('pointerup', 2))
    surface.dispatchEvent(at('dblclick', 2))
    expect(copy).toHaveBeenCalledWith('~/a-b.txt')
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()
    gesture.dispose()
    painter.dispose()
    block.remove()
  })

  it('copies a drag from live output into a card in display order using stored rows', () => {
    const model = createCellModel()
    const root = document.createElement('div')
    const surface = document.createElement('div')
    root.append(surface)
    const card = document.createElement('article')
    card.dataset.entryId = 'block-7'
    root.append(card)
    document.body.append(root)
    const live = frameOf(
      1,
      [
        [
          ['x', 1, true],
          ['y', 1, true],
          ['z', 1, true],
          ['w', 1, true],
        ],
      ],
      { x: 0, y: 0, visible: false },
      { cols: 4, rows: 1, cellWidthPx: 8, cellHeightPx: 20 },
    )
    model.apply(live)
    const painter = createCellPainter({ surface, palette: () => DEFAULT_SNAPSHOT })
    painter.apply(snapshotOf(live))
    vi.spyOn(surface, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      right: 32,
      bottom: 20,
      width: 32,
      height: 20,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    const rows: StoredBlockRows = {
      lines: [
        {
          from: 12,
          row: wireRowOf(
            [
              ['a', 1, true],
              ['漢', 2, true],
              ['', 3, false],
            ],
            true,
          ),
        },
        { from: 13, row: wireRowOf([['b', 1, true]]) },
        { from: 14, row: wireRowOf([['c', 1, true]]) },
      ],
      artifactVersion: 'artifact-v4',
      droppedRows: 0,
      lostRows: 0,
      truncated: null,
    }
    paintStoredRows(card, rows, { metric: null, palette: DEFAULT_SNAPSHOT })
    const originalRow = card.querySelector<HTMLElement>('.term-grid-row')!
    const cardRow = originalRow.cloneNode(true) as HTMLElement
    cardRow.textContent = 'DOM MUST NOT BE COPIED'
    originalRow.replaceWith(cardRow)
    const output = card.querySelector('.cmd-output')!
    output.insertBefore(output.lastElementChild!, cardRow)
    vi.spyOn(cardRow, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 30,
      right: 24,
      bottom: 50,
      width: 24,
      height: 20,
      x: 0,
      y: 30,
      toJSON: () => ({}),
    })
    const copy = vi.fn()
    const gesture = installLiveSelectionGesture({
      model,
      painter,
      surface,
      gestureRoot: root,
      surfaceId: 'live',
      copy,
    })
    const event = (type: string, target: HTMLElement, x: number, y: number) => {
      const e = new MouseEvent(type, { bubbles: true, button: 0, clientX: x, clientY: y })
      Object.defineProperty(e, 'pointerId', { value: 1 })
      target.dispatchEvent(e)
    }
    event('pointerdown', surface, 1, 5)
    const next = frameOf(
      2,
      [
        [
          ['z', 1, true],
          ['z', 1, true],
          ['z', 1, true],
          ['z', 1, true],
        ],
      ],
      { x: 0, y: 0, visible: false },
      { cols: 4, rows: 1, cellWidthPx: 8, cellHeightPx: 20 },
    )
    model.apply(next)
    painter.apply(snapshotOf(next))
    event('pointermove', cardRow, 8, 35)
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()
    event('pointerup', cardRow, 8, 35)
    expect(copy).toHaveBeenCalledWith('漢b\nc\nx')
    expect(surface.querySelector('.term-grid-row')?.textContent).toBe('zzzz')
    gesture.dispose()

    // Reverse the drag. Display ordering, not drag direction, defines copy.
    const reverseCopy = vi.fn()
    const reverse = installLiveSelectionGesture({
      model,
      painter,
      surface,
      gestureRoot: root,
      surfaceId: 'live',
      copy: reverseCopy,
    })
    event('pointerdown', cardRow, 8, 35)
    event('pointermove', surface, 1, 5)
    event('pointerup', surface, 1, 5)
    expect(reverseCopy).toHaveBeenCalledWith('漢b\nc\nz')
    reverse.dispose()
    painter.dispose()
    root.remove()
  })
  // THE PERSON'S DRAG IS THEIRS, WHETHER OR NOT THE PROGRAM REPORTS THE MOUSE
  // (nocx-zg3k3.15.5). This gesture used to stand down for a reporting program,
  // and what owned the drag instead was the BROWSER's own selection over the
  // painter's rows — a second claimant on one input, cleared at the release
  // when the pane returned the keyboard to its hidden input, so the highlight a
  // person had just made disappeared as they let go. The pointer sequence
  // reaches the program through the input renderer, which this gesture never
  // sits in front of, so standing down bought the program nothing.
  it('a drag selects while the program reports the mouse, and a click still does not', () => {
    const model = createCellModel()
    const root = document.createElement('div')
    const surface = document.createElement('div')
    root.append(surface)
    document.body.append(root)
    const frame = frameOf(
      1,
      [
        [
          ['a', 1, true],
          ['b', 1, true],
          ['c', 1, true],
          ['d', 1, true],
        ],
      ],
      { x: 0, y: 0, visible: false },
      { cols: 4, rows: 1, cellWidthPx: 8, cellHeightPx: 20 },
    )
    model.apply(frame)
    const painter = createCellPainter({ surface, palette: () => DEFAULT_SNAPSHOT })
    painter.apply(snapshotOf(frame))
    vi.spyOn(surface, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      right: 32,
      bottom: 20,
      width: 32,
      height: 20,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    const copy = vi.fn()
    const gesture = installLiveSelectionGesture({
      model,
      painter,
      surface,
      gestureRoot: root,
      surfaceId: 'live',
      mouseReportingActive: () => true,
      copy,
    })
    const event = (type: string, x: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: x, clientY: 5 })
    root.dispatchEvent(event('pointerdown', 1))
    window.dispatchEvent(event('pointermove', 28))
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()
    window.dispatchEvent(event('pointerup', 28))
    expect(copy).toHaveBeenCalledWith('abcd')
    // Still painted once the button is up, and still painted after a frame
    // arrives: the model holds it, not the engine.
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()
    painter.apply(snapshotOf(frame))
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()

    // A click on one cell is still a click — how a person gives the terminal
    // focus, and what a reporting program's buttons are pressed with.
    copy.mockClear()
    root.dispatchEvent(event('pointerdown', 12))
    window.dispatchEvent(event('pointerup', 12))
    expect(copy).not.toHaveBeenCalled()
    expect(surface.querySelector('.term-grid-selection')).toBeNull()
    gesture.dispose()
    painter.dispose()
    root.remove()
  })
})

/** A pane, a painter and a gesture over one two-row frame — the shape every
 *  case below needs, so each test is about the rule rather than the wiring. */
function liveGesture(): {
  gesture: ReturnType<typeof installLiveSelectionGesture>
  painter: ReturnType<typeof createCellPainter>
  surface: HTMLElement
  model: ReturnType<typeof createCellModel>
  root: HTMLElement
  copy: ReturnType<typeof vi.fn>
  next: (rows: CellSpec[][]) => ReturnType<typeof snapshotOf>
} {
  const model = createCellModel()
  const root = document.createElement('div')
  const surface = document.createElement('div')
  root.append(surface)
  document.body.append(root)
  const first = frameOf(
    1,
    [
      [
        ['a', 1, true],
        ['b', 1, true],
        ['c', 1, true],
        ['d', 1, true],
      ],
      [
        ['e', 1, true],
        ['f', 1, true],
        ['g', 1, true],
        ['h', 1, true],
      ],
    ],
    { x: 0, y: 0, visible: false },
    { cols: 4, rows: 2, cellWidthPx: 8, cellHeightPx: 20 },
  )
  model.apply(first)
  const painter = createCellPainter({ surface, palette: () => DEFAULT_SNAPSHOT })
  painter.apply(snapshotOf(first))
  vi.spyOn(surface, 'getBoundingClientRect').mockReturnValue({
    left: 0,
    top: 0,
    right: 32,
    bottom: 40,
    width: 32,
    height: 40,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  })
  const copy = vi.fn()
  const gesture = installLiveSelectionGesture({
    model,
    painter,
    surface,
    gestureRoot: root,
    surfaceId: 'live',
    copy,
  })
  const next = (rows: CellSpec[][]): ReturnType<typeof snapshotOf> =>
    snapshotOf(
      frameOf(
        2,
        rows,
        { x: 0, y: 0, visible: false },
        {
          cols: 4,
          rows: rows.length,
          cellWidthPx: 8,
          cellHeightPx: 20,
        },
      ),
    )
  return { gesture, painter, surface, model, root, copy, next }
}

const DRAG: CellSpec[] = [
  ['e', 1, true],
  ['f', 1, true],
  ['g', 1, true],
  ['h', 1, true],
]

describe('a live selection has the lifetime of the text under it (nocx-1ebrs.19)', () => {
  it('ends when the frame rewrites the cells it covers', () => {
    // The program repainted the row the selection names: the highlight must go,
    // because it is the copy source and the text it named is no longer there.
    const { gesture, painter, surface, root, next } = liveGesture()
    const press = (type: string, x: number, y: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: x, clientY: y })
    root.dispatchEvent(press('pointerdown', 8, 5))
    window.dispatchEvent(press('pointermove', 16, 5))
    window.dispatchEvent(press('pointerup', 16, 5))
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()

    gesture.screenChanged(next([DRAG, DRAG]))
    expect(surface.querySelector('.term-grid-selection')).toBeNull()
    gesture.dispose()
    painter.dispose()
    root.remove()
  })

  it('keeps a selection a frame outside the range left alone', () => {
    // Output BELOW the selected rows is the ordinary case in a terminal, and
    // dropping the highlight for it would make a selection impossible to read
    // while a program is printing.
    const { gesture, painter, surface, root, next } = liveGesture()
    const press = (type: string, x: number, y: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: x, clientY: y })
    root.dispatchEvent(press('pointerdown', 8, 5))
    window.dispatchEvent(press('pointermove', 16, 5))
    window.dispatchEvent(press('pointerup', 16, 5))
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()

    gesture.screenChanged(
      next([
        [
          ['a', 1, true],
          ['b', 1, true],
          ['c', 1, true],
          ['d', 1, true],
        ],
        DRAG,
      ]),
    )
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()
    gesture.dispose()
    painter.dispose()
    root.remove()
  })

  it('a press and release on one cell leaves no selection and clears one that exists', () => {
    // THE OWNER'S CRITERION (nocx-1ebrs.19, 2026-10-20): a click without a
    // drag selects nothing - and clears whatever a drag left behind. The press
    // paints nothing and the same-cell release clears, so the character under
    // the pointer must never become a one-cell selection.
    const { gesture, painter, surface, root } = liveGesture()
    const press = (type: string, x: number, y: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: x, clientY: y })
    root.dispatchEvent(press('pointerdown', 8, 5))
    // No move: the press itself must not have painted a one-cell selection.
    expect(surface.querySelector('.term-grid-selection')).toBeNull()
    window.dispatchEvent(press('pointerup', 8, 5))
    expect(surface.querySelector('.term-grid-selection')).toBeNull()

    // A drag first, so a selection EXISTS; the next plain click must clear it.
    root.dispatchEvent(press('pointerdown', 8, 5))
    window.dispatchEvent(press('pointermove', 16, 5))
    window.dispatchEvent(press('pointerup', 16, 5))
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()

    root.dispatchEvent(press('pointerdown', 24, 5))
    expect(surface.querySelector('.term-grid-selection')).toBeNull()
    window.dispatchEvent(press('pointerup', 24, 5))
    expect(surface.querySelector('.term-grid-selection')).toBeNull()
    gesture.dispose()
    painter.dispose()
    root.remove()
  })

  it('the copy chord copies a settled selection and clears it', () => {
    // THE OWNER'S DECISION (nocx-1ebrs.19.1, 2026-10-20): with a visible
    // live-grid selection the copy chord copies it, and WITH NONE the chord
    // keeps the interrupt. The decision says what the copy leaves behind in as
    // many words — "the chord, when it comes, copies the same selection AND
    // CLEARS IT" — and this asserts exactly that: a person who asked to copy
    // sees that the copy happened, and the highlight they asked about is the
    // thing that goes.
    //
    // WHERE THE CHORD IS CLAIMED is the pane's question, not this gesture's
    // (`terminal-content.ts`, in capture, beside the target chord): whether a
    // keystroke is this pane's to answer depends on which surface holds the
    // keyboard, whether an overlay is up and whether the composer is on
    // screen. This file owns the state and the copy.
    const { gesture, painter, surface, root, copy } = liveGesture()
    const press = (type: string, x: number, y: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: x, clientY: y })
    root.dispatchEvent(press('pointerdown', 8, 5))
    window.dispatchEvent(press('pointermove', 16, 5))
    window.dispatchEvent(press('pointerup', 16, 5))
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()
    copy.mockClear()

    // Copy-on-select wrote the text at the release; the chord writes the same
    // value again — a second way to ask, never a second copy path.
    expect(gesture.copyChord()).toBe(true)
    expect(copy).toHaveBeenCalledWith(expect.stringContaining('bc'))
    expect(surface.querySelector('.term-grid-selection')).toBeNull()

    // AND WITH NOTHING SELECTED IT CHANGES NOTHING, which is what lets the
    // pane leave every other meaning of the chord alone.
    expect(gesture.copyChord()).toBe(false)
    expect(copy).toHaveBeenCalledTimes(1)
    gesture.dispose()
    painter.dispose()
    root.remove()
  })

  it('the copy chord refuses a drag still under the pointer', () => {
    // A drag is not a selection yet: its range is not final and its RELEASE is
    // what copies it. Clearing here would drop the anchor too, and the release
    // that follows would copy nothing — the failure e2e/live-to-card-joined
    // measured for `dismiss` (nocx-1ebrs.19). So the chord answers false and
    // the drag goes on being a drag.
    const { gesture, painter, surface, root, copy } = liveGesture()
    const press = (type: string, x: number, y: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: x, clientY: y })
    root.dispatchEvent(press('pointerdown', 8, 5))
    window.dispatchEvent(press('pointermove', 16, 5))
    copy.mockClear()

    expect(gesture.copyChord()).toBe(false)
    expect(copy).not.toHaveBeenCalled()
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()

    // And the drag still ends where a drag ends: the release copies.
    window.dispatchEvent(press('pointerup', 16, 5))
    expect(copy).toHaveBeenCalledWith(expect.stringContaining('bc'))
    gesture.dispose()
    painter.dispose()
    root.remove()
  })

  it('ends when a key is sent to the program', () => {
    const { gesture, painter, surface, root } = liveGesture()
    const press = (type: string, x: number, y: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: x, clientY: y })
    root.dispatchEvent(press('pointerdown', 8, 5))
    window.dispatchEvent(press('pointermove', 16, 5))
    window.dispatchEvent(press('pointerup', 16, 5))
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()

    gesture.dismiss()
    expect(surface.querySelector('.term-grid-selection')).toBeNull()
    gesture.dispose()
    painter.dispose()
    root.remove()
  })

  it('survives a key typed while the drag is still being made, and copies at the release', () => {
    // A DRAG IN PROGRESS IS NOT A SELECTION YET (nocx-1ebrs.19). The person is
    // still holding the button, so the range is not final and there is nothing
    // to copy until it is. `dismiss()` answers the next key SENT TO THE
    // PROGRAM, and the release is where the text reaches the clipboard — here,
    // between the two, so it is neither the key nor the frame that ends the
    // gesture.
    //
    // Measured red: the pane calls `dismiss()` for every key it forwards, and a
    // drag in progress cleared with it — `clear()` drops the ANCHOR too, so the
    // release found no gesture and copied nothing. In the browser that is
    // e2e/live-to-card-joined.spec.ts, which types into a running `cat` during
    // the drag to force a frame; the clipboard still held the text written
    // before the drag, so the person's selection was lost unread.
    const { gesture, painter, surface, root, copy, next } = liveGesture()
    const press = (type: string, x: number, y: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: x, clientY: y })
    root.dispatchEvent(press('pointerdown', 8, 5))
    window.dispatchEvent(press('pointermove', 16, 5))

    gesture.dismiss()
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()

    // AND A FRAME THAT REPAINTS THE CELLS UNDER A LIVE DRAG DOES NOT END IT
    // EITHER: the next pointer event redraws from the capture, so clearing here
    // would only make the highlight flicker and take the anchor with it.
    gesture.screenChanged(next([DRAG, DRAG]))
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()

    window.dispatchEvent(press('pointerup', 16, 5))
    expect(copy).toHaveBeenCalledWith('bc')
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()
    gesture.dispose()
    painter.dispose()
    root.remove()
  })

  it('compares a settled selection against the snapshot it was captured on', () => {
    // THE COPY SOURCE IS THE CAPTURE, so the comparison basis is too
    // (nocx-1ebrs.19). Frames that land DURING the drag advance the model; the
    // selection's text and its "has anything it names changed" question both
    // still answer to the revision the person was looking at when they pressed.
    // Measured red before: the release refreshed the basis to the newest frame,
    // so a screen that came back to the captured text was reported as a change.
    const { gesture, painter, model, surface, root } = liveGesture()
    const press = (type: string, x: number, y: number) =>
      new MouseEvent(type, { bubbles: true, button: 0, clientX: x, clientY: y })
    root.dispatchEvent(press('pointerdown', 8, 5))
    window.dispatchEvent(press('pointermove', 16, 5))
    // A frame arrives mid-drag and rewrites the selected row, so the model's
    // current revision is no longer the one the person pressed on.
    const repainted = frameOf(2, [DRAG, DRAG])
    model.apply(repainted)
    painter.apply(snapshotOf(repainted))
    window.dispatchEvent(press('pointerup', 16, 5))
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()

    // The screen the person selected from comes back, cell for cell: nothing
    // the selection names has changed, so the highlight stays.
    gesture.screenChanged(
      snapshotOf(
        frameOf(
          3,
          [
            [
              ['a', 1, true],
              ['b', 1, true],
              ['c', 1, true],
              ['d', 1, true],
            ],
            [
              ['e', 1, true],
              ['f', 1, true],
              ['g', 1, true],
              ['h', 1, true],
            ],
          ],
          { x: 0, y: 0, visible: false },
          { cols: 4, rows: 2, cellWidthPx: 8, cellHeightPx: 20 },
        ),
      ),
    )
    expect(surface.querySelector('.term-grid-selection')).not.toBeNull()
    gesture.dispose()
    painter.dispose()
    root.remove()
  })
})
