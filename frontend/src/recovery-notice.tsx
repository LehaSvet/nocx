/**
 * The reclaimed pane's missing-output notice (nocx-fz4qa).
 *
 * A reclaim's `session.recoveryStatus` answer is metadata only: it reports
 * the produced offset and known gap ranges, never stream bytes. `reclaimSession`
 * adds the stretch between that offset and the live ring's `replayFrom`
 * (ipc.ts's UNRECORDED). Both land on SessionHandle.recovered.gaps — and
 * until this module nothing read them. A
 * pane came back with a hole in the middle of its scrollback and said nothing,
 * which is the shape AGENTS.md forbids by name: a soft degrade must be visible
 * in the product, not only in a log. A scrollback that is silently short is
 * indistinguishable from a session that printed less.
 *
 * WHY IT IS NOT FOLDED INTO THE INTEGRATION NOTICE. That card reports a
 * SESSION STATE — the shell did not integrate, it may recover, and the card
 * comes and goes with the state (integration/notice.tsx). This reports a
 * FINISHED FACT about one reclaim: the bytes were already gone before this
 * client existed and nothing will make them appear. They have different
 * lifetimes and different remedies, and a card whose title had to cover both
 * would say something vague about each.
 *
 * WHAT IT MUST NOT SAY. It never claims the output can be got back, and it
 * never guesses at a reason. The two reasons that reach a client today are
 * the retention bound (`cap`, internal/content's TruncCap — the store's own
 * word for the middle it dropped) and `unrecorded`; anything else is carried
 * through verbatim rather than translated into a sentence this build cannot
 * stand behind.
 *
 * Everything visible is a kit component this surface places. The identity
 * class positions the card in the pane and repaints nothing, which is the
 * boundary frontend/src/ui/README.md draws.
 */

import { render } from 'solid-js/web'
import { IconButton } from './ui/icon-button'
import { StatusCard } from './ui/status-card'
import { formatBytes } from './ui/format-bytes'
import type { SessionRecovery } from './ipc'
import type { SessionOutputGap } from './generated/session.outputGap'
import { CloseIcon } from './ui/icons'
import { recoveryAccount, uncoveredGapRanges, type RecoveryAccount } from './recovery-account'

// The account itself lives in recovery-account.ts: it is the ONE derivation of
// "how much is missing", rendered here and compared against this card by the
// e2e suite (nocx-zg3k3.14.24). Re-exported so the surface's own tests and the
// live-gap code keep importing it from the notice that draws it.
export { recoveryAccount, uncoveredGapRanges }

/** The clauses, one per owner of a part of the hole, in the order a person
 *  reads them: the bound first because it is the ordinary cause, then the
 *  stretch nothing kept, then whatever this build cannot name. Each carries
 *  its own byte count, so a reclaim that hit two causes is two answerable
 *  numbers rather than one that explains neither. */
function clauses(account: RecoveryAccount): string[] {
  const parts: string[] = []
  if (account.dropped > 0) {
    parts.push(`${formatBytes(account.dropped)} the recording's size limit dropped`)
  }
  if (account.unrecorded > 0) {
    parts.push(`${formatBytes(account.unrecorded)} that was never recorded`)
  }
  if (account.other > 0) {
    parts.push(`${formatBytes(account.other)} missing as "${account.reasons.join('", "')}"`)
  }
  return parts
}

/** The card's headline: the whole hole, as a number a person can weigh
 *  against the run they were watching. */
function title(account: RecoveryAccount): string {
  if (account.statusUnavailable) return 'Could not check this session’s recovery status'
  return `${formatBytes(account.missing)} of this session's output is missing`
}

/** …and what took it. The sentence ends where the honesty does — nothing
 *  here offers to get the bytes back, because nothing can. */
function description(account: RecoveryAccount): string {
  if (account.statusUnavailable) {
    return 'Recovery metadata could not be read. Some earlier session output may be missing.'
  }
  return `This tab was taken back after those bytes were gone: ${clauses(account).join(', and ')}.`
}

export interface RecoveryNoticeProps {
  /** What the reclaim recovered, and what it could not. */
  recovery: SessionRecovery
  /** Live gaps already named by the neighboring live notice. Their byte
   *  ranges are removed from the historical account to avoid double-counting. */
  liveGaps?: readonly Pick<SessionOutputGap, 'start' | 'end'>[]
  /** Take this card away. It records nothing: the hole is permanent and the
   *  card is not, which is the whole of what dismissing it means. */
  onDismiss: () => void
}

function RecoveryNotice(props: { account: RecoveryAccount; onDismiss: () => void }) {
  return (
    <StatusCard
      tone="warning"
      title={title(props.account)}
      description={description(props.account)}
      action={
        <IconButton ariaLabel="Dismiss" size="sm" onClick={() => props.onDismiss()}>
          <CloseIcon />
        </IconButton>
      }
    />
  )
}

/**
 * Mount the notice into a tab's pane, or answer null when this reclaim has
 * nothing to report.
 *
 * NULL RATHER THAN AN EMPTY MOUNT, so the decision "is there a hole" is taken
 * exactly once, here, from recoveryAccount. A caller that had to ask first and
 * then mount would be the second reader of the same derivation, and the two
 * would drift the first time either changed.
 *
 * It goes in the FLOW as the pane's first child, the same placement the
 * integration notice settled on (nocx-rzvq): the pane is a flex column, so
 * the card takes its own height off the top and the terminal is laid out in
 * what is left. Overlaying covers the first prompt line — a message about the
 * session must not hide the session. The caller re-measures the live region
 * after mounting and after disposing, exactly as it already does for the
 * integration card.
 */
export function mountRecoveryNotice(
  target: HTMLElement,
  props: RecoveryNoticeProps,
): (() => void) | null {
  const account = recoveryAccount(props.recovery, props.liveGaps)
  if (account === null) return null
  const host = document.createElement('div')
  host.className = 'nocx-recovery-notice'
  target.insertBefore(host, target.firstChild)
  const dispose = render(
    () => <RecoveryNotice account={account} onDismiss={props.onDismiss} />,
    host,
  )
  return () => {
    dispose()
    host.remove()
  }
}

/** Mount the same per-pane warning-card surface for a gap crossed while the
 * tab remains live. This is a transport fact, distinct from a reclaim card;
 * offsets remain exact byte counts and the terminal surface is left in place.
 * `unannouncedBytes` is smaller than the event range when it overlaps a live
 * loss already named by an earlier notice. */
export function mountLiveOutputGapNotice(
  target: HTMLElement,
  gap: SessionOutputGap,
  onDismiss: () => void,
  unannouncedBytes = gap.end - gap.start,
): () => void {
  const host = document.createElement('div')
  host.className = 'nocx-recovery-notice'
  target.insertBefore(host, target.firstChild)
  const bytes = unannouncedBytes
  const description =
    gap.reason === 'hostWindow'
      ? "The execution host's output window moved past this tab."
      : 'This output gap has an unknown cause.'
  const dispose = render(
    () => (
      <StatusCard
        tone="warning"
        title={`${bytes} bytes of live output are missing`}
        description={description}
        action={
          <IconButton ariaLabel="Dismiss" size="sm" onClick={() => onDismiss()}>
            <CloseIcon />
          </IconButton>
        }
      />
    ),
    host,
  )
  return () => {
    dispose()
    host.remove()
  }
}
