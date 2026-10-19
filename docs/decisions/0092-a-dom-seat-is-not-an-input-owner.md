# ADR-0092 — A DOM seat is not an input owner: the live grid keeps its own gestures inside a running block

- **Status:** Accepted
- **Date:** 2026-10-19
- **Decided by:** the worker on `nocx-zg3k3.14.26`, 2026-10-19, on the four e2e
  shards ADR-0091's seat change turned red. The rule applied is not new —
  AGENTS.md's _"two surfaces may never own the same input"_ and AD-8's one owner
  per behaviour — and what this record decides is where the boundary falls once
  the region's seat moves.
- **Extends:** [ADR-0091](0091-a-running-command-s-output-is-one-block-and-a-separate-live-grid-exists-only-where-there-are-no-blocks.md),
  whose decision 3 puts the live region inside the running block. It takes back
  nothing: every sentence of 0091 holds, including the full-screen and
  markerless cases. This record answers the question 0091 left open by moving
  the region — who answers a press, a word and a keystroke that lands on it.
- **Supersedes:** none.
- **Related:** AD-8 (interface-first, one owner per behaviour),
  [ADR-0009](0009-dom-scrollback-with-explicit-cell-geometry.md) (the frozen
  block's own rows and gestures),
  [ADR-0089](0089-a-live-grid-drag-selects-even-while-the-program-reports-the-mouse.md)
  (the grid's drag is the person's, which presupposes the grid's gesture runs at
  all), [ADR-0040](0040-a-turn-owns-the-blocks-it-caused.md) (the innermost
  block owns a click).

## Context

Before ADR-0091 the live region was a direct child of `.scrollback-inner`. No
block contained it, so two questions that are not the same question had one
answer, and the tree never had to say which was meant:

- "is this node inside a command block?" — `closest('.cmd-block')`;
- "is this node part of the terminal's own painted surface?" — the region and
  everything in it.

ADR-0091 makes the region a running block's body. Every guard written as the
first question now answers YES about the grid's own rows, and the grid's rows
are the cell painter's: its press, its drag, its word rule, its wheel and its
keyboard read the CELL SNAPSHOT (`painter/selection-gesture.ts`, the pane's
input element), never the row's text nodes.

Measured, four shards, all on the branch that moved the region:

- `e2e/live-to-card-joined.spec.ts` — a click on the grid selected the block;
  the next character typed into the running program was eaten by the
  block-selection keyboard branch (`terminal-content.ts`, `selectedBlockId`),
  so the program never received it.
- `e2e/ask-about-a-running-command.spec.ts` — the same press, one gesture later:
  the Escape that should stop a streaming answer reached the branch that
  DESELECTS a block and returned, so `agent.cancel` was never sent.
- `e2e/clipboard.spec.ts` — a double click in a running program copied `dbl`
  where the word was `~/dbl-mvf8dc0n.txt`. The painter's own word rule stood
  down ("a plain command block's selectable text … never a guessed live-grid
  cell"), and what remained was a native selection over the row's text nodes,
  which in painted output are runs of cells.
- `e2e/ask-about-full-screen-program.spec.ts` — the keyboard handed back to a
  full-screen program after Ask did not reach it: `q`, pressed after the Ask
  was dismissed, reached the program's file as the empty string. This is the
  one of the four whose mechanism is NOT read in the tree — both candidates
  (which surface owned the press that preceded the Ask, and where the keyboard
  sat when `q` arrived) are downstream of this record's guard — so it is
  recorded as it was measured: red on the branch head, on both engines, and
  green with the guard in place.

## Decision

1. **The seat decides nothing about ownership.** Moving the region into the
   block moves rows in the DOM; it does not move which module answers an input.
   While a command runs, a press, a drag, a wheel, a double click and a
   keystroke that lands in the live region belong to the grid — the pane's input
   element and the cell painter, exactly as they did when the region sat outside
   every block.
2. **The block owns its own surface and never the terminal's:** its header and
   the controls in it, its chrome, and the rows of its own output
   (`.cmd-output`). A running block's click-to-select, its double-click word
   rule and the pane's press-to-deselect ask the live-region question first and
   stand down when the press is the grid's.
3. **One question, one module, one answer: `frontend/src/live-region.ts`.**
   `insideLiveRegion(node)` is that question. It is the only spelling of "this
   is the terminal's own surface" that a gesture consults, and a new gesture
   that meets `.cmd-block` consults it before it claims a press. Four
   `closest('.cmd-block')` guards each meaning something slightly different is
   what produced the four shards.
4. **What does not change:** a frozen block's gestures are untouched — its
   rows, its header word rule, its selection, its copy. A press outside every
   block still ends a block selection, and the live region counts as outside it
   for that purpose, which is what it was before the seat moved.

## Why this rather than the obvious alternatives

- **Give the region back its old seat and make the block's box cover it with
  CSS.** That would leave the DOM saying the rows are not the block's while the
  box paints as if they were, and it reverses a decision the owner took on the
  stand (ADR-0091) to fix a defect about a box. The box must contain what it
  grounds; the ownership question is answered by asking it, not by declining to
  nest.
- **Patch each red where it appeared.** The cheapest patch for the clipboard
  shard is to keep the block's own word rule and cancel the native selection in
  its `mousedown`. That was tried and it does not even work: with the painter's
  endpoint still refusing a live row inside a block, nothing selects the word —
  the failing copy was a symptom of the painter standing down, not of the
  browser winning a race. Four guards asking one question in four spellings is
  the defect AGENTS.md names, whichever of them is left standing.
- **Let the block's gestures delegate to the painter's when the target is the
  live region.** The block has nothing to delegate with: the painter's rules
  read a cell snapshot the block does not hold, and the pane's input element is
  not the block's child in any sense but the DOM's.

## What the next person inherits

- **A gesture that meets `.cmd-block` asks `insideLiveRegion` first.** The
  region can be inside a block in running mode only, so the guard is a no-op in
  the two no-block modes — but it is not optional: the modes are chosen per
  session, and a guard that skips it is correct until somebody runs a command.
- **"Did the seat change who answers this input?" is the first question for a
  red e2e on a live-region change**, and the four names above are its shape.
- **The keyboard branches that read block selection are downstream of the
  press.** Nothing in `_globalKeydown` was changed for this record: once the
  grid's press stops selecting a block, `selectedBlockId` is null exactly when
  it was null before ADR-0091.
