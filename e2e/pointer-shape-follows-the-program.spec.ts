/**
 * THE CURSOR THE PANE ACTUALLY SHOWS (nocx-1ebrs.24).
 *
 * A person's complaint, measured where they made it: in a full-screen program
 * that reports the mouse, the pointer over the grid was an I-beam while every
 * other terminal showed the arrow. The half that fixed it — the arrow while
 * the program owns the click, the I-beam while the terminal does — shipped
 * first; this spec adds the half a program can ask for itself, OSC 22, and
 * holds both to the same rule: what the pane shows is read from the GRID'S
 * COMPUTED cursor, not from an inline style, so a stylesheet rule cannot be
 * overridden silently and an inheriting `auto` cannot pass for an answer.
 *
 * The fixture is a real program on a real pty — raw mode, the alternate
 * screen, DECSET 1000, and OSC 22 — and it repaints a marker after each step,
 * so every wait is on a state the screen shows rather than on a duration.
 * Nothing here reads the painter's own attribute: that is the mechanism, and
 * the person sees the cursor.
 */
import { test, expect, promptReady } from './harness'
import type { Page } from './harness'
import { mkdtempSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { tmpdir } from 'node:os'

const LIVE = '.pane.active .terminal-live-grid'

/** The pointer the grid COMPUTES, and any cursor declared inline on it. */
async function pointerOf(page: Page): Promise<{ computed: string; inline: string }> {
  return page.evaluate((live) => {
    const grid = document.querySelector<HTMLElement>(live)
    if (grid === null) throw new Error('the live grid is not mounted')
    return { computed: getComputedStyle(grid).cursor, inline: grid.style.cursor }
  }, LIVE)
}

/** One step of the program: send its key, wait for the marker it paints, then
 *  read the pointer. The wait is on the screen the program drew, never on a
 *  clock — the frame that carries the new mode carries the marker with it. */
async function step(page: Page, key: string, marker: string): Promise<string> {
  await page.keyboard.press(key)
  await expect(page.locator(LIVE)).toContainText(marker, { timeout: 20_000 })
  const pointer = await pointerOf(page)
  // Stated BY THE STYLESHEET: an inline cursor would win over every rule,
  // including the hand an armed link shows, and the computed value would then
  // prove nothing about the shipped cascade.
  expect(pointer.inline, 'the pointer must come from a rule, never an inline style').toBe('')
  return pointer.computed
}

test.describe('the pointer over the live grid', () => {
  test('follows the click owner, then the shape the program asks for', async ({ page }) => {
    await page.goto('/')
    await expect(page.locator('.nocx-tab')).toHaveCount(1)
    await promptReady(page)

    const dir = mkdtempSync(join(tmpdir(), 'nocx-pointer-'))
    const script = join(dir, 'pointer-job.sh')
    writeFileSync(
      script,
      [
        '#!/bin/sh',
        // A full-screen program's own terms: no echo, no line discipline, the
        // alternate screen, and one key per state the pane can be in.
        'old=$(stty -g)',
        'cleanup() { stty "$old"; exit 0; }',
        'trap cleanup INT TERM HUP',
        'stty raw -echo',
        `printf '\\033[?1049h'`,
        `printf '\\033[2J\\033[H'`,
        `printf 'POINTER-START\\r\\n'`,
        'while : ; do',
        '  key=$(dd bs=1 count=1 2>/dev/null)',
        '  case "$key" in',
        // The program takes the clicks: the pointer must stop promising text.
        `    a) printf '\\033[?1000h'; printf 'STEP-TRACKING-ON\\r\\n' ;;`,
        // And asks for a shape of its own on top of that.
        `    b) printf '\\033]22;crosshair\\007'; printf 'STEP-SHAPE-CROSSHAIR\\r\\n' ;;`,
        // The clicks come back to the terminal; the REQUEST is still standing,
        // because a program that asked once is still asking.
        `    c) printf '\\033[?1000l'; printf 'STEP-TRACKING-OFF\\r\\n' ;;`,
        // And it takes the request back by asking for the arrow.
        `    d) printf '\\033]22;default\\007'; printf 'STEP-SHAPE-DEFAULT\\r\\n' ;;`,
        `    q) printf '\\033[?1049l'; cleanup ;;`,
        '  esac',
        'done',
        '',
      ].join('\n'),
    )

    await page.keyboard.type(`sh ${script}`)
    await page.keyboard.press('Enter')
    await expect(page.locator(LIVE)).toContainText('POINTER-START', { timeout: 20_000 })

    // The state the terminal decides, before the program says anything: no
    // program reports the mouse, so the grid is text the person may select and
    // the I-beam says so.
    expect(await pointerOf(page)).toEqual({ computed: 'text', inline: '' })

    // The state the program takes: its clicks are its own, so the pointer
    // stops promising a text selection it will not get.
    expect(await step(page, 'a', 'STEP-TRACKING-ON')).toBe('default')
    // The state the program asks for, over the one it owns.
    expect(await step(page, 'b', 'STEP-SHAPE-CROSSHAIR')).toBe('crosshair')
    // The request outlives the mode that was set beside it.
    expect(await step(page, 'c', 'STEP-TRACKING-OFF')).toBe('crosshair')
    // And it can be taken back: the program asks for the arrow, and the arrow
    // is what the person gets even though the terminal owns the click again.
    expect(await step(page, 'd', 'STEP-SHAPE-DEFAULT')).toBe('default')
  })
})
