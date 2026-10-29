/**
 * "The machine came back" — one owner for the wake signal
 * (nocx-1ebrs.66.1.1).
 *
 * Two things need the same answer and used to derive it twice: the pane wake
 * reporter (`wake-report.ts`, a diagnostic) observes what the panes looked
 * like after a suspend, and the update check has to run again when the window
 * comes back — a window kept open for days otherwise waits for an interval
 * measured in days before it hears about a release. Two listener pairs for one
 * fact is the arrangement AGENTS.md forbids: they agree everywhere anyone
 * looks and disagree at the moment that matters, and a wake delivers
 * `visibilitychange` and `focus` within a frame or two of each other, so the
 * pair that coalesced and the pair that did not would disagree about how many
 * wakes there were.
 *
 * The signal is deliberately narrow: "the window may have just been hidden and
 * shown again, or may have just regained focus". A consumer decides what that
 * means for it (the reporter coalesces duplicate signals; the update schedule
 * throttles by an hour), because the meaning is the consumer's and not the
 * source's.
 */

/** A source of wake events. The DOM's own; injected so a test can drive it. */
export interface WakeSource {
  subscribe(onWake: () => void): () => void
}

/**
 * The browser's account of coming back: the document becoming visible again,
 * and the window regaining focus. Both fire on a lid opening and neither is
 * reliable alone, so both are reported and the consumer coalesces.
 */
export const documentWakeSource: WakeSource = {
  subscribe(onWake: () => void): () => void {
    const onVisibility = () => {
      if (document.visibilityState === 'visible') onWake()
    }
    document.addEventListener('visibilitychange', onVisibility)
    window.addEventListener('focus', onWake)
    return () => {
      document.removeEventListener('visibilitychange', onVisibility)
      window.removeEventListener('focus', onWake)
    }
  },
}
