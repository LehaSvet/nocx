/**
 * e2e: the "neither running nor finished" notice is a band of the pane
 * (nocx-xn63t.6.36).
 *
 * THE OWNER'S CRITERION, and this is it watched on the real product: on a
 * restored tab holding an unreconciled block, the notice's left and right
 * edges equal the pane's, and its bottom edge meets the top of what the pane
 * draws under it with a 0 px gap. Before this change the notice was a rounded
 * inset card: 8 px of the surface's own margin above it, and 32 px of the
 * kit's `margin-bottom` (`--space-8`) below it, which is the empty band the
 * owner photographed.
 *
 * # Which element is "the pane header"
 *
 * `.ui-pane-context` is the pane's own header strip, and the tree draws it
 * ONLY while an alt-screen program owns the pane — `TerminalContent`'s
 * `_syncPaneContext` sets `hidden` from `scrollback.mode !== 'fullscreen'`,
 * and e2e/visual-board.spec.ts asserts `toBeHidden()` for the ordinary
 * integrated pane. The state this spec has to produce — restored rows that
 * nobody could be asked about — has no live session at all, so no program
 * owns the pane and no header is drawn. What is drawn under the notice is the
 * pane's content. The rule is the same rule in both states, so the assertion
 * names the element that is actually under the notice and requires the gap to
 * be zero against it; when a header is drawn it is that element by
 * construction (the notice is inserted at the pane's first child, and the
 * strip is the next thing after it).
 *
 * # Why the session cannot be judged
 *
 * A coordinator replacement carries over every recorded session and judges
 * none of them at `Open` (nocx-k6p18.5). This spec ends the helper daemon
 * that holds the shell BEFORE the restart, so the ask has nowhere to go and
 * the verdict is `unknown` — the third state the notice exists for. The
 * helper's death is waited for on the OS's own answer, not on a duration
 * (the same `endDaemonsUnder` local-session-reattach.spec.ts uses).
 */
import { expect, type Page } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { mkdtempSync } from 'node:fs'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import {
  standalone as base,
  VaultBackend,
  bindEndpoint,
  promptReady,
  type DisposableRoot,
} from './harness'
import { readStand } from './stand'

/** Lazily: the stand is started by globalSetup, which runs after Playwright
 *  has collected this file. */
const serverBin = () => readStand().server

const TAB = '.nocx-tab'
const PANE = '.pane.active'
const INPUT = `${PANE} .nocx-editor-input`
const NOTICE = `${PANE} .nocx-unreconciled-notice`

const test = base

/** The program the pane runs, and it stays in the foreground: the entry it
 *  opens is what the next coordinator carries over. */
const STAYS_OPEN = 'sleep 600'

/** Sub-pixel tolerance. Layout is fractional — a device pixel ratio and
 *  three percentage columns put edges on halves — and "flush" is a claim
 *  about the edge the person sees, not about float equality. */
const FLUSH_PX = 0.5

/** The pids of this machine's helper daemons started under one home. */
function daemonsFor(isolatedHome: string): number[] {
  try {
    return execFileSync('pgrep', ['-f', join(isolatedHome, '.nocx', 'helper')], {
      encoding: 'utf8',
    })
      .split('\n')
      .map((line) => Number(line.trim()))
      .filter((pid) => Number.isInteger(pid) && pid > 0)
  } catch {
    // pgrep exits non-zero when nothing matches, which is an answer.
    return []
  }
}

/** Whether every helper daemon under a home has gone, bounded by a deadline. */
async function daemonsGone(isolatedHome: string, withinMs: number): Promise<boolean> {
  const deadline = Date.now() + withinMs
  for (;;) {
    if (daemonsFor(isolatedHome).length === 0) return true
    if (Date.now() > deadline) return false
    const { promise, resolve: resume } = Promise.withResolvers<void>()
    setTimeout(resume, 50)
    await promise
  }
}

/**
 * End the helper daemons under one home. SIGTERM first and SIGKILL only for
 * what ignored it, each phase waited for on the OS's own answer.
 *
 * THIS IS THE FIXTURE'S OWN STEP AND NOT A PRODUCT GESTURE: what a person
 * does instead is lose the network, lock the vault or walk away from a host.
 * All of them land on the same verdict — nobody could be asked — and the
 * helper is the one this stand can end deterministically.
 */
async function endDaemonsUnder(isolatedHome: string): Promise<void> {
  if (isolatedHome === '') return
  for (const signal of ['SIGTERM', 'SIGKILL'] as const) {
    const survivors = daemonsFor(isolatedHome)
    if (survivors.length === 0) return
    for (const pid of survivors) {
      try {
        process.kill(pid, signal)
      } catch {
        /* already gone */
      }
    }
    if (await daemonsGone(isolatedHome, 15_000)) return
  }
  throw new Error(
    `this spec's helper daemon(s) ${daemonsFor(isolatedHome).join(', ')} outlived SIGTERM and SIGKILL`,
  )
}

interface Rect {
  left: number
  right: number
  top: number
  bottom: number
  width: number
  height: number
}

interface Geometry {
  pane: Rect
  host: Rect
  card: Rect
  /** The first rendered child of the pane after the notice — the pane header
   *  where the pane draws one, the pane's content otherwise. */
  under: { cls: string; rect: Rect } | null
  cardStyle: { borderRadius: string; marginBottom: string; background: string }
  paneChildren: Array<{ cls: string; hidden: boolean | 'until-found'; height: number; top: number }>
}

