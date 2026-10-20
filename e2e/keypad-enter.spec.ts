/**
 * e2e: the numerical keypad's Enter is Enter (nocx-1ebrs.32).
 *
 * FOUND BY THE OWNER on the shipped 0.6.0: the keypad's Enter did nothing where
 * Return worked. Two surfaces read a keyboard and only one of them was guilty,
 * and this file watches the one that can only be watched here — the LIVE GRID,
 * whose bytes are the runtime's and whose key identity the pane names
 * (frontend/src/intent-input.ts). A unit test can assert the intent the pane
 * emits; only this can say that the intent became a newline in a program's
 * stdin, over the real transport, the real pty and the real encoder.
 *
 * WHAT IS ASSERTED, and why it is a rule rather than a mechanism. A person can
 * run a command with the keypad's Enter from the composer, and can answer a
 * program that is reading a line with the keypad's Enter from the grid. Both
 * markers are PRINTED by a program — the typed text spells the format string,
 * never the marker — so a passing assertion cannot be the fixture echoing what
 * this file typed. The second one is the case that was broken: before the fix
 * the keydown was dropped as an unknown key, `read` never returned, and no
 * marker could appear at all.
 *
 * The modes are the program's, and this spec leaves them alone: bash enables
 * neither application keypad mode (DECNKM) nor the Kitty keyboard protocol, so
 * the rule being watched is "with no protocol enabled, the keypad's Enter and
 * Return produce the same bytes". The other half — SS3 M once a program has
 * asked for the keypad's own sequences, and KP_ENTER under the Kitty protocol —
 * is a byte-level decision no browser can observe, and it is asserted against
 * the encoder itself (internal/emulator/ghostty: the keypad's Enter is Return
 * until the program asks for its own sequence).
 */
import { test, expect, promptReady } from './harness'

/** The live region's painted rows — where a person reads what a program said. */
const LIVE_ROWS = '.pane.active .terminal-live-container .term-grid-row'

test('the keypad’s Enter submits a command and answers a program that is reading', async ({
  page,
}) => {
  await page.goto('/')
  await expect(page.locator('.nocx-tab')).toHaveCount(1)
  await promptReady(page)

  // ── the composer ────────────────────────────────────────────────────────
  //
  // The draft is a command whose OUTPUT is the marker; the text this test types
  // contains only the format string.
  await page.keyboard.type("printf 'NOCX-KP-%s\\n' COMPOSER")
  await page.keyboard.press('NumpadEnter')
  await expect(page.locator(LIVE_ROWS, { hasText: 'NOCX-KP-COMPOSER' })).toBeVisible({
    timeout: 10_000,
  })

  // ── the grid ────────────────────────────────────────────────────────────
  //
  // `read` takes the pty and blocks: the composer is hidden, so the next
  // keystrokes are the GRID's — the surface whose key table had no name for the
  // keypad's Enter and dropped the keydown there. The line it reads is the
  // marker's argument, so the printed marker proves the CR arrived.
  await promptReady(page)
  await page.keyboard.type('IFS= read -r line; printf \'NOCX-KP-%s\\n\' "$line"')
  await page.keyboard.press('Enter')

  await page.keyboard.type('GRID')
  await page.keyboard.press('NumpadEnter')
  await expect(page.locator(LIVE_ROWS, { hasText: 'NOCX-KP-GRID' })).toBeVisible({
    timeout: 10_000,
  })

  // The prompt comes back, which is the shell's own account that the line it
  // read ended where it should: a program left waiting would never repaint it.
  await promptReady(page)
})
