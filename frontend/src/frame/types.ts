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

/** Identity copied from the accepted backend frame for a pinned view.
 *  `generation` records that frame's revision; buffer and geometry prevent
 *  comparing cells across a buffer replacement or resize. */
interface CaptureIdentity {
  buffer: BufferInstance
  cols: number
  rows: number
  generation: number
}

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
