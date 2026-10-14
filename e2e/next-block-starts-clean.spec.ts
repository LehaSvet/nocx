/**
 * A command's block holds only what that command wrote (nocx-zg3k3.23).
 *
 * An inline TUI that quits by parking the cursor above rows it had painted —
 * omp does — left those rows on the screen below the prompt, and the next
 * command's output was written over them: its block began with omp's input
 * row ("Hi!" on its background) or carried the tail of a longer stale row.
 * The prompt now erases below the cursor before its A marker.
 */

import { test, expect, promptReady } from './harness'

test('rows a program left below its parked cursor are not the next command’s output', async ({
  page,
}) => {
  await page.goto('/')
  await promptReady(page)
  // Paints three rows, the last on a background, and parks two rows up.
  await page.keyboard.type(
    "printf 'TUI-1\\nTUI-2\\n\\033[44m STALE-ROW-LONG \\033[0m\\n\\033[2A\\r'",
  )
  await page.keyboard.press('Enter')
  await promptReady(page)
  await page.keyboard.type('echo NEXT-OUT')
  await page.keyboard.press('Enter')
  const next = page
    .locator('.pane.active .cmd-block', { hasText: 'NEXT-OUT' })
    .filter({ hasNotText: 'printf' })
    .last()
  await expect(next).not.toHaveClass(/cmd-block-running/, { timeout: 15_000 })
  await expect(next.locator('.cmd-output')).toHaveText('NEXT-OUT')
})

// An inline TUI that erases its frame on the way out moves the cursor up past
// its own first row and rewrites from there — claude does — and the block it
// leaves must not carry the previous command's rows it never touched
// (nocx-zg3k3.23).
test('a command that rewrites rows above its start does not carry the rows it left alone', async ({
  page,
}) => {
  await page.goto('/')
  await promptReady(page)
  await page.keyboard.type('for i in 1 2 3 4 5 6; do echo PRIOR-$i; done')
  await page.keyboard.press('Enter')
  await promptReady(page)
  await page.keyboard.type("printf '\\033[2A\\033[2KUP-ROW\\n\\033[2KEND-ROW\\n'")
  await page.keyboard.press('Enter')
  const block = page.locator('.pane.active .cmd-block', { hasText: 'UP-ROW' }).last()
  await expect(block).not.toHaveClass(/cmd-block-running/, { timeout: 15_000 })
  await expect(block.locator('.cmd-output')).toHaveText(/^UP-ROW\s*END-ROW$/)
})
