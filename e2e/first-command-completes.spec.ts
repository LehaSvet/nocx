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
// closes the first block, in the two shapes the owner measured: a home with no
// user rc at all and the widget-wrapping zsh plugin stack.
//
// EACH VARIANT RUNS AGAINST ITS OWN BACKEND, IN ITS OWN HOME. The first shape
// of this spec reached its "no user rc" state by DELETING the rc files from
// the shared stand's home — and the shared stand is shared by every spec of
// the run, before and after this file: visual-board.spec.ts readFileSync's the
// stand's own .bashrc two hundred specs later, and the webkit 2 shard of this
// PR's CI run answered ENOENT (run 39660179212). A per-variant VaultBackend
// gives the pane's shell a HOME that belongs to this test alone, so "no user
// rc" is a fact about an owned home rather than a mutation of a shared one —
// and the guard test at the bottom of this file holds that invariant.
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
// WHERE EACH VARIANT RUNS, AND WHERE IT SKIPS — SAID OUT LOUD. A local pane
// runs the account's own login shell — nothing in panes.create chooses one —
// so the no-rc variant runs on every host (zsh on a stock macOS runner, bash
// in the Linux e2e image) and the plugin-stack variant runs exactly where both
// of its preconditions hold: the pinned plugins are installed
// (scripts/install-zsh-zle-plugins.sh, the same provisioning the Go
// composition test uses, which exports NOCX_ZSH_PLUGINS to the whole CI job)
// AND the login shell is zsh, read from the backend's own launch decision.
// Anywhere else it SKIPS, and the skip reason names which precondition
// failed: without the plugins there is no stack to wrap with, and a zsh
// plugin composition measured through bash is not evidence of anything. The
// shell-kind precondition is why this variant brings its app up before it can
// decide — the decision it reads is a pane launch the backend already made,
// not a guess from $SHELL, which the harness strips on purpose.
//
// THE SHELL KIND COMES FROM THE HELPER DAEMON'S OWN LOG, not the backend's.
// The "pane launch decided" line is logged by the process that actually
// spawns the shell — the helper daemon (internal/helper/session/spawn_local)
// — and that daemon writes `<home>/.nocx/helper.log`
// (cmd/nocx-helper/servelog.go), never the backend's own log. Both earlier
// shapes of this read looked in the stand's backend.log (once as a 2 MB tail,
// once as a whole-file read of a path that does not exist), so EVERY host
// printed shell=unknown in the timing line — and the plugin variant, whose
// skip decision read the same function, skipped on ci-mac, the one host whose
// zsh coverage this spec exists to carry. VaultBackend.helperLogFile is the
// public accessor for exactly that path, and the app's own first pane — the
// one it opens on connect — is the launch whose kind is read before this
// spec's pane exists.

import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import {
  appReadyForInput,
  bindEndpoint,
  endDaemonsUnder,
  promptReady,
  VaultBackend,
  type BackendEndpoint,
  type DisposableRoot,
  test,
  expect,
} from './harness'
import { readStand } from './stand'

const COMMAND = 'echo ruf5o-complete-done'
/** Below every declared deadline (carrier 3 s, pane screen / tool surface
 *  5 s), above honest pty/renderer jitter — the same bound the
 *  first-command-delay spec holds the start interval to. */
const BOUND_AFTER_OUTPUT_MS = 2_500
const CHECKING = 'Checking this shell…'
/** The rc files a user's shell may read before nocx's hooks install — the
 *  same list the first shape of this spec removed, now removed from a home
 *  this test owns instead of the shared stand's. */
const USER_RC_FILES = ['.bashrc', '.zshrc', '.zprofile', '.zshenv', '.zlogin']

/** The plugin entries, pinned by nocx-1ebrs.27's commit pins; the entry file
 *  sits at each checkout's root. Mirrors the Go fixture
 *  (zsh_parent_block_close_test.go) so the wrapping measured here is the
 *  wrapping that test composes with. */
