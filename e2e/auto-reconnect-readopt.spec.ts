/**
 * A page reconnects to a restarted backend and reattaches its live pane
 * without a user action (nocx-xn63t.6.33).
 */
import { expect } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import {
  standalone as test,
  VaultBackend,
  bindResolvableEndpoint,
  openControlPlane,
  promptReady,
  type BackendEndpoint,
  type DisposableRoot,
} from './harness'
import { readStand } from './stand'

const serverBin = () => readStand().server
/**
 * The program the pane runs, and it prints BOTH facts on every line: the tick,
 * which can only grow while it keeps running, and its own pid (`$$`), which is
 * what makes "the same program" checkable from the screen at any moment rather
 * than only while its first line happens to still be visible. The ticker this
 * is modelled on (e2e/local-session-reattach.spec.ts) prints the pid once, and
 * that spec answers the scroll problem by not comparing it at all after a
 * restart; printing it every time buys the comparison back.
 *
 * The marker is split across the shell's own string concatenation so the pty's
 * ECHO of the typed line cannot be mistaken for the program's output — the
 * same trick internal/app/local_pane_test.go uses, and for the same reason.
 */
const ticker =
  'echo "NOCXJOB=""$$"; i=0; while :; do i=$((i+1)); echo "NOCXTICK=""$i"" PID=""$$"; sleep 0.2; done'

interface LiveSession {
  sessionId: string
  paneId: string | null
}

async function screenText(page: import('@playwright/test').Page): Promise<string> {
  return (await page.locator('.pane.active .terminal-live-grid').textContent()) ?? ''
}

/** The highest tick on screen, and the pid the program printed beside it — the
 *  LAST such line, which is the newest byte the pane drew. Reading the pid off
 *  the newest tick rather than off the one-time `NOCXJOB=` line is what keeps
 *  this readable after the grid has scrolled. */
function progress(text: string): { pid: number; tick: number } {
  let pid = 0
  let tick = 0
  for (const match of text.matchAll(/NOCXTICK=(\d+) PID=(\d+)/g)) {
    tick = Math.max(tick, Number(match[1]))
    pid = Number(match[2])
  }
  return { pid, tick }
}

/** The pids of this machine's helper daemons started under one home. */
function daemonsFor(isolatedHome: string): number[] {
  try {
    return execFileSync('pgrep', ['-f', join(isolatedHome, '.nocx', 'helper')], {
      encoding: 'utf8',
    })
      .split('\n')
      .map((line) => Number(line.trim()))
      .filter((pid) => Number.isInteger(pid) && pid > 0)
  } catch {
    // pgrep exits non-zero when nothing matches, which is an answer.
    return []
  }
}

/** Whether every helper daemon under a home has gone, bounded by a deadline. */
async function daemonsGone(isolatedHome: string, withinMs: number): Promise<boolean> {
  const deadline = Date.now() + withinMs
  for (;;) {
    if (daemonsFor(isolatedHome).length === 0) return true
    if (Date.now() > deadline) return false
    const { promise, resolve: resume } = Promise.withResolvers<void>()
    setTimeout(resume, 50)
    await promise
  }
}

/**
 * End the helper daemons started under one home, the way the Go tests end
 * theirs — and for the same reason: the daemon is DETACHED by design (D1) and
 * nothing retires a generation yet, so a spec that stopped only its own
 * `nocx-server` would leave a process holding a PTY under a home the run is
 * finished with.
 *
 * SIGTERM FIRST AND SIGKILL ONLY FOR WHAT IGNORED IT, and each phase is WAITED
 * FOR on the OS's own answer — is this pid still there — rather than on a
 * duration, so a daemon that will not go is reported rather than left behind.
 * Same two steps, and the same reason, as e2e/local-session-reattach.spec.ts,
 * which is where they were bought.
 *
 * A FUNCTION RATHER THAN AN INLINE LOOP because `no-unsafe-finally` refuses a
 * literal `throw` in the caller's `finally`: written there it reads as a
 * deliberate replacement of whatever the body threw, and a leaked daemon is
 * not a verdict on this spec's subject. As a call, the cleanup's own failure
 * is reported as itself.
 */
