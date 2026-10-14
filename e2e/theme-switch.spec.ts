import { appReadyForInput, promptReady, test, expect } from './harness'

test.describe('theme switching', () => {
  const THEME_ROW = '.ui-settings-row[data-key="ui.theme"]'
  const THEME_SELECT = `${THEME_ROW} select`

  test('switches theme via settings with no terminal remount', async ({ page }) => {
    // Open the app and wait for the initial terminal tab
    await page.goto('/')
    await expect(page.locator('.nocx-tab-title').first()).not.toHaveText('', { timeout: 10_000 })
    await appReadyForInput(page)

    // Count panes before theme change
    const initialPaneCount = await page.locator('.pane').count()

    // Open Settings and find the theme select. `ui.theme` lives in the
    // Interface section (internal/settings/settings.go), and Settings opens on
    // its FIRST section — so the row is in the DOM and hidden until the rail
    // navigates to it.
    await page.keyboard.press('Meta+,')
    await page.locator('.ui-grouped-nav__item[data-item="Interface"] button').click()
    await expect(page.locator(THEME_SELECT)).toBeVisible({ timeout: 5000 })

    // Switch to light theme and wait for the async round-trip:
    // selectOption → RPC set → backend → notifier → observer refetch →
    // reconcileThemeFromGo → data-theme attribute
    await page.selectOption(THEME_SELECT, 'light')
    await page.waitForFunction(
      () => document.documentElement.getAttribute('data-theme') === 'light',
      { timeout: 5000 },
    )

    // Close Settings
    await page.keyboard.press('Meta+w')
    await expect(page.locator('.nocx-tab-title').first()).not.toHaveText('', { timeout: 5000 })

    // Verify no terminal panes were created or destroyed (no remount)
    await expect(page.locator('.pane')).toHaveCount(initialPaneCount, { timeout: 5000 })

    // Switch back to tokyo-night
    await page.keyboard.press('Meta+,')
    await page.locator('.ui-grouped-nav__item[data-item="Interface"] button').click()
    await expect(page.locator(THEME_SELECT)).toBeVisible({ timeout: 5000 })
    await page.selectOption(THEME_SELECT, 'tokyo-night')
    await page.waitForFunction(
      () => document.documentElement.getAttribute('data-theme') === 'tokyo-night',
      { timeout: 5000 },
    )

    // Close Settings
    await page.keyboard.press('Meta+w')
    await expect(page.locator('.nocx-tab-title').first()).not.toHaveText('', { timeout: 5000 })

    // Still no pane churn after switching back
    await expect(page.locator('.pane')).toHaveCount(initialPaneCount, { timeout: 5000 })
  })

  /**
   * The registration checks in frontend/src/theme-catalogue.test.ts read the
   * @import line out of style.css as text. That proves somebody wrote the line;
   * it does not prove the bundler resolved it, and a theme whose stylesheet never
   * loaded fails in the one way that looks like success — `data-theme` flips, no
   * rule matches, every token keeps the default theme's value and the app simply
   * stays Tokyo Night. That is exactly how a missing @import passed for a working
   * theme switch once already (nocx-u7wq.2).
   *
   * So this asserts a token VALUE after the switch, against the running bundle.
   * One ported theme is enough: they share an import block and a scoping rule, so
   * if this one resolves the mechanism works, and the catalogue test is what keeps
   * the other nine in the block (nocx-7o42).
   */
  test('a ported theme changes the resolved tokens, not just the attribute', async ({ page }) => {
    await page.goto('/')
    await expect(page.locator('.nocx-tab-title').first()).not.toHaveText('', { timeout: 10_000 })
    await appReadyForInput(page)

    const canvas = (): Promise<string> =>
      page.evaluate(() =>
        getComputedStyle(document.documentElement).getPropertyValue('--color-canvas').trim(),
      )

    // Tokyo Night's canvas, applied by the bare `:root` block before any switch.
    expect(await canvas()).toBe('#1a1b26')

    await page.keyboard.press('Meta+,')
    await page.locator('.ui-grouped-nav__item[data-item="Interface"] button').click()
    await expect(page.locator(THEME_SELECT)).toBeVisible({ timeout: 5000 })
    await page.selectOption(THEME_SELECT, 'dracula')
    await page.waitForFunction(
      () => document.documentElement.getAttribute('data-theme') === 'dracula',
      { timeout: 5000 },
    )

    // Dracula's canvas. If dracula.css had not loaded, `data-theme` would still
    // read 'dracula' and this would still read #1a1b26.
    await expect.poll(canvas, { timeout: 5000 }).toBe('#282a36')

    // The terminal tokens the xterm adapter resolves come from the same block,
    // and they are what actually repaints the panes.
    const ansiRed = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--terminal-ansi-1').trim(),
    )
    expect(ansiRed).toBe('#ff5555')

    // Leave the app on the default so a shared-state run does not inherit it.
    await page.selectOption(THEME_SELECT, 'tokyo-night')
    await page.waitForFunction(
      () => document.documentElement.getAttribute('data-theme') === 'tokyo-night',
      { timeout: 5000 },
    )
  })
  /**
   * The live grid paints a program's palette colours from the CHOSEN theme,
   * and a theme switch repaints it in place (nocx-zg3k3.28). Built without the
   * theme, the painter fell back to Tokyo Night under every theme: mc's
   * panels came out #7aa2f7 under Graphite, and the contrast floor then
   * darkened mc's marked and unmarked names alike into one colour.
   */
  test('the live grid wears the theme’s palette and follows a switch', async ({ page }) => {
    await page.goto('/')
    await promptReady(page)
    await page.keyboard.type("printf '\\033[44m PANEL \\033[0m\\n'; read x")
    await page.keyboard.press('Enter')
    const grid = page.locator('.pane.active .terminal-live-grid')
    await expect(grid).toContainText('PANEL', { timeout: 15_000 })
    const panel = (): Promise<{ painted: string; token: string }> =>
      page.evaluate(() => {
        const span = [
          ...document.querySelectorAll<HTMLElement>(
            '.pane.active .terminal-live-grid .term-grid-row span',
          ),
        ].find((s) => (s.textContent ?? '').includes('PANEL'))!
        const probe = document.createElement('div')
        probe.style.color = getComputedStyle(document.documentElement)
          .getPropertyValue('--terminal-ansi-4')
          .trim()
        document.body.appendChild(probe)
        const token = getComputedStyle(probe).color
        probe.remove()
        return { painted: getComputedStyle(span).backgroundColor, token }
      })
    const before = await panel()
    expect(before.painted).toBe(before.token)

    await page.keyboard.press('Meta+,')
    await page.locator('.ui-grouped-nav__item[data-item="Interface"] button').click()
    await expect(page.locator(THEME_SELECT)).toBeVisible({ timeout: 5000 })
    await page.selectOption(THEME_SELECT, 'dracula')
    await page.waitForFunction(
      () => document.documentElement.getAttribute('data-theme') === 'dracula',
      { timeout: 5000 },
    )
    await page.keyboard.press('Meta+w')
    // Dracula's ansi-4, #bd93f9: the row repainted without new output.
    await expect
      .poll(async () => (await panel()).painted, { timeout: 5000 })
      .toBe('rgb(189, 147, 249)')

    await page.keyboard.press('Meta+,')
    await page.locator('.ui-grouped-nav__item[data-item="Interface"] button').click()
    await page.selectOption(THEME_SELECT, 'tokyo-night')
    await page.waitForFunction(
      () => document.documentElement.getAttribute('data-theme') === 'tokyo-night',
      { timeout: 5000 },
    )
    await page.keyboard.press('Meta+w')
    await page.keyboard.press('Enter')
  })
})
