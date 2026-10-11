// The frame model (spec §2.2–2.4, bead nocx-3j9b).
//
// A frame is cells + attributes + cursor of ONE capture identity, frozen as a
// local display view. Text, never a picture. The capture identity records the
// active buffer instance, geometry, and screen revision.
//
// The two capture sources are NOT the same path, and the frame records which
// one it came from — they are never silently substituted for one another:
//
//   live   — the accepted backend screen frame. Its absolute row range and
//            buffer identity use the same coordinates as session.read,
//            session.historyPage, and block output anchors.
//   frozen — a block after its freeze: no live cells remain, and the serializer
//            has joined wrapped lines and dropped leading/trailing blanks.
//            Provenance records source=frozen and the serializer version.
//
// AD-8: the backend frame is the owner of live screen cells. The serializer
// (scrollback/serializer.ts) remains the single owner of frozen text derivation;
// this module never parses VT output or reconstructs a screen from PTY bytes.

import type { CellAttrs } from '../scrollback/serializer'

/** Which buffer instance a frame belongs to. The normal buffer is one
 *  instance; EACH entry into the alternate screen is a new one (entering
 *  mints a new identity, leaving terminates it — spec §2.3). */
type BufferInstance =
  { kind: 'normal'; instance?: string } | { kind: 'alternate'; altSession: number | string }

/** Capture identity: what a frame belongs to and can be compared against.
 *  Backend captures use the accepted screen revision and exact buffer instance.
 *  The legacy renderer adapter advances its generation on parsed writes and
 *  state-changing operations, never on paint. */
export interface CaptureIdentity {
  buffer: BufferInstance
  cols: number
  rows: number
  generation: number
}

/** The outcome of comparing a saved identity with the current one.
 *
 *  `notComparable` is a DISTINCT VALUE, never a flag on staleness: a buffer
 *  switch or a resize is incomparability (the alt buffer's contents are
 *  discarded on exit; a resize reflows and shifts absolute line indices).
 *  Only `moved` — same instance, same geometry, generation differs — is
 *  "the screen was written to", and per ADR-0029 it is a trigger for
 *  re-evaluation, never itself a verdict.
 */
export type CaptureComparability =
  { status: 'same' } | { status: 'moved' } | { status: 'notComparable' }

/** One cell of a live frame: accepted character and attributes, resolved
 *  against the theme snapshot used when the local view was minted. */
interface FrameCell {
  char: string
  attrs: CellAttrs
}

/** One row of a frame. Live rows are cells+attributes; frozen rows are TEXT —
 *  a frozen block has no live cells, and its text has already been transformed
 *  by the serializer. The row kind records which. */
export type FrameRow = { kind: 'cells'; cells: FrameCell[] } | { kind: 'text'; text: string }

/** Provenance of a live frame. Records the buffer instance, geometry,
 *  generation, and that rows may be evicted by the 10000-line scrollback cap
 *  while the block still runs. */
interface LiveProvenance {
  source: 'live'
  identity: CaptureIdentity
  /** Absolute buffer row range this frame holds ([start, end)). */
  range: { start: number; end: number }
  /** The active buffer is a 10000-line cap: as new output arrives the OLDEST
   *  rows are evicted from the top of the buffer, so the frame's low row
   *  indices are a suffix — the block may have had rows that the cap already
   *  dropped while it still ran. The frame records what survives, and this
   *  note names why earlier rows may be gone. */
  scrollbackCapLines: 10000
}

/** Provenance of a frozen frame. Records source=frozen and the serializer
 *  version — the version of the transform that produced the text (wrapped
 *  lines joined, leading/trailing blanks dropped). A frozen block's identity
 *  is closed: it can never move, so it can never go stale. */
interface FrozenProvenance {
  source: 'frozen'
  /** Serializer version that produced this block's text — a bumpable
   *  integer; the constant lives with the serializer (AD-8). */
  serializerVersion: number
  /** The serializer's transforms, recorded so the text's provenance is never
   *  mistaken for raw cells: wrapped lines are joined and leading/trailing
   *  blanks dropped. */
  transforms: 'wrapped lines joined; leading/trailing blanks dropped'
  /** A frozen block's identity is closed — its generation never advances
   *  again (spec §2.3, "the degenerate case"). */
  closed: true
}

type CaptureProvenance = LiveProvenance | FrozenProvenance

/** A local pinned view: rows + cursor of one accepted capture identity,
 *  with provenance naming the source. Text, never a picture. */
export interface CapturedFrame {
  rows: FrameRow[]
  /** Absolute cursor position. null for a frozen frame — there is no cursor
   *  in a serialized block. */
  cursor: { line: number; col: number } | null
  provenance: CaptureProvenance
}

/** The renderer fact surface the capture identity is derived from.
 *
 *  XtermRenderer satisfies this structurally (the same event shapes as
 *  TerminalRenderer). The tracker requires the WHOLE contract: a source that
 *  cannot report reset/write-pending would silently under-count the
 *  generation, which is exactly the false "unchanged" the spec forbids.
 */
export interface CaptureEventSource {
  /** Fires at the end of every parse pass — the generation's advance signal.
   *  It is NOT the capture fence: xterm breaks its parse loop on a 12 ms
   *  budget, so a pass can end with bytes that were already queued still
   *  unparsed. The fence is awaitWriteBarrier(). */
  onWriteParsed(cb: () => void): void
  onBufferChange(cb: (type: 'normal' | 'alternate') => void): void
  onResize(cb: (cols: number, rows: number) => void): void
  /** Fired AFTER the renderer executed a full reset. */
  onReset(cb: () => void): void
  /** Fires when the source is disposed (tab close, renderer replacement) —
   *  the fence's CLOSING event. A capture parked on hasUnsettledWrite()
   *  after this can never settle on its own: the per-write callback went
   *  away with the terminal, so the pending count is stuck. AGENTS.md rule
   *  3 — an invariant needs both ends; this is the end that lets the
   *  tracker reject pending awaitSettled() waiters instead of orphaning
   *  them. */
  onDispose(cb: () => void): void
  /** True while bytes queued via write() have not finished parsing. This
   *  decides whether a capture needs the fence at all: with nothing queued
   *  the buffer already holds every byte the renderer was given, and issuing
   *  a barrier would cost a parse pass that ADVANCES THE GENERATION on a
   *  motionless screen (ADR-0029: never manufacture a false "moved"). */
  hasUnsettledWrite(): boolean

  /** THE CAPTURE FENCE: a write barrier. Resolves once every write queued
   *  BEFORE this call has been parsed — and no later. xterm's WriteBuffer is
   *  FIFO, so writes arriving afterwards queue behind the barrier and cannot
   *  postpone it: a continuously repainting TUI cannot starve the fence, and
   *  a bulk write cannot be answered half-parsed.
   *
   *  It settles for the renderer that is alive, however busy. A barrier that
   *  never settles means a wedged parse queue, which the tracker bounds
   *  locally and answers as a failed capture. */
  awaitWriteBarrier(): Promise<void>
  readonly cols: number
  readonly rows: number
}
