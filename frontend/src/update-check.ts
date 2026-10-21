/**
 * update-check — the one owner of what the last update check did
 * (nocx-1ebrs.36).
 *
 * 0.6.0 checked for updates at startup and every 24 hours, and swallowed
 * every failure in a bare catch: a dead check looked exactly like "no
 * update", which is how a shipped build went a whole release without
 * offering the next one. This module owns that outcome for both callers
 * (the startup check and the interval) so there is one state to read, not
 * two that agree until they don't: an available update still goes to the
 * notice, and a failure is kept with its cause — logged at warn and shown
 * in About — rather than dropped.
 */

import { createSignal, type Accessor } from 'solid-js'

import { CheckForUpdate } from '../bindings/github.com/shady2k/nocx/wailsapp'
import { log } from './log'
import type { UpdateNoticeController } from './update-notice'

export interface UpdateCheckController {
  /** Why the last check failed, or null when it succeeded or has not run.
   *  An accessor, so the About card is live: the 24-hour re-check that
   *  succeeds clears a stale failure without anybody re-rendering by hand. */
  error: Accessor<string | null>
  /** Run one check. Safe to call repeatedly; the interval does exactly that. */
  run: () => Promise<void>
}

export function createUpdateCheck(
  notice: Pick<UpdateNoticeController, 'showAvailable'>,
): UpdateCheckController {
  const [error, setError] = createSignal<string | null>(null)

  const run = async (): Promise<void> => {
    try {
      const info = await CheckForUpdate()
      setError(null)
      if (info) notice.showAvailable(info.Version, info.NotesURL)
    } catch (err) {
      const reason = err instanceof Error ? err.message : String(err)
      setError(reason)
      log.warn('update check failed', { reason })
    }
  }

  return { error, run }
}
