/**
 * update-toast — the actionable announcement of a newer build
 * (nocx-1ebrs.54).
 *
 * The check (update-check.ts) owns WHAT the last check did; the strip
 * (update-notice.tsx) owns the quiet marker that stays in the tab bar. This
 * module owns the moment a newer build is ANNOUNCED and answered: a kit
 * toast, bottom-right, carrying the release notes link and the two decisions
 * a person has — Install now, or Later.
 *
 * The dismissal rule is the bead's own and lives here, once: Later silences
 * only that version, for this app run, so the 24-hour recheck does not
 * re-interrupt the person about the version they already declined, while a
 * genuinely newer build still announces itself. The strip keeps showing the
 * declined version, which is what makes the dismissal quiet rather than a
 * loss.
 *
 * Restart now launches the installed app before the current process quits. The
 * new process resolves the existing coordinator, which keeps its live sessions
 * across a compatible update (nocx-1ebrs.53).
 */

import { ApplyUpdate, RestartWindow } from '../bindings/github.com/shady2k/nocx/wailsapp'
import { showToast, dismissToast } from './ui/toast'

/** What the announcement needs from its embedding. The ApplyUpdate path is
 *  the existing one — this module adds no second update mechanism. */
export interface UpdateToastDeps {
  /** Downloads and applies the checked update. */
  apply?: () => Promise<void>
  /** Restarts the window through its native shell binding. */
  restart?: () => void | Promise<void>
}

export interface UpdateToastController {
  /** Announce an available version, unless this run already declined it or
   *  is already acting on an update. Driven by the check's own finding, so
   *  the startup check, the interval and About's button all announce through
   *  this one path. */
  showAvailable: (version: string, notesUrl: string) => void
}

/** What this controller is doing with the update it holds. */
type Phase = 'idle' | 'announcing' | 'downloading' | 'installed'

export function createUpdateToast(deps: UpdateToastDeps = {}): UpdateToastController {
  const apply = deps.apply ?? (() => ApplyUpdate())
  const restart = deps.restart ?? (() => RestartWindow())
  /** Versions declined with Later in this app run. In memory by design:
   *  "until next app start" is the bead's rule, and a persisted value would
   *  silence the first check of the next run too. */
  const declined = new Set<string>()
  let phase: Phase = 'idle'
  let version = ''
  let toastId: number | null = null

  const closeCurrent = (): void => {
    if (toastId !== null) {
      dismissToast(toastId)
      toastId = null
    }
  }

  const install = (): void => {
    phase = 'downloading'
    closeCurrent()
    toastId = showToast({
      message: `Downloading nocx ${version}…`,
      // Work in flight is not an offer; it stays until it ends, one way
      // or the other.
      duration: 0,
    })
    void (async () => {
      try {
        await apply()
        phase = 'installed'
        closeCurrent()
        toastId = showToast({
          level: 'success',
          message: `nocx ${version} installed — restart to apply`,
          duration: 0,
          actions: [
            {
              label: 'Restart now',
              variant: 'primary',
              onClick: () => void restart(),
            },
          ],
        })
      } catch (err) {
        phase = 'idle'
        closeCurrent()
        toastId = showToast({
          level: 'danger',
          message: `Update failed: ${err instanceof Error ? err.message : String(err)}`,
          // A failure stays until answered — the kit's own rule for danger.
          duration: 0,
        })
      }
    })()
  }

  const later = (): void => {
    declined.add(version)
    phase = 'idle'
  }

  return {
    showAvailable: (nextVersion, notesUrl) => {
      // Already acting on an update: the announcement that matters now is
      // the download's own progress, not a second offer of it.
      if (phase === 'downloading' || phase === 'installed') return
      // The same version's toast is already up; the interval can land twice
      // before anybody answers.
      if (phase === 'announcing' && version === nextVersion) return
      if (declined.has(nextVersion)) return
      version = nextVersion
      phase = 'announcing'
      toastId = showToast({
        message: `nocx ${nextVersion} is available`,
        link: { label: 'Release notes', href: notesUrl },
        actions: [
          { label: 'Install now', variant: 'primary', onClick: install },
          { label: 'Later', onClick: later },
        ],
        // It holds two decisions a person may press at any moment.
        duration: 0,
      })
    },
  }
}
