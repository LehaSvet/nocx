// nocx-ruf5o: the FIRST command in a fresh pane completes when the prompt
// returns — it never stays Running with the new prompt inside it.
//
// The observation this guards (owner, 2026-10-21, nocx 0.6.1 on macOS): in a
// fresh tab the first command's block never settled — "Running 8.9s" while the
// prompt was already printed inside the block, with vi showing the same shape
// and NO lifecycle `complete` fact ever minted. That build predates the two
// fixes in this family — the bootstrap window finding a loader token behind
// renderer bytes (PR #433, nocx-1ebrs.39) and returning the token the trim
// uncovers (PR #447, nocx-1ebrs.50) — so this spec proves the CURRENT tree
// closes the first block, in the two shapes the owner measured: an empty
// ZDOTDIR (no user rc at all) and the widget-wrapping zsh plugin stack.
//
// The instrument is state-to-state, never a sleep: the command's own output
// in the pane's grid is "the command ran" made observable, and the assertion
// bounds the interval from that output to the block's settled state. The
// bound sits above honest renderer/pty jitter and BELOW every bounded
// deadline the product declares (the carrier's 3 s, the pane screen and tool
// surface 5 s), so a pane that waits one out fails here rather than passing
// silently. Settled means the pane holds no `.cmd-block-running` and the
// command's block carries its `data-outcome` — the same settled vocabulary
// block-outcome.spec.ts reads — and the editor has the prompt back.
//
// WHERE EACH VARIANT RUNS. A local pane runs the account's own login shell —
// nothing in panes.create chooses one — so the empty-ZDOTDIR variant runs on
// every host (zsh on a stock macOS runner, bash in the Linux e2e image) and
// the plugin-stack variant runs exactly where both of its preconditions hold:
// the login shell is zsh AND the pinned plugins are installed
// (scripts/install-zsh-zle-plugins.sh, the same provisioning the Go
// composition test uses, which exports NOCX_ZSH_PLUGINS to the whole CI job).
// Anywhere else it skips with the reason said out loud, because a zsh plugin
// composition measured under bash is not evidence of anything.

