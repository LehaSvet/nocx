/**
 * A running block's own rows are ONE body, at ONE row pitch (nocx-zg3k3.14.26).
 *
 * The owner saw this twice on the stand while a command ran, and described it
 * twice: the running block's background ended after one row of its output and
 * the rest was painted below it, with an extra half-row of space exactly at
 * that boundary; then, with plain `seq`, the same extra half-row at the same
 * place with no block edge in view.
 *
 * The rows are painted in two boxes, and both are the command's body. The block
 * paints the rows the durable record has already taken — the ones that left the
 * screen — into its own `.cmd-output`; the sibling live region below paints the
 * screen the command is still writing (ADR-0078's two tiers, and the mockup
 * decision's rule that live output stays the controller-owned sibling). Each
 * box used to wear the WHOLE body padding, so the joint between them — between
 * two consecutive rows of one body — carried both: 6px + 2px = 8px.
 *
 * Measured on this spec's own scenario before the fix, at the default pane:
 * `{"blockRows":30,"blockPitch":17,"blockTail":6,"jointGap":8,"jointStep":25}` —
 * a 25px step where every other row pair steps 17px, and 6px of bare block body
 * under the block's last row.
 *
 * The scenario is the owner's own: sixty rows of `printf`, then a wait long
 * enough to look at them.
 */

import type { Page } from '@playwright/test'
import { test, expect, promptReady, clickIntoEditor } from './harness'

const SCENARIO =
  'w=$(tput cols); for i in $(seq 1 60); do printf \'%*s|\\n\' $((w-1)) "$i"; done; sleep 60'

interface Geometry {
  /** How many rows the block paints in its own body: the ones the durable
   *  record has taken, i.e. the rows that left the screen. */
  blockRows: number
  /** The pitch of those rows, from their own boxes. */
  blockPitch: number
  /** Space between the block's last own row and the block's own box end. */
  blockTail: number
  /** Space between the block's last own row and the live grid's first row. */
  jointGap: number
  /** The same joint as a distance between row TOPS: equal to `blockPitch`
   *  when the two boxes are one body at one pitch. */
  jointStep: number | null
  /** The row either side of the joint, so a failure says which rows. */
  lastBlockRow: string
  firstLiveRow: string
}

/** Read inside an animation frame, after the painter's own callback for it:
 *  a frame's rows are painted there while its layout is applied when the
 *  frame is accepted, and a read between the two sees a screen nobody does. */
async function geometry(page: Page): Promise<Geometry> {
  return page.evaluate(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
    const pane = document.querySelector<HTMLElement>('.pane.active')!
    const running = pane.querySelector<HTMLElement>('.cmd-block.cmd-block-running')
    const body = running?.querySelector<HTMLElement>(':scope > .cmd-output') ?? null
    const grid = pane.querySelector<HTMLElement>('.terminal-live-grid')
    if (!running || !grid) throw new Error('no running block or live grid')
    const bodyRows = [...(body?.querySelectorAll<HTMLElement>('.term-grid-row') ?? [])]
    const liveRows = [...grid.querySelectorAll<HTMLElement>('.term-grid-row')]
    const box = (el: Element): DOMRect => el.getBoundingClientRect()
    const text = (el: Element | undefined): string =>
      (el?.textContent ?? '').trim().replace(/\s+/g, ' ')
    const lastBody = bodyRows[bodyRows.length - 1]
    const firstLive = liveRows[0]
    return {
      blockRows: bodyRows.length,
      blockPitch: bodyRows.length > 1 ? box(bodyRows[1]).top - box(bodyRows[0]).top : 0,
      blockTail: lastBody ? +(box(running).bottom - box(lastBody).bottom).toFixed(2) : 0,
      jointGap: lastBody && firstLive ? +(box(firstLive).top - box(lastBody).bottom).toFixed(2) : 0,
      jointStep:
        lastBody && firstLive ? +(box(firstLive).top - box(lastBody).top).toFixed(2) : null,
      lastBlockRow: text(lastBody),
      firstLiveRow: text(firstLive),
    }
  })
}

async function run(page: Page, command: string): Promise<void> {
  await clickIntoEditor(page)
  await page.keyboard.type(command)
  await page.keyboard.press('Enter')
}

test.describe('the body of a running block that has outgrown its screen', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
    await promptReady(page)
  })

  test('keeps one row pitch across the rows the record holds and the rows still live', async ({
    page,
  }) => {
    await run(page, SCENARIO)
    // The state this is about: the command has printed past the screen, so the
    // block paints rows of its own body and the live region paints the rest.
    // Waited on the state, never on a duration.
    await expect
      .poll(async () => (await geometry(page)).blockRows, { timeout: 20_000 })
      .toBeGreaterThan(0)
    const g = await geometry(page)
    const said = `running block body: ${JSON.stringify(g)}`
    // The block's box ends at its last own row: a body that continues into the
    // live region does not pay the body's bottom padding there.
    expect(Math.abs(g.blockTail), said).toBeLessThan(0.5)
    // ... and the live region continues that body, so it pays no top padding
    // either: the two rows either side of the joint are consecutive rows of one
    // body, exactly one pitch apart, as every other pair of rows is.
    expect(Math.abs(g.jointGap), said).toBeLessThan(0.5)
    expect(g.jointStep, said).not.toBeNull()
    expect(Math.abs(g.jointStep! - g.blockPitch), said).toBeLessThan(0.5)
    await page.keyboard.press('Control+C')
  })

  test('still pays the body top padding once when the block has no rows of its own yet', async ({
    page,
  }) => {
    // The paired half, so the fix cannot be "the live region never pads": a
    // command that has printed nothing past its own screen has its whole body
    // in the live region, and that body's first row sits below the header by
    // the body's own top padding — the same offset the frozen body it becomes
    // has (`.cmd-output`'s own pad, which is where the number lives).
    await run(page, "printf 'FIRST\\n'; sleep 60")
    await expect(page.locator('.pane.active .terminal-live-grid')).toContainText('FIRST', {
      timeout: 20_000,
    })
    const g = await geometry(page)
    expect(g.blockRows, `block rows: ${JSON.stringify(g)}`).toBe(0)
    const top = await page.evaluate(async () => {
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
      const pane = document.querySelector<HTMLElement>('.pane.active')!
      const live = pane.querySelector<HTMLElement>('.terminal-live-container')!
      const grid = pane.querySelector<HTMLElement>('.terminal-live-grid')!
      // The first VISIBLE row: the echo row above the body's start is
      // translated out of the clip, so the first row a person sees is the
      // body's own first row.
      let clipTop = -Infinity
      for (let n: HTMLElement | null = grid.parentElement; n; n = n.parentElement) {
        if (getComputedStyle(n).overflowY !== 'visible') {
          clipTop = Math.max(clipTop, n.getBoundingClientRect().top)
        }
      }
      const first = [...grid.querySelectorAll<HTMLElement>('.term-grid-row')].find(
        (row) => row.getBoundingClientRect().top >= clipTop - 0.5,
      )
      return {
        padTop: parseFloat(getComputedStyle(live).paddingTop),
        token: parseFloat(
          getComputedStyle(document.documentElement).getPropertyValue('--cmd-output-pad-top'),
        ),
        offset: +(first!.getBoundingClientRect().top - live.getBoundingClientRect().top).toFixed(2),
      }
    })
    const said = `live body top: ${JSON.stringify(top)}`
    expect(top.padTop, said).toBe(top.token)
    expect(Math.abs(top.offset - top.padTop), said).toBeLessThan(0.5)
    await page.keyboard.press('Control+C')
  })
})
