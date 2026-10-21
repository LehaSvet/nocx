/**
 * e2e: a local pane's shell outlives the coordinator that opened it
 * (nocx-ie23r.5 — the last criterion of "quit nocx mid-build, open it again").
 *
 * WHAT A USER CAN DO THAT THEY COULD NOT BEFORE, and it is what this spec
 * watches: start something long in a local pane, quit nocx entirely, open it
 * again, and the SAME shell is there — same process, same session, the output
 * continuing where it left off, and the pane the pipe of it.
 *
 * # Why this is an e2e spec and not a Go test
 *
 * The criterion names `cmd/nocx-server`, the SHIPPED coordinator, and the
 * reason is the one internal/coordinator/launcher_test.go wrote down: building
 * that binary from a unit test makes the unit suite depend on a compiler run,
 * and its local helper artifact (`make helper-local`, which needs the pinned
 * Zig) is a build prerequisite of every target that produces a runnable
 * binary. This suite is where that binary exists and where a backend can be
 * restarted mid-test. The MECHANISM has a gate that runs everywhere —
 * internal/app/restart_screen_test.go, over the shipped App, two instances,
 * one home, a real helper and a real PTY — and this is the journey.
 *
 * # What is read, and from where
 *
 * The pid comes off the OS: the program prints its own pid, and Node asks the
 * kernel whether that pid is still there (`process.kill(pid, 0)`). Nothing
 * here trusts a record nocx wrote about itself, and the tick counter is the
 * second half of the same claim — a pid that is alive and a program that has
 * stopped printing are different failures, and a zombie would satisfy the
 * first alone.
 *
 * The screen is read through the painter-owned cell model, not replayed from
 * raw terminal bytes. The tick counter and pid remain the program's own
 * observable output, and the page reads them from the live screen after each
 * coordinator start. After restart, the PID line may have scrolled out of view;
 * process identity is checked at the OS and through the session/pane pairing.
 */
import { expect, type Page } from '@playwright/test'
import { spawnSync } from 'node:child_process'
import { mkdtempSync } from 'node:fs'
import { join } from 'node:path'
import { tmpdir } from 'node:os'
import {
  standalone as base,
  VaultBackend,
  bindEndpoint,
  endDaemonsUnder,
  openControlPlane,
  promptReady,
  type DisposableRoot,
} from './harness'
import { createHomeIsolation } from './home-isolation'
import { readStand } from './stand'

/** Lazily: the stand is started by globalSetup, which runs after Playwright
 *  has collected this file. */
const serverBin = () => readStand().server

const TABS = '.nocx-tab'

interface LiveSession {
  sessionId: string
  paneId: string | null
}

const test = base

/**
 * The program the pane runs, and it is written so BOTH facts are its own:
 * `$$` is the pid the shell reports for itself, and the tick is a number that
 * can only grow while it keeps running.
 *
 * The marker is split across the shell's own string concatenation so the pty's
 * ECHO of the typed line cannot be mistaken for the program's output — the
 * same trick internal/app/local_pane_test.go uses, and for the same reason.
 */
const TICKER =
  'echo "NOCXJOB=""$$"; i=0; while :; do i=$((i+1)); echo "NOCXTICK=""$i"; sleep 0.2; done'

/** Read text from the painter-owned live-screen surface. */
async function screenText(page: Page): Promise<string> {
  return (await page.locator('.pane.active .terminal-live-grid').textContent()) ?? ''
}

/** The pid the program reported for itself, and the highest tick so far. */
function progress(text: string): { pid: number; tick: number } {
  const pid = /NOCXJOB=(\d+)/.exec(text)
  let tick = 0
  for (const match of text.matchAll(/NOCXTICK=(\d+)/g)) {
    tick = Math.max(tick, Number(match[1]))
  }
  return { pid: pid ? Number(pid[1]) : 0, tick }
}

/**
 * Is this pid there, asked of the KERNEL?
 *
 * `process.kill(pid, 0)` sends no signal and reports whether the process
 * exists, which is the criterion's "read the pid from the operating system"
 * literally: nothing here consults a file, a log or a record nocx keeps.
 */
function aliveFromTheOS(pid: number): boolean {
  try {
    process.kill(pid, 0)
    return true
  } catch {
    return false
  }
}