const PLUGIN_ENTRIES = [
  'fast-syntax-highlighting/fast-syntax-highlighting.plugin.zsh',
  'zsh-syntax-highlighting/zsh-syntax-highlighting.zsh',
  'zsh-autosuggestions/zsh-autosuggestions.zsh',
]

/**
 * The shell kind of this backend's most recent pane launch, from the helper
 * daemon's own log — the process that spawns the shell is the one that says
 * which shell it chose, so this is the backend's launch decision and not a
 * guess from an environment the harness deliberately strips.
 *
 * '' (not 'unknown') while no launch has been logged yet: the file may not
 * exist before the first pane. 'unknown' is a REAL answer — the account's
 * login shell is neither bash nor zsh — and is reported as itself.
 */
function lastPaneShellKind(backend: VaultBackend): string {
  try {
    const log = readFileSync(backend.helperLogFile, 'utf8')
    const lines = log.split('\n').filter((l) => l.includes('pane launch decided'))
    const last = lines[lines.length - 1] ?? ''
    return last.match(/shell_kind=(\w+)/)?.[1] ?? ''
  } catch {
    return ''
  }
}

/**
 * The account's login shell as THIS backend decided it, from its own first
 * pane — the one the app opens on connect. Waits for that launch to be logged
 * rather than assuming a moment: the pane exists when it exists, and the
 * observable is the helper's own line, never a duration.
 */
async function loginShellKind(backend: VaultBackend): Promise<string> {
  await expect
    .poll(() => lastPaneShellKind(backend), {
      timeout: 15_000,
      message: `a pane launch decided line in this backend's helper log (${backend.helperLogFile})`,
    })
    .not.toBe('')
  return lastPaneShellKind(backend)
}

/** Point the page at this test's backend and bring the app up on it. */
async function reachApp(
  page: import('@playwright/test').Page,
  endpoint: BackendEndpoint,
): Promise<void> {
  await bindEndpoint(page, endpoint)
  await page.goto('/')
  await appReadyForInput(page)
}

