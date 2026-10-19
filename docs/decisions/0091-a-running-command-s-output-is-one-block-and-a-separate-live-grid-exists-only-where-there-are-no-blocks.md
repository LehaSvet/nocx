# ADR-0091 — A running command's output is one block; a separate live grid exists only where there are no blocks

- **Status:** Accepted
- **Date:** 2026-10-19
- **Decided by:** the owner, on 2026-10-19, on the defect `nocx-zg3k3.14.26` ("a running
  block's own output past its first screenful is painted outside the block"). Verbatim:
  _"Нет, так не должно быть. Нет никакой живой поверхности, у нас блок. Вот если блоков нет
  (полноэкранный режим или без интеграции) тогда да."_
- **Supersedes, in part:** `.internal/specs/2026-09-15-terminal-screen-mockup-decision.md`,
  the two sentences that fix the live region as a sibling of the running block — line 286,
  _"Never move a live xterm descendant into the frame; live output remains the
  controller-owned sibling."_, and line 135, _"a running `.cmd-block` contains only its
  header; `.xterm-live-container` is a sibling."_ That record is **not** edited: it is the
  evidence of what was decided then, and every citation of it keeps pointing at it.
- **Related:** [ADR-0078](0078-the-terminals-history-has-two-tiers-and-the-live-one-is-the-emulators-own.md)
  (the two history tiers — it decides where rows COME from, not which box paints them),
  [ADR-0009](0009-dom-scrollback-with-explicit-cell-geometry.md) (the frozen block's cell
  geometry), [ADR-0024](0024-authenticated-shell-integration-channel.md) (the lifecycle
  projection a running block is).

## Context

With shell integration on the normal screen, a running command's rows are painted in two
boxes: the block paints the rows the durable record has already taken — the ones that left
the screen — into its own `.cmd-output`, and `.terminal-live-container`, a SIBLING of the
block, paints the screen the command is still writing. The owner met the consequence twice
on the stand: the running block's own surface ended after the first rows of its output, the
rest was painted below it with an extra half-row of space at the boundary, and one row of
that half-row was the block's own bottom padding paid at a joint inside one body
(`e2e/running-block-body-seam.spec.ts` measured 6px + 2px = 8px, a 25px step where every
other row pair steps 17px).

The padding was the symptom. The structure was the defect: the product tells a person that
a command's output is the block — and then the block's box stops one screenful in.

## Decision

1. **With shell integration on the normal screen, every row of a running command's output
   is inside its block.** The block's surface runs unbroken from its header, through its
   status and Stop, through the rows the durable record holds and the rows still live, to
   its last row and its trailing space. There is no second surface under it.
2. **A separate live grid exists only where there are no blocks:** an alternate-screen
   program (the pane belongs to the program), and a session with no shell integration
   (there are no execution intervals, so no blocks). Both keep exactly what they had.
3. **The live region is therefore a DESCENDANT of the running block element in running
   mode, and a direct child of `.scrollback-inner` in the other modes.** It is still owned
   by `ScrollbackController` — the controller builds it, sizes it, clips it and applies the
   echo shift to the grid inside it — but where it is SEATED is `BlockManager`'s answer,
   because the manager is what owns the block elements it is seated beside or inside.

## Why the old answer stopped holding

The mockup decision's two sentences were written as a _rendering_ rule: keep the live
xterm surface out of the block frame so the frame's markup and the live painter cannot
entangle. That was right, and it is not what is being taken back — the region stays
controller-owned and outside the frame's own header/output markup. What stopped holding is
the conclusion drawn from it: that because the region is a sibling, the block's BOX must
end where its own painted rows end. A person reading a running command does not see two
components; they see one block, and the product's own vocabulary — "the block printed this"
— was contradicted by its own layout.

ADR-0078 was cited for the sibling arrangement in the work that preceded this record. It
does not decide it: 0078 decides that the rows above the live screen come from the durable
tier and that the live tier is the emulator's own scrollback. Both remain true. Which DOM
box paints them is this record's question, and the owner answered it.

## What changes in the tree

- `frontend/src/scrollback/blocks.ts` — the seat: `holdLiveInRunningBlock()` /
  `releaseLiveRegion()`, called by the controller as the mode changes; the region is
  re-seated last in the block after every `paintStoredRows` (the block's own body order is
  header, committed rows, then the live body); released BEFORE `freezeBlock` replaces the
  running element, because a child of that element would leave the document with it.
- `frontend/src/scrollback/controller.ts` — `setRunning()` holds the region in the block;
  `setIdle()`, `setUnstructured()` and `enterFullscreen()` release it. The mode is the one
  input: the controller already owns it.
- `frontend/src/style.css` — the block's own body order and padding: `.cmd-output` pays no
  bottom padding while the block runs (the body does not end there), the live region pays no
  top padding when the block already holds rows (the body did not start there), and the
  block's `<body>`-trailing space is paid once, by the live region that ends it.
- `frontend/src/terminal-content.ts` — `liveInlineInsetPx` follows the DOM: the inline
  inset between the scroller's `clientWidth` and the grid is the live row's own padding
  PLUS its ancestors' up to the scroller, which is the block's gutter once the region is
  nested. A second constant here would be nocx-vydj returning.

## What the next person inherits

- **The two modes with no blocks are the test of the rule, not an exception to it.** A
  full-screen program's pane, and a markerless session's, are checked by
  `e2e/running-block-body-seam.spec.ts`: the live grid is there NOT inside any `.cmd-block`.
- **A child of a block is a child of an element that gets REPLACED at the freeze.** Anything
  nested in a running block must be moved out before `freezeBlock` swaps the element, and
  `clearAll`/`applyClearBoundary` owe the same.
- **The block's extent is no longer a number the controller derives.** Its height is its
  contents: header, the rows the record holds, and the live body. `runningLiveCap` still
  caps the LIVE body (the scroller's height less the header); the block itself may be
  taller than the pane and scroll like any other block.
