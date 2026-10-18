/**
 * A drag over the live grid selects, inside a full-screen program that reports
 * the mouse (nocx-zg3k3.15.5).
 *
 * THE PERSON'S GESTURE, MEASURED. In a full-screen alternate-screen program
 * that enables mouse reporting — Claude Code, k9s, mc — dragging across the
 * output used to leave a selection on screen that stayed. A person dragging
 * there now watches a highlight appear and vanish at the release.
 *
 * The gesture the pane offers for the live grid is the painter's
 * (`.term-grid-selection`, from the cell snapshot), and the pane stood that
 * gesture down whenever the accepted frame said the program reports the mouse.
 * What was left owning the drag was the BROWSER's own selection over the
 * painter's rows: a second claimant on one input, painted by the engine and
 * not by nocx, and cleared again at the release when the pane returns the
 * keyboard to its hidden input. The painter never held it, so no frame
 * repainted it.
 *
 * The fixture is a faithful program, not a simulation of one: a real pty, raw
 * mode, the alternate screen, DECSET 1000/1002/1006, and a reader that paints
 * back every byte it is sent. So the same screen answers the other half of the
 * requirement by itself — the program is still sent the press, the drag motions
 * and the release, and it still owns the keyboard. Nothing here waits on a
 * duration: every wait is on a state the screen, the layout or the clipboard
 * shows.
 */