/** Everything this spec asserts, measured in the page, in one round trip. */
async function noticeGeometry(page: Page): Promise<Geometry> {
  return page.evaluate(
    (sel) => {
      const rect = (el: Element): Rect => {
        const r = el.getBoundingClientRect()
        return {
          left: r.left,
          right: r.right,
          top: r.top,
          bottom: r.bottom,
          width: r.width,
          height: r.height,
        }
      }
      const pane = document.querySelector(sel.pane) as HTMLElement
      const host = pane.querySelector('.nocx-unreconciled-notice') as HTMLElement
      const card = host.querySelector('.ui-status-card') as HTMLElement
      let next: Element | null = host.nextElementSibling
      while (next !== null && next.getBoundingClientRect().height === 0) {
        next = next.nextElementSibling
      }
      const style = getComputedStyle(card)
      return {
        pane: rect(pane),
        host: rect(host),
        card: rect(card),
        under: next === null ? null : { cls: (next as HTMLElement).className, rect: rect(next) },
        cardStyle: {
          borderRadius: style.borderTopLeftRadius,
          marginBottom: style.marginBottom,
          background: style.backgroundColor,
        },
        paneChildren: Array.from(pane.children).map((el) => ({
          cls: (el as HTMLElement).className,
          hidden: (el as HTMLElement).hidden,
          height: Math.round(el.getBoundingClientRect().height),
          top: Math.round(el.getBoundingClientRect().top),
        })),
      }
    },
    { pane: PANE },
  )
}

test.describe('the third state sits in the pane as a band (nocx-xn63t.6.36)', () => {
  let home: DisposableRoot
  let backend: VaultBackend

  test.beforeEach(() => {
    home = { root: mkdtempSync(join(tmpdir(), 'nocx-notice-band-')) }
    backend = new VaultBackend(serverBin(), home)
  })

  test.afterEach(async () => {
    backend?.stop()
    let isolatedHome = ''
    try {
      isolatedHome = backend.isolatedHome
    } catch {
      /* the backend never started, so it started no daemon either */
    }
    await endDaemonsUnder(isolatedHome)
  })

  test('the notice spans the pane and meets what is under it with no gap', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 })

    // ── A pane with an open command, whose session is held by this home's
    //    helper daemon ────────────────────────────────────────────────────
    const ep1 = await backend.start()
    await bindEndpoint(page, ep1)
    await page.goto('/')
    await promptReady(page)
    await expect(page.locator(TAB)).toHaveCount(1, { timeout: 30_000 })

    const input = page.locator(INPUT)
    await input.fill(STAYS_OPEN)
    await page.keyboard.press('Enter')
    await expect(page.locator(`${PANE} .cmd-block.cmd-block-running`)).toBeVisible({
      timeout: 30_000,
    })

    // ── The coordinator that knew about it goes away, and the helper that
    //    held the shell goes with it: the replacement has nobody to ask ────
    await endDaemonsUnder(backend.isolatedHome)
    const ep2 = await backend.restart()
    await bindEndpoint(page, ep2)
    await page.reload()
    await expect(page.locator(TAB)).toHaveCount(1, { timeout: 90_000 })

    // THE THIRD STATE IS UP, which is the state the whole spec is about: the
    // notice is drawn because a row's session could not be judged.
    const notice = page.locator(NOTICE)
    await expect(notice).toBeVisible({ timeout: 60_000 })

    const g = await noticeGeometry(page)
    console.log(`notice geometry: ${JSON.stringify(g)}`)

    // FULL WIDTH. Nothing of the pane is left beside the band — asserted on
    // the band itself rather than on the host that carries it, because the
    // host is a plain div and the painted element is what the person sees.
    expect(
      Math.abs(g.card.left - g.pane.left),
      `the band is inset ${g.card.left - g.pane.left}px on the left: ${JSON.stringify(g)}`,
    ).toBeLessThanOrEqual(FLUSH_PX)
    expect(
      Math.abs(g.pane.right - g.card.right),
      `the band is inset ${g.pane.right - g.card.right}px on the right: ${JSON.stringify(g)}`,
    ).toBeLessThanOrEqual(FLUSH_PX)

    // FLUSH AGAINST THE PANE, on every edge that has something on the other
    // side of it: the pane's own top, and the top of whatever it draws next.
    expect(
      Math.abs(g.card.top - g.pane.top),
      `the band floats ${g.card.top - g.pane.top}px below the pane's top edge: ${JSON.stringify(g)}`,
    ).toBeLessThanOrEqual(FLUSH_PX)
    expect(g.under, `nothing is drawn under the notice: ${JSON.stringify(g)}`).not.toBeNull()
    expect(
      Math.abs(g.under!.rect.top - g.card.bottom),
      `there is a ${g.under!.rect.top - g.card.bottom}px gap under the band (${g.under!.cls}): ${JSON.stringify(g)}`,
    ).toBeLessThanOrEqual(FLUSH_PX)

    // AND IT IS A BAND, NOT THE CARD: a rounded box with its own air below it
    // is what the owner photographed, and it is the kit's rule that buys it
    // back — the surface may place a component and never repaint it.
    expect(g.cardStyle.borderRadius, `the band is still rounded: ${JSON.stringify(g)}`).toBe('0px')
    expect(g.cardStyle.marginBottom, `the band still owns air below it`).toBe('0px')
  })
})
