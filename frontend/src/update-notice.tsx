import { type Component, type Accessor, Switch, Match, createSignal } from 'solid-js'
import { render } from 'solid-js/web'
import { ApplyUpdate, FinishPendingUpdate } from '../bindings/github.com/shady2k/nocx/wailsapp'
import { Button } from './ui/button'

export type UpdateState =
  | { kind: 'hidden' }
  | { kind: 'available'; version: string; notesUrl: string }
  | { kind: 'downloading' }
  | { kind: 'pending'; version: string }
  | { kind: 'held'; version: string }
  | { kind: 'error'; message: string }

/** What the held state's finish action needs. Injectable so a test stands
 *  on the decision rather than on the binding; the default is the window's
 *  own FinishPendingUpdate — stop the kept coordinator, then quit. */
export interface UpdateNoticeDeps {
  finish?: () => void | Promise<void>
}

export interface UpdateNoticeController {
  showAvailable: (version: string, notesUrl: string) => void
  showDownloading: () => void
  showPendingRestart: (version: string) => void
  /** Shows the ADR-0099 held state: an update is installed and a
   *  protocol-compatible older coordinator is still serving it. The copy
   *  names the whole truth — finishing closes the open sessions — and the
   *  action is the person's to press, never forced. */
  showHeldByCoordinator: (version: string) => void
  showError: (message: string) => void
}

export function mountUpdateNotice(
  bar: HTMLElement,
  deps: UpdateNoticeDeps = {},
): UpdateNoticeController {
  const host = document.createElement('div')
  bar.append(host)

  const [state, setState] = createSignal<UpdateState>({ kind: 'hidden' })

  const apply = () => {
    void (async () => {
      setState({ kind: 'downloading' })
      try {
        await ApplyUpdate()
        setState({ kind: 'pending', version: '' })
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err)
        setState({ kind: 'error', message: msg })
      }
    })()
  }

  // The finish either quits the application (success — this strip's last
  // frame) or explains itself: a stop that failed leaves the sessions it
  // was going to end still running, and silence there would be a button
  // that appears to have done nothing.
  const finish = () => {
    void (async () => {
      try {
        await (deps.finish ? deps.finish() : FinishPendingUpdate())
      } catch (err) {
        setState({ kind: 'error', message: err instanceof Error ? err.message : String(err) })
      }
    })()
  }

  render(() => <UpdateNoticeView state={state} onApply={apply} onFinish={finish} />, host)

  return {
    showAvailable: (version: string, notesUrl: string) =>
      setState({ kind: 'available', version, notesUrl }),
    showDownloading: () => setState({ kind: 'downloading' }),
    showPendingRestart: (version: string) => setState({ kind: 'pending', version }),
    showHeldByCoordinator: (version: string) => setState({ kind: 'held', version }),
    showError: (message: string) => setState({ kind: 'error', message }),
  }
}

interface UpdateNoticeViewProps {
  state: Accessor<UpdateState>
  onApply: () => void
  onFinish: () => void | Promise<void>
}

const UpdateNoticeView: Component<UpdateNoticeViewProps> = (props) => {
  const className = () => {
    const k = props.state().kind
    if (k === 'hidden' || k === 'available') return 'update-notice'
    return `update-notice ${k}`
  }

  const hidden = () => props.state().kind === 'hidden'

  return (
    <div class={className()} hidden={hidden()}>
      <Switch fallback={null}>
        <Match when={props.state().kind === 'available'}>
          <span>
            nocx {(props.state() as Extract<UpdateState, { kind: 'available' }>).version} available
          </span>
          {' · '}
          <a
            href={(props.state() as Extract<UpdateState, { kind: 'available' }>).notesUrl}
            target="_blank"
            rel="noopener"
            class="update-notes-link"
          >
            release notes
          </a>{' '}
          <Button variant="primary" onClick={() => props.onApply()}>
            Update
          </Button>
        </Match>
        <Match when={props.state().kind === 'downloading'}>
          <span>Downloading update…</span>
        </Match>
        <Match when={props.state().kind === 'pending'}>
          <span>
            nocx {(props.state() as Extract<UpdateState, { kind: 'pending' }>).version} installed
            {' — '}restart to apply
          </span>
        </Match>
        <Match when={props.state().kind === 'held'}>
          <span>
            nocx {(props.state() as Extract<UpdateState, { kind: 'held' }>).version} installed
            {' — '}finish the update: restart nocx fully (closes open sessions)
          </span>{' '}
          <Button variant="primary" onClick={() => void props.onFinish()}>
            Restart now
          </Button>
        </Match>
        <Match when={props.state().kind === 'error'}>
          <span>
            Update failed: {(props.state() as Extract<UpdateState, { kind: 'error' }>).message}
          </span>
        </Match>
      </Switch>
    </div>
  )
}