import { test, expect, promptReady } from './harness'
import type { Page } from './harness'
import { mkdtempSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { tmpdir } from 'node:os'

const LIVE = '.pane.active .terminal-live-grid'
const SELECTION = '.pane.active .term-grid-selection'
const LIVE_BOX = '.pane.active .terminal-live-container'

const nonce = Date.now().toString(36)
/** A word the row carries once, short enough to drag across whole. */
const MARKER = `SEL-TUI-${nonce}`

/** The clipboard needs the browser's, not the Wails runtime's (clipboard.ts). */
async function disableWailsRuntime(page: Page) {
  await page.addInitScript(() => {
    Object.defineProperty(window, 'runtime', {
      get() {
        return undefined
      },
      set(_value: unknown) {
        void _value
      },
      configurable: true,
      enumerable: true,
    })
  })
}

interface Target {
  readonly row: number
  readonly first: number
  readonly last: number
  readonly x0: number
  readonly y: number
  readonly cellWidth: number
  /** The first selected cell's own offset inside the grid. */
  readonly offsetX: number
  readonly offsetY: number
}

/**
 * Where the marker's first and last cells are, in page coordinates.
 *
 * The ROW AND COLUMN are the pane's own screen reading (`__nocxPaneScreen`), and
 * the pixels are its own geometry seam (`__nocxPaneCellToPixel`) plus the grid's
 * own box — no second geometry in the spec, and no DOM text, which is not frame
 * authority (terminal-content's seam is installed for exactly this).
 */
async function markerTarget(page: Page, marker: string): Promise<Target> {
  const target = await page.evaluate(
    ({ marker, live }) => {
      const host = window as unknown as {
        __nocxPaneScreen?: () => { rows?: string[] } | null
        __nocxPaneCellToPixel?: (col: number, row: number) => { x: number; y: number } | null
      }
      const rows = host.__nocxPaneScreen?.()?.rows ?? []
      const row = rows.findIndex((text) => text.includes(marker))
      if (row < 0) return null
      const first = rows[row]!.indexOf(marker)
      const last = first + marker.length - 1
      const start = host.__nocxPaneCellToPixel?.(first, row) ?? null
      const end = host.__nocxPaneCellToPixel?.(last, row) ?? null
      const next = host.__nocxPaneCellToPixel?.(first + 1, row) ?? null
      const grid = document.querySelector<HTMLElement>(live)
      if (!start || !end || !next || !grid) return null
      const rect = grid.getBoundingClientRect()
      return {
        row,
        first,
        last,
        x0: rect.left + start.x + 1,
        y: rect.top + start.y + 4,
        cellWidth: next.x - start.x,
        offsetX: start.x,
        offsetY: start.y,
      }
    },
    { marker, live: LIVE },
  )
  if (target === null) throw new Error('the marker is not on a painted live row')
  return target as Target
}

/** The painted selection's own claim: which cells, and where. */
async function paintedSelection(page: Page) {
  return page.evaluate(
    ({ sel, live }) => {
      const nodes = [...document.querySelectorAll<HTMLElement>(sel)]
      const grid = document.querySelector<HTMLElement>(live)
      const gridRect = grid?.getBoundingClientRect() ?? null
      return nodes.map((node) => {
        const rect = node.getBoundingClientRect()
        return {
          row: Number(node.dataset.row ?? '-1'),
          start: Number(node.dataset.start ?? '-1'),
          end: Number(node.dataset.end ?? '-1'),
          left: gridRect ? rect.left - gridRect.left : Number.NaN,
          top: gridRect ? rect.top - gridRect.top : Number.NaN,
          width: rect.width,
          height: rect.height,
        }
      })
    },
    { sel: SELECTION, live: LIVE },
  )
}

/** Start the program, wait for its screen to be STILL, and drag across the
 *  marker. Returns the grid locator and the target the drag aimed at. */
async function dragAcrossTheMarker(page: Page) {
  const dir = mkdtempSync(join(tmpdir(), 'nocx-d3-fullscreen-'))
  const script = join(dir, 'mouse-reporting-job.sh')
  writeFileSync(
    script,
    [
      '#!/bin/sh',
      // A real TUI's own terms: no echo, no line discipline, its buffer and
      // its mouse modes — the modes the runtime reads off the accepted frame.
      'stty raw -echo',
      `printf '\\033[?1049h'`,
      `printf '\\033[?1000h\\033[?1002h\\033[?1006h'`,
      `printf '\\033[2J\\033[H'`,
      // The program's own rows start BELOW the pane's chrome: the pane-context
      // chip sits over the live box's first rows, and where it ends differs by
      // engine — measured on WebKit, where a point four pixels into painted row
      // 0 landed on .ui-pane-context and the drag reached neither the grid nor
      // the program. A drag has to start on the grid it is about.
      `printf '\\033[5;1H%s alpha bravo charlie\\r\\n' '${MARKER}'`,
      `printf 'a second row of the program\\r\\n'`,
      `printf '\\033[8;1H'`,
      // The reader: every byte the program is sent is painted back, so the
      // screen itself is the receipt.
      'exec cat -v',
      '',
    ].join('\n'),
  )

  await page.keyboard.type(`sh ${script}`)
  await page.keyboard.press('Enter')
  const grid = page.locator(LIVE)
  await expect(page.locator(`${LIVE} .term-grid-row`, { hasText: MARKER })).toHaveCount(1, {
    timeout: 20_000,
  })

  // AND ITS BOX HAS STOPPED MOVING. Entering the alternate screen moves the
  // live box — the block chrome steps aside and the box grows to the scroller —
  // so a target measured during that move names the row ABOVE the marker by the
  // time the press lands. Measured on WebKit: the press arrived one row high,
  // and the echo the program painted back said so. Both waits below are on
  // states the pane publishes, not on elapsed time.
  await expect(page.locator(LIVE_BOX)).toHaveClass(/live-fullscreen/)
  await expect
    .poll(() =>
      page.evaluate(
        () => document.querySelector('.pane.active .scrollback-inner')?.getAnimations().length ?? 0,
      ),
    )
    .toBe(0)

  expect(await paintedSelection(page), 'nothing is selected before the drag').toEqual([])

  const target = await markerTarget(page, MARKER)

  // ON THE GRID IT IS ABOUT. An overlay over the live box (the pane-context
  // chip, a builder strip) would take the press silently, and the spec would
  // then be asserting about a gesture nobody made.
  const onTheGrid = await page.evaluate(
    ({ live, x, y }) => {
      const grid = document.querySelector<HTMLElement>(live)
      const at = document.elementFromPoint(x, y)
      return grid !== null && at !== null && (at === grid || grid.contains(at))
    },
    { live: LIVE, x: target.x0, y: target.y },
  )
  expect(onTheGrid, 'the drag must start on the live grid, not on an overlay above it').toBe(true)

  // THE DRAG A PERSON MAKES: press on the marker's first cell, move across it
  // in steps, release on its last cell.
  const span = target.last - target.first
  await page.mouse.move(target.x0, target.y)
  await page.mouse.down()
  await page.mouse.move(target.x0 + target.cellWidth * (span / 2), target.y, { steps: 5 })
  await page.mouse.move(target.x0 + target.cellWidth * span, target.y, { steps: 5 })
  await page.mouse.up()
  return { grid, target }
}

test.describe('a full-screen program that reports the mouse', () => {
  test('a drag across its output leaves a selection on screen, and the program keeps its input', async ({
    page,
  }) => {
    await page.goto('/')
    await expect(page.locator('.nocx-tab')).toHaveCount(1)
    await promptReady(page)
    const { grid, target } = await dragAcrossTheMarker(page)

    // THE PROGRAM STILL RECEIVED THE WHOLE GESTURE, and it says so itself:
    // cat -v paints back the press, the drag motions (mode 1002) and the
    // release the runtime encoded for it. A selection that stole the gesture
    // would leave none of these on screen.
    await expect(grid).toContainText(`[<0;${target.first + 1};${target.row + 1}M`, {
      timeout: 10_000,
    })
    await expect(grid).toContainText('[<32;', { timeout: 10_000 })
    await expect(grid).toContainText(`[<0;${target.last + 1};${target.row + 1}m`, {
      timeout: 10_000,
    })

    // AND THE SELECTION IS STILL THERE, painted over exactly the cells the
    // person dragged across, through the repaint those echoes caused.
    const selected = await paintedSelection(page)
    expect(selected, 'the drag left one painted selection').toHaveLength(1)
    expect(selected[0]).toMatchObject({
      row: target.row,
      start: target.first,
      end: target.last + 1,
    })
    expect(selected[0]!.width).toBeCloseTo(target.cellWidth * MARKER.length, 0)
    expect(selected[0]!.left).toBeCloseTo(target.offsetX, 0)
    expect(selected[0]!.top).toBeCloseTo(target.offsetY, 0)
    expect(selected[0]!.height).toBeGreaterThan(0)

    // AND IT IS THE PANE'S SELECTION, not the engine's: nothing of the
    // browser's own selection is left over the painter's rows. That second
    // claimant is what the person's drag used to land on, and what died at the
    // release (nocx-zg3k3.15.5).
    expect(await page.evaluate(() => window.getSelection()?.toString() ?? '')).toBe('')

    // AND THE KEYBOARD IS STILL THE PROGRAM'S: the keys typed next reach it,
    // and it paints what it got. A gesture that had taken the input path with
    // it would have left these characters in the browser.
    await page.keyboard.type('zq7')
    await expect(grid).toContainText('zq7', { timeout: 10_000 })
    // The selection the person made is still on screen after that too.
    expect(await paintedSelection(page), 'the selection outlives the next keys').toHaveLength(1)

    // AND A CLICK IS STILL HOW THE KEYBOARD MOVES INTO THE PANE — in a
    // reporting program exactly as in any other, which is what a person does
    // before pressing the program's own keys.
    await page.locator('[aria-label="New tab"]').focus()
    await expect(page.locator('[aria-label="New tab"]')).toBeFocused()
    await page.mouse.click(target.x0, target.y)
    await page.keyboard.type('k4m')
    await expect(grid).toContainText('k4m', { timeout: 10_000 })
  })

  // Chromium reads the clipboard under Playwright; WebKit grants neither
  // clipboard-read nor clipboard-write (clipboard.spec.ts's own note).
  test.describe('copy-on-select', () => {
    test.skip(
      ({ browserName }) => browserName !== 'chromium',
      'clipboard-read permission is Chromium-only; WebKit must be checked manually',
    )

    test('the drag copies exactly the text it covered', async ({ page }) => {
      await disableWailsRuntime(page)
      await page.goto('/')
      await expect(page.locator('.nocx-tab')).toHaveCount(1)
      await promptReady(page)
      await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
      await dragAcrossTheMarker(page)
      await expect
        .poll(() => page.evaluate(() => navigator.clipboard.readText()), { timeout: 5_000 })
        .toBe(MARKER)
    })
  })
})
