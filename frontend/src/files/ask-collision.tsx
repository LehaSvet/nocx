// Asking the collision question — the kit's CollisionDialog, mounted for
// one question and disposed with the answer.
//
// The same shape as the kit's own showConfirm (ui/dialog.tsx): a host
// element, one render, a promise that settles exactly once and a disposal
// deferred so the Dialog's own cleanup — popOverlay and the focus restore —
// runs against a live root. The flow needs an ANSWER, not a component
// lifetime, and a dialog kept alive behind an `open` flag would have to
// remember a decision between openings; this one remembers nothing, which
// is why the kit mounts it per question.
//
// Nothing here paints: the panel, the tick box and the three buttons are
// the kit's, and dismissal answers `skip` because that decision is the
// dialog's own and it is the single outcome it exists to protect.

import { render } from 'solid-js/web'
import {
  CollisionDialog,
  type CollisionRequest,
  type CollisionResult,
} from '../ui/collision-dialog'
import { t, tPlural } from '../i18n/locale-model'

export function askCollision(request: CollisionRequest): Promise<CollisionResult> {
  return new Promise<CollisionResult>((resolve) => {
    const host = document.createElement('div')
    document.body.appendChild(host)

    let dispose: (() => void) | null = null
    let settled = false

    const finish = (result: CollisionResult): void => {
      if (settled) return
      settled = true
      queueMicrotask(() => {
        dispose?.()
        host.remove()
      })
      resolve(result)
    }

    dispose = render(
      () => (
        <CollisionDialog
          request={request}
          title={t('ui.collision.title', 'File already exists')}
          overwriteLabel={t('ui.collision.overwrite', 'Overwrite')}
          keepBothLabel={t('ui.collision.keepBoth', 'Keep both')}
          skipLabel={t('ui.collision.skip', 'Skip')}
          bodyPrefix={(name, destination) => (
            <>
              <code>{name}</code> {t('ui.collision.existsIn', 'already exists in')}{' '}
              <code>{destination}</code>.
            </>
          )}
          applyAllLabel={(others) =>
            others === 1
              ? t('ui.collision.applyOne', 'Apply to the 1 remaining file')
              : tPlural('ui.collision.applyMany', others, 'Apply to the {count} remaining files')
          }
          onResolve={finish}
        />
      ),
      host,
    )
  })
}
