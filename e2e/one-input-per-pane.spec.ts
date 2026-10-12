// e2e: one input path per pane (nocx-7jd59).
//
// The pane owns one current terminal input path. Exercise it as a person does:
// type a command, submit it, and watch its output on the painter's accessible
// grid. The old xterm helper textarea and its ARIA name are not the contract;
// the output grid is the accessible terminal screen.
import { test, expect, promptReady } from './harness'

const marker = `NOCX-INPUT-REACHED-${Date.now().toString(36)}`

test('one terminal input path reaches the shell and the painter grid is accessible output', async ({
  page,
}) => {
  await page.goto('/')
  await expect(page.locator('.nocx-tab')).toHaveCount(1)
  await promptReady(page)

  const pane = page.locator('.pane.active')
  await expect(pane.locator('.ui-terminal-input')).toHaveCount(1)

  // The command's echoed spelling is split, so only its actual output can
  // contain the contiguous marker.
  await page.keyboard.type(`printf '%s\n' 'NOCX''${marker.slice(4)}'`)
  await page.keyboard.press('Enter')

  const grid = pane.getByRole('grid', { name: 'Terminal output' })
  await expect(grid).toHaveCount(1)
  await expect(grid).toHaveAttribute('aria-label', 'Terminal output')
  await expect(grid).toContainText(marker, { timeout: 15_000 })
})
