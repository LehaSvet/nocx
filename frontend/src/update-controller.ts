import { createSignal } from 'solid-js'
import {
  ApplyUpdate,
  FinishPendingUpdate,
  RestartWindow,
} from '../bindings/github.com/shady2k/nocx/wailsapp'
import { showToast, dismissToast, type ToastOptions } from './ui/toast'
import { log } from './log'

type UpdateState =
  | { kind: 'hidden' }
  | { kind: 'available'; version: string; notesUrl: string }
  | { kind: 'downloading'; version: string }
  | { kind: 'pending'; version: string }
  | { kind: 'held'; version: string }
  | { kind: 'error'; message: string }

export interface UpdateControllerDeps {
  apply?: () => Promise<void>
  finish?: () => void | Promise<void>
  restart?: () => void | Promise<void>
}

export interface UpdateController {
  state: () => UpdateState
  showAvailable: (version: string, notesUrl: string) => void
  showOffer: () => void
  apply: () => Promise<void>
  showHeldByCoordinator: (version: string) => void
}

/** One owner for update state and its one kit surface. The sidebar is rebuilt
 * on reconnect; this controller lives for the whole window run (AD-8). */
export function createUpdateController(deps: UpdateControllerDeps = {}): UpdateController {
  const [state, setState] = createSignal<UpdateState>({ kind: 'hidden' })
  const applyUpdate = deps.apply ?? (() => ApplyUpdate())
  const finishUpdate = deps.finish ?? (() => FinishPendingUpdate())
  const restartWindow = deps.restart ?? (() => RestartWindow())
  let toastId: number | null = null
  // Per-run memory, never localStorage: both Later and the kit close button
  // silence the announcement, not the persistent indicator.
  const announced = new Set<string>()

  const present = (options: ToastOptions): void => {
    if (toastId !== null) dismissToast(toastId)
    toastId = showToast({ ...options, duration: 0 })
  }
  const fail = (error: unknown): void => {
    log.warn('update action failed', { error })
    setState({
      kind: 'error',
      message: 'The update could not finish. Try again, or install the latest release manually.',
    })
    showOffer()
  }
  const restart = (): void => {
    void Promise.resolve().then(restartWindow).catch(fail)
  }
  const finish = (): void => {
    void Promise.resolve().then(finishUpdate).catch(fail)
  }
  const showOffer = (installLabel = 'Install'): void => {
    const current = state()
    switch (current.kind) {
      case 'hidden':
        return
      case 'available':
        present({
          message: `nocx ${current.version} is available`,
          link: { label: 'Release notes', href: current.notesUrl },
          actions: [
            { label: installLabel, variant: 'primary', onClick: () => void apply() },
            { label: 'Later', onClick: () => {} },
          ],
        })
        return
      case 'downloading':
        present({ message: `Downloading nocx ${current.version}…` })
        return
      case 'pending':
        present({
          level: 'success',
          message: `nocx ${current.version} installed — restart to apply`,
          actions: [{ label: 'Restart now', variant: 'primary', onClick: restart }],
        })
        return
      case 'held':
        present({
          level: 'warning',
          message: `nocx ${current.version} installed — finish the update: restart nocx fully (closes open sessions)`,
          actions: [{ label: 'Restart now', variant: 'primary', onClick: finish }],
        })
        return
      case 'error':
        present({ level: 'danger', message: current.message })
        return
    }
  }
  const apply = async (): Promise<void> => {
    const current = state()
    if (current.kind !== 'available') return
    setState({ kind: 'downloading', version: current.version })
    showOffer()
    try {
      await applyUpdate()
      setState({ kind: 'pending', version: current.version })
      showOffer()
    } catch (error) {
      fail(error)
    }
  }
  return {
    state,
    showOffer,
    apply,
    showAvailable: (version, notesUrl) => {
      const current = state()
      if (current.kind === 'downloading' || current.kind === 'pending' || current.kind === 'held')
        return
      setState({ kind: 'available', version, notesUrl })
      if (!announced.has(version)) {
        announced.add(version)
        showOffer('Install now')
      }
    },
    showHeldByCoordinator: (version) => {
      setState({ kind: 'held', version })
    },
  }
}