import { existsSync, rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

import { appReadyForInput, promptReady, test, expect } from './harness'
import { readStand, standBackendLogTail } from './stand'

const COMMAND = 'echo ruf5o-complete-done'
/** Below every declared deadline (carrier 3 s, pane screen / tool surface
 *  5 s), above honest pty/renderer jitter — the same bound the
 *  first-command-delay spec holds the start interval to. */
const BOUND_AFTER_OUTPUT_MS = 2_500
const CHECKING = 'Checking this shell…'

/** The plugin entries, pinned by nocx-1ebrs.27's commit pins; the entry file
 *  sits at each checkout's root. Mirrors the Go fixture
 *  (zsh_parent_block_close_test.go) so the wrapping measured here is the
 *  wrapping that test composes with. */
const PLUGIN_ENTRIES = [
  'fast-syntax-highlighting/fast-syntax-highlighting.plugin.zsh',
  'zsh-syntax-highlighting/zsh-syntax-highlighting.zsh',
  'zsh-autosuggestions/zsh-autosuggestions.zsh',
]

/** The shell the pane actually ran, from the backend's own launch decision —
 *  the pane launch decided line is Info level, so it is in every stand's log,
 *  and it names the account's login shell without this spec guessing it from
 *  an environment the stand deliberately strips. */
function lastPaneShellKind(): string {
  const log = standBackendLogTail(2_000_000)
  const lines = log.split('\n').filter((l) => l.includes('pane launch decided'))
  const last = lines[lines.length - 1] ?? ''
  const m = last.match(/shell_kind=(\w+)/)
  return m?.[1] ?? 'unknown'
}

async function openFreshPane(page: import('@playwright/test').Page): Promise<string> {
  const t0 = Date.now()
  await page.keyboard.press('Control+Shift+P')
  const item = page.locator('.quick-connect__item').first()
  await expect(item).toContainText('Local shell')
  await page.keyboard.press('Enter')
  const pane = page.locator('#pane-2')
  await expect(pane).toBeVisible()
  return `pane-open:${Date.now() - t0}ms`
}

/** The whole guarded behaviour in one shape of shell startup: open a fresh
 *  pane, run the first command, and hold the block to its settled state
 *  within the bound of the command's own output. */
async function firstCommandCompletes(
  page: import('@playwright/test').Page,
  label: string,
): Promise<void> {
  await page.goto('/')
  await appReadyForInput(page)
  const tOpen = Date.now()
  const openMs = await openFreshPane(page)

  const pane = page.locator('#pane-2')
  // Submitted at the pane's own first prompt: the editor is the prompt.
  const editor = pane.locator('.nocx-editor-input')
  await expect(editor).toBeVisible({ timeout: 10_000 })
  await expect(editor).toBeFocused({ timeout: 10_000 })
  await expect(pane.getByRole('status', { name: CHECKING })).toBeHidden({ timeout: 10_000 })
  await editor.fill(COMMAND)
  await page.keyboard.press('Enter')

  // "The command ran", observable: its own stdout is in the pane's grid.
  const grid = pane.locator('[role="grid"][aria-label="Terminal output"]')
  await expect
    .poll(async () => (await grid.textContent()) ?? '', { timeout: 20_000 })
    .toContain('ruf5o-complete-done')
  const tOutput = Date.now()

  // THE ASSERTION, with both ends: from the command's output being visible,
  // the block settles inside a bound that is below every declared deadline —
  // a block still Running here is the 0.6.1 defect, however late the prompt
  // painted inside it.
  await expect(pane.locator('.cmd-block-running')).toHaveCount(0, {
    timeout: BOUND_AFTER_OUTPUT_MS,
  })
  const block = pane.locator('.cmd-block[data-block-kind="command"]').filter({ hasText: COMMAND })
  await expect(block).toHaveAttribute('data-outcome', 'success', {
    timeout: BOUND_AFTER_OUTPUT_MS,
  })
  // And the prompt is back for the next command.
  await promptReady(page)
  console.log(
    `[ruf5o-complete][${label}] shell=${lastPaneShellKind()} ${openMs} ` +
      `tab-open->output:${tOutput - tOpen}ms output->settled:${Date.now() - tOutput}ms`,
  )
}

test('a fresh pane completes its first command when the prompt returns (no user rc)', async ({
  page,
}) => {
  // The owner's decisive shape: an empty ZDOTDIR, no user rc at all — the
  // block still never settled on 0.6.1, so the fastest regression is the one
  // with nothing in front of the integration.
  const home = readStand().home
  for (const rc of ['.bashrc', '.zshrc', '.zprofile', '.zshenv', '.zlogin']) {
    rmSync(join(home, rc), { force: true })
  }
  await firstCommandCompletes(page, 'no-rc')
})

test('a fresh pane completes its first command with the widget-wrapping zsh plugin stack', async ({
  page,
}) => {
  // The composition an ordinary zsh has (nocx-1ebrs.27's three), loaded from
  // the user's own .zshrc before nocx's hooks — the order the launcher
  // produces for everyone who sources plugins the usual way.
  const plugins = process.env.NOCX_ZSH_PLUGINS ?? ''
  const shellKind = lastPaneShellKind()
  test.skip(
    !plugins || !existsSync(plugins),
    `the pinned zsh plugins are not installed (NOCX_ZSH_PLUGINS unset); ` +
      `provision them with scripts/install-zsh-zle-plugins.sh`,
  )
  test.skip(
    shellKind !== 'zsh',
    `this host's login shell is ${shellKind}, and a zsh plugin composition ` +
      `cannot be measured through it`,
  )

  const sources = PLUGIN_ENTRIES.map((entry) => join(plugins, entry)).filter((p) => existsSync(p))
  test.skip(
    sources.length !== PLUGIN_ENTRIES.length,
    `expected all three pinned plugins under ${plugins}, found ${sources.length}`,
  )

  const home = readStand().home
  writeFileSync(
    join(home, '.zshrc'),
    [
      '# An ordinary zsh setup: the widget-wrapping plugins most people have.',
      ...sources.map((p) => `. ${JSON.stringify(p)}`),
      '',
    ].join('\n'),
  )
  try {
    await firstCommandCompletes(page, 'plugins')
  } finally {
    rmSync(join(home, '.zshrc'), { force: true })
  }
})