async function endDaemonsUnder(isolatedHome: string): Promise<void> {
  if (isolatedHome === '') return
  for (const signal of ['SIGTERM', 'SIGKILL'] as const) {
    const survivors = daemonsFor(isolatedHome)
    if (survivors.length === 0) return
    for (const pid of survivors) {
      try {
        process.kill(pid, signal)
      } catch {
        /* already gone */
      }
    }
    if (await daemonsGone(isolatedHome, 15_000)) return
  }
  throw new Error(
    `this spec's helper daemon(s) ${daemonsFor(isolatedHome).join(', ')} outlived SIGTERM and SIGKILL`,
  )
}

function aliveFromTheOS(pid: number): boolean {
  try {
    process.kill(pid, 0)
    return true
  } catch {
    return false
  }
}

test('reconnects and reattaches a live pane after backend restart without a click', async ({
  page,
}) => {
  test.setTimeout(120_000)
  const home: DisposableRoot = { root: mkdtempSync(join(tmpdir(), 'nocx-auto-reconnect-')) }
  const backend = new VaultBackend(serverBin(), home)
  let endpoint: BackendEndpoint | null = null
  let isolatedHome = ''
  try {
    endpoint = await backend.start()
    await bindResolvableEndpoint(page, async () => {
      if (!endpoint) {
        return {
          ok: false,
          kind: 'no-server',
          message: 'The backend is unavailable.',
          remedy: 'Retry when the backend is available.',
        }
      }
      return {
        ok: true,
        host: '127.0.0.1',
        port: endpoint.port,
        token: endpoint.token,
        kind: '',
        message: '',
        remedy: '',
      }
    })
    await page.goto('/')
    await promptReady(page)
    await expect(page.locator('.nocx-tab')).toHaveCount(1, { timeout: 30_000 })

    const firstWire = await openControlPlane(endpoint.port, endpoint.token)
    const live = (await firstWire.call('sessions.live', {})) as { sessions: LiveSession[] }
    expect(live.sessions).toHaveLength(1)
    const sessionId = live.sessions[0]!.sessionId
    const paneId = live.sessions[0]!.paneId
    firstWire.close()

    await page.keyboard.type(ticker)
    await page.keyboard.press('Enter')
    await expect
      .poll(async () => progress(await screenText(page)).pid, { timeout: 30_000 })
      .toBeGreaterThan(0)
    const before = progress(await screenText(page))
    expect(before.tick).toBeGreaterThan(0)
    expect(aliveFromTheOS(before.pid)).toBe(true)

    // Replace the coordinator while leaving the renderer and its socket open.
    endpoint = await backend.restart()

    // No click, Retry, reload or endpoint rebinding: the renderer's provider
    // returns the new endpoint on its own next automatic attempt.
    await expect(page.locator('dialog.ui-connection-overlay')).not.toHaveAttribute('open', {
      timeout: 30_000,
    })
    await expect
      .poll(async () => progress(await screenText(page)).tick, { timeout: 30_000 })
      .toBeGreaterThan(before.tick)
    // THE SAME PROGRAM IS STILL PRINTING. The pid rides every tick line, so
    // this is the pid beside the NEWEST tick — the one the poll above waited
    // for — and not a first line that happened to stay on screen. A pane that
    // had opened a fresh shell would print a different one, however alive the
    // old process still is on the helper.
    const after = progress(await screenText(page))
    expect(after.pid).toBe(before.pid)
    // And the OS agrees that pid is still there — the program was never
    // restarted, whatever the screen says.
    expect(aliveFromTheOS(before.pid)).toBe(true)
    await expect(page.locator('body')).not.toContainText(/The connection is gone/)

    // THE SESSION THE REPLACEMENT RE-ADOPTED IS THE ONE THE PANE WAS THE PIPE
    // OF — same session id, same pane. The entry carries the whole identity
    // (instanceId, sessionEpoch, replayFrom, attached), and objectContaining
    // is how the two fields this spec is about are named without asserting the
    // rest of the backend's own answer.
    const secondWire = await openControlPlane(endpoint.port, endpoint.token)
    const adopted = (await secondWire.call('sessions.live', {})) as { sessions: LiveSession[] }
    expect(adopted.sessions as unknown[]).toContainEqual(
      expect.objectContaining({ sessionId, paneId }),
    )
    secondWire.close()
  } finally {
    backend.stop()
    try {
      isolatedHome = backend.isolatedHome
    } catch {
      // The backend did not start, so it started no helper daemon.
    }
    await endDaemonsUnder(isolatedHome)
  }
})
