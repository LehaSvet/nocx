/**
 * The painted grid has ONE row pitch (nocx-zg3k3.20, nocx-zg3k3.21).
 *
 * A full-screen program fills rows with background colour and parks its
 * cursor. Two defects lived here, both visible only at a fractional display
 * ratio (a Retina screen at a browser zoom other than 100%):
 *
 * - the rows were laid out at the DOM's own line box while the cursor was
 *   placed by the accepted geometry, that box rounded up to device pixels —
 *   at a ratio of 1.8 that was 16.80px against 17.22px, so the cursor drifted
 *   off its row as it went down the screen;
 * - a background covered the font's content box and not the row, so rows of
 *   one colour (mc's panels) were striped.
 */

import { test, expect, promptReady } from './harness'
import type { Page } from './harness'

const LIVE = '.pane.active .terminal-live-grid'

for (const ratio of [1, 1.8]) {
  test.describe(`at a display ratio of ${String(ratio)}`, () => {
    test.use({ deviceScaleFactor: ratio })

    test('coloured rows tile without gaps and the cursor sits on its row', async ({ page }) => {
      await page.setViewportSize({ width: 1280, height: 800 })
      await page.goto('/')
      await promptReady(page)
      // The alternate screen; six rows on a blue background (one of braille,
      // one of box drawing), then the cursor parked on row 12, column 6.
      await page.keyboard.type(
        "printf '\\033[?1049h\\033[H'; " +
          'for i in 1 2 3; do printf \'\\033[44;37m%-60s\\033[0m\\n\' "plain row $i"; done; ' +
          "printf '\\033[44;37m%-60s\\033[0m\\n' '⣿⣿⡿⠿ braille ⠿⢿⣿'; " +
          "printf '\\033[44;37m%-60s\\033[0m\\n' '┌──┐ box │ ═══ ║'; " +
          "printf '\\033[44;37m%-60s\\033[0m\\n' 'plain row 6'; " +
          "printf '\\033[12;6HX\\033[12;6H\\033[?25h'; sleep 60",
      )
      await page.keyboard.press('Enter')
      await expect(page.locator(`${LIVE} .term-grid-row`, { hasText: 'plain row 6' })).toHaveCount(
        1,
        {
          timeout: 15_000,
        },
      )
      await expect
        .poll(() => cursorRow(page), { message: 'the cursor was never painted on row 12' })
        .not.toBeNull()

      const m = await measure(page)
      // Rows 3 and 4 (braille, box drawing) can be drawn from a fallback face
      // whose content box is not the primary font's, so the padding measured
      // on the primary font leaves them a fraction of a device pixel off
      // (ADR-0009 rule 4 measures one face, and forbids the inline-block that
      // would not care). A seam touching either is held within a device
      // pixel; every other seam must close exactly.
      const devicePixel = 1 / ratio
      for (let i = 0; i < 6; i++) {
        const row = m.rows[i]!
        const fallback = i >= 3 && i <= 5
        expect(row.bgHeight, `row ${String(i)}'s background covers the row`).toBeCloseTo(
          row.height,
          1,
        )
        if (i > 0) {
          const above = m.rows[i - 1]!
          const seam = Math.abs(row.bgTop - (above.bgTop + above.bgHeight))
          expect(seam, `row ${String(i)}'s background meets the one above`).toBeLessThan(
            fallback ? devicePixel : 0.05,
          )
        }
      }
      expect(m.cursorTop, 'the cursor is on row 12, where the X was written').toBeCloseTo(
        m.rows[11]!.top,
        0,
      )
      await page.keyboard.press('Control+C')
    })
  })
}

async function cursorRow(page: Page): Promise<number | null> {
  return page.evaluate((sel) => {
    const cursor = document.querySelector<HTMLElement>(`${sel} .term-grid-cursor`)
    return cursor && !cursor.hidden ? cursor.getBoundingClientRect().top : null
  }, LIVE)
}

async function measure(page: Page): Promise<{
  rows: { top: number; height: number; bgTop: number; bgHeight: number }[]
  cursorTop: number
}> {
  return page.evaluate((sel) => {
    const grid = document.querySelector<HTMLElement>(sel)!
    const origin = grid.getBoundingClientRect().top
    const rows = [...grid.querySelectorAll<HTMLElement>('.term-grid-row')]
      .slice(0, 12)
      .map((row) => {
        const box = row.getBoundingClientRect()
        const painted = [...row.querySelectorAll<HTMLElement>('span')].find(
          (s) => getComputedStyle(s).backgroundColor !== 'rgba(0, 0, 0, 0)',
        )
        const bg = painted?.getBoundingClientRect()
        return {
          top: box.top - origin,
          height: box.height,
          bgTop: bg ? bg.top - origin : Number.NaN,
          bgHeight: bg ? bg.height : Number.NaN,
        }
      })
    const cursor = grid.querySelector<HTMLElement>('.term-grid-cursor')!
    return { rows, cursorTop: cursor.getBoundingClientRect().top - origin }
  }, LIVE)
}

// The cursor is a solid block in the theme's cursor colour that blinks while
// the terminal has the keyboard, and an outline while it does not
// (nocx-zg3k3.24) — it had been a faint translucent block either way.
test('the cursor is a blinking block with the keyboard, and an outline without it', async ({
  page,
}) => {
  await page.goto('/')
  await promptReady(page)
  await page.keyboard.type("printf '\\033[?1049h\\033[3;3HX\\033[3;3H\\033[?25h'; sleep 60")
  await page.keyboard.press('Enter')
  await expect.poll(() => cursorRow(page)).not.toBeNull()
  const cursor = page.locator(`${LIVE} .term-grid-cursor`)

  await page.locator(LIVE).click()
  await expect(cursor).toHaveCSS('animation-name', 'term-cursor-blink')
  await expect(cursor).toHaveText('X')
  const solid = await cursor.evaluate((el) => getComputedStyle(el).backgroundColor)
  expect(solid).not.toBe('rgba(0, 0, 0, 0)')

  await page
    .locator('.pane.active .nocx-editor, .pane.active .tab-chrome')
    .first()
    .evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
  await expect(cursor).toHaveCSS('animation-name', 'none')
  await expect(cursor).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await page.locator(LIVE).click()
  await page.keyboard.press('Control+C')
})
