// macOS zsh regression (nocx-ruf5o): the FIRST command in a fresh pane
// completes when the prompt returns; it never stays Running with the new prompt
// inside the block. Linux e2e keeps its account shell as bash, so this spec is
// collected only on macOS, where missing zsh/plugins still fail the test.
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
// THE SHELL IS THE TEST ACCOUNT'S LOGIN SHELL. A local pane normally runs
// the account's own login shell (nothing in panes.create chooses one), and the
// resolver correctly asks the OS account record before SHELL. The macOS CI
// step and Linux container entry set that account shell to zsh before the
// backend/helper starts. The helper log assertion below proves the actual pane
// shell is zsh; missing zsh or plugins fails rather than silently measuring a
// different shell.
//
// THE EMPTY-ZDOTDIR SHAPE, MADE RATHER THAN HOPED FOR. The pane's zsh is
// launched through nocx's transient ZDOTDIR, which replays the USER's file
// of each phase from the ZDOTDIR the daemon itself saw — and the e2e home
// boundary strips ZDOTDIR from every backend it launches
// (e2e/home-isolation.ts), so the daemon has none and the user's files are
// replayed from the variant's own disposable HOME. The no-rc variant holds
// no rc files there at all (deleted below): the zsh nocx starts reads no
// user startup file, the owner's measured shape, identically on macOS and
// in the Linux image. The plugin variant's HOME holds exactly one .zshrc —
// the plugins — which the launcher replays after its own phases, the order
// everyone who sources plugins the usual way gets.
//
// THE SHELL KIND COMES FROM THE HELPER DAEMON'S OWN LOG, not the backend's.
// The "pane launch decided" line is logged by the process that actually
// spawns the shell — the helper daemon (internal/helper/session/spawn_local)
// — and that daemon writes `<home>/.nocx/helper.log`
// (cmd/nocx-helper/servelog.go), never the backend's own log. Both earlier
// shapes of this read looked in the stand's backend.log (once as a 2 MB tail,
// once as a whole-file read of a path that does not exist), so EVERY host
// printed shell=unknown in the timing line. VaultBackend.helperLogFile is
// the public accessor for exactly that path, and the line is now an
// ASSERTION: it must say zsh, which is the proof the named shell reached the
// daemon the pane actually ran.

import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

/** Where this spec's disposable homes are made. macOS $TMPDIR is
 *  /private/var/folders/<two random components>/T/ — 49 bytes before the
 *  home's own name — and a home is where this spec's backend binds its
 *  sockets (<home>/Library/Application Support/nocx-dev/run/*.sock): under
 *  $TMPDIR that came to 130 bytes against darwin's 103-byte sun_path limit
 *  on the first ci-mac run of this spec (39678155081), the server refused to
 *  serve, and every variant failed before a pane opened. Linux's /tmp is
 *  short, which is why the container runs never saw it. The Go suite's answer
 *  is /tmp on darwin for exactly this (storagetest.DisposableRoot,
 *  nocx-lvdj3): a real user's home is short, only a test's under $TMPDIR is
 *  not — mirrored here rather than invented a second time. */
function disposableRoot(): string {
  return process.platform === 'darwin' ? '/tmp' : tmpdir()
}

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

/** The shell both variants measure. CI and container setup set the account
 *  login shell to zsh; SHELL is set on the backend as documentation. */
const ZSH = '/bin/zsh'

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
 * exist before the first pane. 'unknown' is a REAL answer — the shell the
 * daemon was told to start is neither bash nor zsh — and is reported as
 * itself.
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
  // The app's own first pane is up — its launch line is the shell oracle the
  // assertion at the end reads — and it is what makes the pane this spec
  // opens the second one in the tab.
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
  // The helper log is the shell the pane really ran, and the assertion is
  // explicit: a line that says bash, or no line at all, is a regression in the
  // launch environment this spec is intended to prove.
  const kind = lastPaneShellKind(backend)
  expect(kind, `the pane launch decided line in ${backend.helperLogFile} must say zsh`).toBe('zsh')
  console.log(
    `[ruf5o-complete][${label}] shell=${kind} ${openMs} ` +
      `tab-open->output:${tOutput - tOpen}ms output->settled:${Date.now() - tOutput}ms`,
  )
}

test.describe('a fresh pane completes its first command (nocx-ruf5o)', () => {
  let home: DisposableRoot
  let backend: VaultBackend

  test.beforeEach(() => {
    home = { root: mkdtempSync(join(disposableRoot(), 'nocx-ruf5o-')) }
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
    // The owner's decisive shape: zsh reading no user rc at all — the block
    // still never settled on 0.6.1, so the fastest regression is the one with
    // nothing in front of the integration. This backend receives SHELL=/bin/zsh;
    // the account resolver also names zsh because CI/container setup changes it.
    // A missing /bin/zsh fails rather than letting another shell answer a
    // question about zsh. The helper log below proves which shell actually ran.
    expect(
      existsSync(ZSH),
      `${ZSH} is absent — this variant measures zsh; macOS ships it there and the e2e image installs it (e2e/Dockerfile)`,
    ).toBe(true)
    const endpoint = await backend.start({ SHELL: ZSH })
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
    // NOTHING HERE SKIPS, EVER AGAIN. The first ci-mac round skipped this
    // variant at a shell-kind precondition (the account logged in with bash)
    // and the owner rejected the run: a variant that skips on the one host
    // whose zsh coverage it exists to carry is a variant that proves nothing.
    // The plugins precondition FAILS naming the provisioning — ci-mac
    // installs them in this job and the e2e container's entry script does,
    // both from the one script that owns the pins — and the shell is NAMED
    // through this backend's SHELL and the account's zsh login shell, as in
    // the no-rc variant.
    const plugins = process.env.NOCX_ZSH_PLUGINS ?? ''
    expect(
      plugins && existsSync(plugins),
      `the pinned zsh plugins are not provisioned — run ` +
        `scripts/install-zsh-zle-plugins.sh <dir> and point NOCX_ZSH_PLUGINS at it ` +
        `(ci-mac's "Install the pinned widget-wrapping zsh plugins" step and ` +
        `e2e/container-entry.sh both do this)`,
    ).toBeTruthy()
    const sources = PLUGIN_ENTRIES.map((entry) => join(plugins, entry))
    const present = sources.filter((p) => existsSync(p))
    expect(
      present.length,
      `expected all three pinned plugins under ${plugins}, found ${present.length}: ${sources.join(', ')}`,
    ).toBe(PLUGIN_ENTRIES.length)

    expect(
      existsSync(ZSH),
      `${ZSH} is absent — this variant measures zsh; macOS ships it there and the e2e image installs it (e2e/Dockerfile)`,
    ).toBe(true)
    const endpoint = await backend.start({ SHELL: ZSH })
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
    // The app is already up and pane 1 already launched — against the plugins
    // rc, which is the stack. The measured pane is the fresh one; the shell
    // kind the pane really ran is asserted inside firstCommandCompletes, from
    // the helper's own launch line.
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