test.describe('a local pane survives the coordinator being replaced (nocx-ie23r.5)', () => {
  let home: DisposableRoot
  let backend: VaultBackend

  test.beforeEach(() => {
    home = { root: mkdtempSync(join(tmpdir(), 'nocx-local-reattach-')) }
    backend = new VaultBackend(serverBin(), home)
  })

  test.afterEach(async () => {
    backend?.stop()
    let isolatedHome = ''
    try {
      isolatedHome = backend.isolatedHome
    } catch {
      /* the backend never started, so it started no daemon either */
    }
    await endDaemonsUnder(isolatedHome)
  })

  test('the same shell is still running in the pane after the coordinator is replaced', async ({
    page,
  }) => {
    const ep1 = await backend.start()
    await bindEndpoint(page, ep1)
    await page.goto('/')
    await promptReady(page)
    await expect(page.locator(TABS)).toHaveCount(1, { timeout: 30_000 })

    // The pane the renderer opened. Its id is the renderer's and the session id
    // is the backend's (AD-7), and the criterion is about the SECOND: the shell
    // the daemon minted must be the one the replacement takes back, keyed to
    // the pane it was the pipe of.
    const wire1 = await openControlPlane(ep1.port, ep1.token)
    const live1 = (await wire1.call('sessions.live', {})) as { sessions: LiveSession[] }
    expect(live1.sessions).toHaveLength(1)
    const { sessionId, paneId } = live1.sessions[0]

    // START THE LONG-RUNNING PROGRAM IN THE LOCAL PANE, the way a person does.
    await page.keyboard.type(TICKER)
    await page.keyboard.press('Enter')

    // WAIT ON THE OBSERVABLE, never on a duration: the pid the program printed
    // for itself and at least one tick beside it.
    await expect
      .poll(async () => progress(await screenText(page)).pid, { timeout: 30_000 })
      .toBeGreaterThan(0)
    const before = progress(await screenText(page))
    expect(before.tick).toBeGreaterThan(0)
    expect(aliveFromTheOS(before.pid)).toBe(true)

    // Let it run past the point where the first ticks were read, so "the output
    // continues" afterwards is a claim about NEW bytes rather than a repeat of
    // what was already recorded.
    const seen = before.tick
    await expect
      .poll(async () => progress(await screenText(page)).tick, { timeout: 30_000 })
      .toBeGreaterThan(seen)

    // ── THE COORDINATOR IS REPLACED, and nothing of it survives ────────────
    //
    // The first control plane is CLOSED first: it points at a port the next
    // coordinator will not have, and a socket left open across the restart
    // would be a reader of a process that is gone.
    wire1.close()
    const ep2 = await backend.restart()
    await bindEndpoint(page, ep2)
    await page.reload()
    await promptReady(page)

    // THE PANE CAME BACK ON THE SAME SESSION, listed by the NEW coordinator and
    // still keyed to the pane it was the pipe of. A replacement that opened a
    // fresh shell instead would answer with a different session id here (and no
    // further ticks below).
    const wire2 = await openControlPlane(ep2.port, ep2.token)
    const live2 = (await wire2.call('sessions.live', {})) as { sessions: LiveSession[] }
    expect(live2.sessions).toHaveLength(1)
    expect(live2.sessions[0].sessionId).toBe(sessionId)
    expect(live2.sessions[0].paneId).toBe(paneId)

    // THE SAME PROCESS, read from the operating system, and still producing:
    // the OS proves the original pid is alive, and the accepted-frame screen
    // proves the ticker advanced after the replacement. Its older PID line may
    // have scrolled out of the visible viewport by now.
    expect(aliveFromTheOS(before.pid)).toBe(true)
    await expect
      .poll(async () => progress(await screenText(page)).tick, { timeout: 30_000 })
      .toBeGreaterThan(seen)

    // ── A SECOND COORDINATOR FOR THE SAME GENERATION IS REFUSED ───────────
    //
    // Two nocx against one home is the documented refusal and not a race: the
    // app-directory lock makes the second one exit 3 — "another nocx-server
    // already holds this app directory", which a launcher reads as a daemon
    // already running and not as a crash to recover from — and the coordinator
    // that IS serving goes on serving: the same session, the same pane, the
    // same process. What a second one must never do is raise a second helper
    // beside the daemon holding the shells, and the session still being spoken
    // for by its own pid is the observable that says it did not.
    const second = spawnSync(serverBin(), [], {
      env: createHomeIsolation({ inheritedEnv: process.env, root: home.root })
        .env as NodeJS.ProcessEnv,
      timeout: 60_000,
      encoding: 'utf8',
    })
    expect(second.status).toBe(3)
    expect((await wire2.call('sessions.live', {})) as { sessions: LiveSession[] }).toMatchObject({
      sessions: [{ sessionId, paneId }],
    })
    expect(aliveFromTheOS(before.pid)).toBe(true)

    wire2.close()
  })
})
