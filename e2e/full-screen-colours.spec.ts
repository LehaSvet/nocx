/**
 * The colours a 16-colour program paints come out as it painted them, and
 * readable (nocx-zg3k3.26, nocx-zg3k3.27).
 *
 * mc's menu bar and htop's header are a background erased to the end of the
 * row: the text stops, the colour does not. And htop's tab brackets are green
 * on green, which 0.4's xterm drew legible through its 4.5:1 floor. Both were
 * lost with xterm: the bar stopped where its text did, the brackets vanished.
 */

import { test, expect, promptReady } from './harness'

test('an erased background reaches the row’s end, and same-colour text stays readable', async ({
  page,
}) => {
  await page.goto('/')
  await promptReady(page)
  await page.keyboard.type(
    "printf '\\033[44mMENU-BAR\\033[K\\033[0m\\n\\033[32;42m[TAB]\\033[0m\\n'; read x",
  )
  await page.keyboard.press('Enter')
  const grid = page.locator('.pane.active .terminal-live-grid')
  await expect(grid).toContainText('MENU-BAR', { timeout: 15_000 })
  const facts = await page.evaluate(() => {
    const rows = [
      ...document.querySelectorAll<HTMLElement>('.pane.active .terminal-live-grid .term-grid-row'),
    ]
    const bar = rows.find((r) => (r.textContent ?? '').startsWith('MENU-BAR'))!
    // The row's LAST node, element or bare text: a blank tail painted with no
    // background is a text node or a span without one.
    const tail = bar.lastChild
    const tailBackground =
      tail instanceof HTMLElement ? getComputedStyle(tail).backgroundColor : 'none'
    const tailText = tail?.textContent ?? ''
    const tab = rows.find((r) => (r.textContent ?? '').startsWith('[TAB]'))!
    const span = [...tab.querySelectorAll<HTMLElement>('span')].find((s) =>
      (s.textContent ?? '').includes('[TAB]'),
    )!
    const cs = getComputedStyle(span)
    return {
      tailEndsBlank: /\s$/.test(tailText),
      barTailBackground: tailBackground,
      barHeadBackground: getComputedStyle(bar.firstElementChild as HTMLElement).backgroundColor,
      tabColor: cs.color,
      tabBackground: cs.backgroundColor,
    }
  })
  // The blank tail of the bar wears the bar's colour.
  expect(facts.tailEndsBlank).toBe(true)
  expect(facts.barTailBackground).toBe(facts.barHeadBackground)
  // Green on green is drawn in a different colour from its ground.
  expect(facts.tabColor).not.toBe(facts.tabBackground)
  await page.keyboard.press('Enter')
})
