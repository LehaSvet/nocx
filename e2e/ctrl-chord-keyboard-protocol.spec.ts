/**
 * A Ctrl chord reaches a program that asked for the Kitty keyboard protocol
 * as the chord (nocx-zg3k3.14.20). codex pushes those flags; Ctrl+C reached it
 * as the letter "c", so it could neither be interrupted nor quit.
 */

import { test, expect, promptReady, clickIntoEditor } from './harness'

const GRID = '.pane.active .terminal-live-container'

for (const [protocol, enable, want] of [
  ['legacy', '', '03'],
  ['the Kitty protocol', '\\033[>1u', '1b 5b 39 39 3b 35 75'],
] as const) {
  test(`Ctrl+C under ${protocol} reaches the program as the chord`, async ({ page }) => {
    await page.goto('/')
    await promptReady(page)
    await clickIntoEditor(page)
    // A raw-mode reader prints the bytes it receives, up to a full sequence.
    const count = want.split(' ').length
    await page.keyboard.type(
      `printf '${enable}'; stty raw -echo; printf 'RE%sADY\\n' ''; dd bs=1 count=${String(count)} 2>/dev/null | od -An -tx1; stty sane`,
    )
    await page.keyboard.press('Enter')
    // The marker is split in the command text and only appears after raw mode is active.
    await expect(page.getByRole('row', { name: 'READY', exact: true })).toBeVisible()
    await page.locator(GRID).click()
    await page.keyboard.press('Control+c')
    await expect(page.locator(GRID)).toContainText(want, { timeout: 10_000 })
  })
}
