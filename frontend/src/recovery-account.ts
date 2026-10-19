/**
 * What one reclaim's holes add up to, and which bytes a live notice has
 * already named (nocx-fz4qa, nocx-zg3k3.14.24).
 *
 * ITS OWN MODULE, and for a reason the e2e suite made concrete: this
 * derivation is what the card renders AND what a test must compare the card
 * against, so the two cannot be written twice. Before it existed here the
 * spec derived the same account with its own arithmetic and got a different
 * number from the card — one that summed the recording's `cap` ranges only,
 * while the card states every missing byte. A test that re-derives a product
 * fact is a second owner of it, and the two drift silently.
 *
 * It imports nothing but TYPES, deliberately: `e2e/remote-coordinator-reclaim
 * .spec.ts` imports this file directly, and a module that reached the renderer
 * (the kit, ipc.ts, a stylesheet) could not be loaded there.
 */
import type { SessionOutputGap } from './generated/session.outputGap'

/** The retention bound's word, minted by the store (internal/content's
 *  TruncCap) and repeated on the wire by session.recoveryStatus. Named here
 *  rather than inlined so the one place that translates it into English is
 *  greppable from the Go side. Not exported: the account is the only reader,
 *  and a caller that needed the word would be doing the arithmetic itself. */
const CAP = 'cap'

/** The gap reason for a stretch that neither the recording nor the ring
 *  holds. The renderer DERIVES this one because only it can: `produced`
 *  belongs to session.output and `replayFrom` to sessions.live, and neither
 *  owner can see the other's number.
 *
 *  The backend mints the same word for a range nobody recorded on its side
 *  (internal/content's GapReasonUnrecorded, nocx-k6p18.2), and that is
 *  deliberate rather than a second owner: it is one fact — nobody has these
 *  bytes — arrived at from two directions, and one fact told to a user in two
 *  vocabularies is the defect. Keep them equal. What must stay distinct is
 *  this and `cap`: the cap dropped bytes that were here, and telling a person
 *  the cap took bytes it never had would send them to a limit that did
 *  nothing. */
export const UNRECORDED = 'unrecorded'

/** What the gaps of one reclaim add up to, split by who is answerable for
 *  each part. Its own exported derivation because "how much is missing" is
 *  a question about the data and not about the card: the card renders it,
 *  and a caller deciding whether there is anything to say asks it too. */
export interface RecoveryAccount {
  /** Every missing byte over the recovered span. */
  missing: number
  /** What the recording's retention bound dropped. */
  dropped: number
  /** What nothing kept at all — neither the recording nor the ring. */
  unrecorded: number
  /** Missing for a reason this build has no sentence for. */
  other: number
  /** Those reasons, in the wire's own words, de-duplicated and in the order
   *  they arrived. Carried so the card can print them instead of inventing
   *  an explanation. */
  reasons: string[]
  statusUnavailable: boolean
}

/** The part of a recovery answer this derivation reads. Structural on
 *  purpose: `SessionRecovery` (ipc.ts) satisfies it, and so does a
 *  session.recoveryStatus result straight off the wire, which is what lets a
 *  test ask the same question the card does. */
export interface RecoveryInput {
  gaps: readonly Pick<SessionOutputGap, 'start' | 'end' | 'reason'>[]
  statusUnavailable?: boolean
}

/** Partition a byte range by removing the portions already covered by other
 *  ranges. This is shared by historical accounting and live-notice mounting:
 *  one interval owner keeps overlap semantics identical in both places. */
export function uncoveredGapRanges(
  gap: Pick<SessionOutputGap, 'start' | 'end'>,
  covered: readonly Pick<SessionOutputGap, 'start' | 'end'>[],
): Array<{ start: number; end: number }> {
  if (!Number.isFinite(gap.start) || !Number.isFinite(gap.end) || gap.end <= gap.start) {
    return []
  }
  let remaining: Array<[number, number]> = [[gap.start, gap.end]]
  for (const range of covered) {
    if (!Number.isFinite(range.start) || !Number.isFinite(range.end) || range.end <= range.start) {
      continue
    }
    const next: Array<[number, number]> = []
    for (const [start, end] of remaining) {
      const overlapStart = Math.max(start, range.start)
      const overlapEnd = Math.min(end, range.end)
      if (overlapStart >= overlapEnd) {
        next.push([start, end])
        continue
      }
      if (start < overlapStart) next.push([start, overlapStart])
      if (overlapEnd < end) next.push([overlapEnd, end])
    }
    remaining = next
    if (remaining.length === 0) return []
  }
  return remaining.map(([start, end]) => ({ start, end }))
}

/**
 * Add up one reclaim's holes, or null when there is nothing to report.
 *
 * NULL IS THE COMMON ANSWER and it is the important one: a pane that opened
 * its own session recovered nothing (`recovered` is null), and a reclaim that
 * got the whole recording back has no gaps. A notice that appeared for those
 * would appear on nearly every tab and would therefore mean nothing.
 *
 * A range that runs backwards or is not a finite number is SKIPPED, not
 * subtracted: it is a hole of unknown size, and letting it cancel a real one
 * would turn a reporting bug into a silence.
 */
export function recoveryAccount(
  recovery: RecoveryInput | null,
  liveGaps: readonly Pick<SessionOutputGap, 'start' | 'end'>[] = [],
): RecoveryAccount | null {
  if (recovery === null) return null
  const account: RecoveryAccount = {
    missing: 0,
    dropped: 0,
    unrecorded: 0,
    other: 0,
    reasons: [],
    statusUnavailable: recovery.statusUnavailable ?? false,
  }
  for (const gap of recovery.gaps) {
    const originalBytes = gap.end - gap.start
    if (!Number.isFinite(originalBytes) || originalBytes <= 0) continue
    // A live gap notice already names these exact byte offsets. Remove their
    // overlap from the historical account so the same lost bytes are never
    // attributed twice; any distinct recording/cap loss remains visible.
    const bytes = uncoveredGapRanges(gap, liveGaps).reduce(
      (total, range) => total + range.end - range.start,
      0,
    )
    if (!Number.isFinite(bytes) || bytes <= 0) continue
    account.missing += bytes
    if (gap.reason === CAP) {
      account.dropped += bytes
      continue
    }
    if (gap.reason === UNRECORDED) {
      account.unrecorded += bytes
      continue
    }
    account.other += bytes
    if (!account.reasons.includes(gap.reason)) account.reasons.push(gap.reason)
  }
  return account.missing > 0 || account.statusUnavailable ? account : null
}