async function openFreshPane(page: import('@playwright/test').Page): Promise<string> {
  // The app's own first pane is up — it is the login-shell oracle, and it is
  // what makes the pane this spec opens the second one in the tab.
  await expect(page.locator('.pane')).toHaveCount(1, { timeout: 15_000 })
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
 *  within the bound of the command's own output. The app is already up —
 *  reachApp happened, so the login-shell oracle is readable and pane 1 is the
 *  app's own. */
async function firstCommandCompletes(
  page: import('@playwright/test').Page,
  label: string,
  backend: VaultBackend,
): Promise<void> {
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
  // The timing line names the shell the pane really ran. It is ASSERTED, not
  // logged blind: a line that prints nothing where the shell belongs once
  // read as a formatting nit while the plugin variant skipped on the truth of
  // it.
  const kind = lastPaneShellKind(backend)
  expect(kind, `a pane launch decided line in ${backend.helperLogFile}`).not.toBe('')
  console.log(
    `[ruf5o-complete][${label}] shell=${kind} ${openMs} ` +
      `tab-open->output:${tOutput - tOpen}ms output->settled:${Date.now() - tOutput}ms`,
  )
}

test.describe('a fresh pane completes its first command (nocx-ruf5o)', () => {
  let home: DisposableRoot
  let backend: VaultBackend

  test.beforeEach(() => {
    home = { root: mkdtempSync(join(tmpdir(), 'nocx-ruf5o-')) }
    backend = new VaultBackend(readStand().server, home)
  })

  test.afterEach(async () => {
    backend?.stop()
    let isolatedHome = ''
    try {
      isolatedHome = backend.isolatedHome
    } catch {
      /* the backend never started, so it started no daemon either */
    }
    // The daemon BEFORE the root: it holds a PTY under this home, and a
    // daemon still running is reported by endDaemonsUnder rather than
    // silently orphaned by the rmSync below it.
    await endDaemonsUnder(isolatedHome)
    rmSync(home.root, { recursive: true, force: true })
  })

  test('a fresh pane completes its first command when the prompt returns (no user rc)', async ({
    page,
  }) => {
    // The owner's decisive shape: a home with no user rc at all — the block
    // still never settled on 0.6.1, so the fastest regression is the one with
    // nothing in front of the integration.
    const endpoint = await backend.start()
    // EnsureInstalled wrote nocx's own gate lines into this home's .zshrc and
    // .bashrc on the way up; the shape under test has no user rc in front of
    // the hooks at all, so they go — from THIS test's home, which is the
    // entire difference from the first shape of this spec (it deleted them
    // from the stand's shared home, and visual-board read the hole two
    // hundred specs later).
    for (const rc of USER_RC_FILES) {
      rmSync(join(backend.isolatedHome, rc), { force: true })
    }
    await reachApp(page, endpoint)
    await firstCommandCompletes(page, 'no-rc', backend)
  })

  test('a fresh pane completes its first command with the widget-wrapping zsh plugin stack', async ({
    page,
  }) => {
    // The composition an ordinary zsh has (nocx-1ebrs.27's three), loaded from
    // the user's own .zshrc before nocx's hooks — the order the launcher
    // produces for everyone who sources plugins the usual way.
    //
    // The plugins precondition is checked BEFORE a backend is started,
    // because it needs no backend: on the containers, where NOCX_ZSH_PLUGINS
    // is never set, this is the whole cost of the skip. The shell-kind
    // precondition is checked after the app is up — see loginShellKind.
    const plugins = process.env.NOCX_ZSH_PLUGINS ?? ''
    test.skip(
      !plugins || !existsSync(plugins),
      `the pinned zsh plugins are not installed (NOCX_ZSH_PLUGINS unset); ` +
        `provision them with scripts/install-zsh-zle-plugins.sh`,
    )
    const sources = PLUGIN_ENTRIES.map((entry) => join(plugins, entry)).filter((p) => existsSync(p))
    test.skip(
      sources.length !== PLUGIN_ENTRIES.length,
      `expected all three pinned plugins under ${plugins}, found ${sources.length}`,
    )

    const endpoint = await backend.start()
    // THIS home's .zshrc is the user rc the pane's zsh replays: the plugins,
    // and nothing else. Overwriting the gate line EnsureInstalled wrote is
    // the measured shape — the hooks arrive through the launcher's own
    // composed ZDOTDIR, after the user's file, which is the order under test.
    writeFileSync(
      join(backend.isolatedHome, '.zshrc'),
      [
        '# An ordinary zsh setup: the widget-wrapping plugins most people have.',
        ...sources.map((p) => `. ${JSON.stringify(p)}`),
        '',
      ].join('\n'),
    )
    await reachApp(page, endpoint)
    const shellKind = await loginShellKind(backend)
    test.skip(
      shellKind !== 'zsh',
      `this host's login shell is ${shellKind}, and a zsh plugin composition ` +
        `cannot be measured through it`,
    )
    // The app is already up and pane 1 already launched — against the plugins
    // rc, which is the stack. The measured pane is the fresh one.
    await firstCommandCompletes(page, 'plugins', backend)
  })

  test('the shared stand home still holds its rc files after the variants ran', () => {
    // THE INVARIANT THE OWN-BACKEND REDESIGN BOUGHT, held as an assertion
    // rather than a memory: visual-board.spec.ts reads the stand's .bashrc
    // readFileSync-first, so a variant that deletes it turns a later shard
    // red with ENOENT — measured on this PR's webkit 2 shard
    // (run 39660179212). Runs after the variants by declaration order: tests
    // in a file share one worker and run in order, so this sees the variants'
    // aftermath, not their intent.
    const stand = readStand()
    for (const rc of ['.bashrc', '.zshrc']) {
      const p = join(stand.home, rc)
      expect(existsSync(p), `${p} — the stand's own ${rc}, deleted by an earlier variant`).toBe(
        true,
      )
    }
  })
})
