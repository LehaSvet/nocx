/**
 * update-schedule — WHEN the update check runs (nocx-1ebrs.66.1.1).
 *
 * Before this module the check ran at startup and every 24 hours, so a window
 * kept open for days could sit through a whole release without hearing about
 * it: the person who needs an update most is the one who never closes the app.
 * The owner decided the rule: startup, on wake from sleep and on window focus —
 * at most once an hour — plus every four hours.
 *
 * ONE MECHANISM, NOT TWO. "At most once an hour" is stated as an interval
 * since the last check, and every trigger asks the same question through it:
 * the startup call, the wake signal, the four-hourly tick. A rule written as
 * two (a throttle for focus and an interval for the timer) is two rules that
 * agree until somebody changes one; this way the four-hour tick is simply a
 * trigger that always finds the hour elapsed.
 *
 * A FAILED CHECK IS STILL A CHECK. The timestamp is taken before the call, so
 * a broken network is not re-probed on every focus and every wake. What
 * happened is not lost: `update-check.ts` owns that outcome and shows it in
 * About and in the log.
 *
 * ONE CHECK IN FLIGHT. A focus arriving while a download-sized check is still
 * running must not start a second one — the second answer would race the first
 * into the same announcement.
 */
import { log } from './log'
import { documentWakeSource, type WakeSource } from './wake-source'

/** An hour between automatic checks, from whichever trigger fires first. */
export const HOUR_MS = 60 * 60 * 1000

/** The floor: even a window nobody touches checks again this often. */
export const PERIOD_MS = 4 * HOUR_MS

/** The two timer calls this module needs. Injected so a test drives time
 *  without waiting for it (the repository's rule: no test depends on timing —
 *  which this module's own behaviour is entirely about). */
export interface UpdateScheduleTimers {
  setInterval(handler: () => void, ms: number): unknown
  clearInterval(handle: unknown): void
}

const globalTimers: UpdateScheduleTimers = {
  setInterval: (handler, ms) => globalThis.setInterval(handler, ms),
  clearInterval: (handle) => globalThis.clearInterval(handle as ReturnType<typeof setInterval>),
}

export interface UpdateScheduleDeps {
  /** One update check — `updateCheck.run()`. It owns its own outcome, so a
   *  failure it reports never reaches this module as a rejection. A plain
   *  value is allowed as well as a promise: the schedule waits for whichever
   *  it gets, so a caller with a synchronous check needs no wrapper. */
  check: () => unknown
  /** Where "the machine came back" comes from. Defaults to the document's own
   *  signal, which the pane wake reporter reads through the same owner. */
  wake?: WakeSource
  timers?: UpdateScheduleTimers
  now?: () => number
  /** Overridable for tests; production takes the two constants above. */
  throttleMs?: number
  periodMs?: number
}

/**
 * Start the schedule and return its stop function.
 *
 * The first check is due immediately: the timestamp starts at negative
 * infinity rather than at zero, because zero is a real reading on an injected
 * clock and would swallow the startup check on a machine whose clock is young.
 */
export function startUpdateSchedule(deps: UpdateScheduleDeps): () => void {
  const wake = deps.wake ?? documentWakeSource
  const timers = deps.timers ?? globalTimers
  const now = deps.now ?? (() => Date.now())
  const throttleMs = deps.throttleMs ?? HOUR_MS
  const periodMs = deps.periodMs ?? PERIOD_MS

  let lastCheck = Number.NEGATIVE_INFINITY
  let inFlight = false
  let stopped = false

  const runDue = (): void => {
    if (stopped || inFlight) return
    const at = now()
    if (at - lastCheck < throttleMs) return
    // Taken BEFORE the call: a check that fails is a check that happened.
    lastCheck = at
    inFlight = true
    void (async () => {
      try {
        await deps.check()
      } catch (err) {
        // `check` owns its own failures, so this is only reachable when the
        // check threw before it could record anything. Never swallow it: a
        // schedule that quietly stopped working is a soft degrade nobody sees.
        log.warn('update schedule: the check threw outside its outcome owner', {
          reason: err instanceof Error ? err.message : String(err),
        })
      } finally {
        inFlight = false
      }
    })()
  }

  runDue()
  const unsubscribe = wake.subscribe(runDue)
  const handle = timers.setInterval(runDue, periodMs)

  return () => {
    if (stopped) return
    stopped = true
    unsubscribe()
    timers.clearInterval(handle)
  }
}
