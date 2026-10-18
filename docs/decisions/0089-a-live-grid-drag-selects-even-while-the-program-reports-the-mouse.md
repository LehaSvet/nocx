# ADR-0089: A live-grid drag selects even while the program reports the mouse

Date: 2026-10-18

Status: Accepted (owner's run-4 observation on `nocx-zg3k3.15.5`).

Supersedes: none. Takes back the drag half of the gate `nocx-zg3k3.15.1` put in the
live selection gesture, and keeps the rest of it.

## Context

A full-screen alternate-screen program that enables DEC mouse reporting — Claude Code,
k9s, mc — left a person unable to keep a selection. Dragging across the program's output
painted a highlight and the highlight was gone at the release.

Measured, in a real pty running a real reporting program (`e2e/live-grid-drag-in-a-fullscreen-program.spec.ts`):
the pane's own selection gesture (`.term-grid-selection`, painted from the cell snapshot)
never ran while the accepted frame said the program reports the mouse, so the highlight
the person saw was the BROWSER's selection over the painter's rows — a second claimant on
one input — which the pane's own release path clears again when it returns the keyboard to
its input element. No frame repainted it, because the painter's model never held it.

That stand-down was `nocx-zg3k3.15.1`'s, and its reason was real at the time: the live
region was xterm, this gesture's `preventDefault` on `pointerdown` suppressed the
compatibility `mousedown` xterm encoded the report from, and the program never received
its click.

## Decision

The live grid's drag is the person's whether or not the program reports the mouse. The
gesture runs, the painter holds the selection, and it is still painted after the release
and after the frames that follow it.

What the program receives does not change: the pointer sequence is emitted by the pane's
input renderer from its own listeners, which this gesture is not in front of. Press, drag
motions and the release all still reach a reporting program — the e2e proves it from the
program's own screen, because the fixture paints back every byte it is sent.

A DOUBLE click while the program reports the mouse stays the program's
(`nocx-zg3k3.15.4`), which is the one mouse-reporting gate left in the gesture.

## Why this rather than the obvious alternative

The obvious alternative is xterm's forced-selection escape hatch, which this repository
already plans to use for the picker (`nocx-7ul8`): plain drag to the program, a modifier
(Option on macOS, Shift elsewhere) for the person. It is rejected here for the reason the
gate itself turned out to be wrong, not for taste: the modifier would answer "the program
must keep its pointer sequence", and the program keeps it either way. What the modifier
would add is a discovery cost on top of the defect the person reported — the selection
they make with the mouse they already have.

The e2e measures both halves at once, so this decision cannot be satisfied by stealing:
the program's screen shows the press, the drag motions and the release it was sent, and
the selection is on screen over exactly the cells the drag covered.

## What the next person inherits

- `nocx-7ul8`'s picker gesture is a DIFFERENT gesture from this one and still needs its
  own modifier: it mints a frozen frame at mouse-down, which a plain drag must not do.
  Nothing in this record stands its modifier down, and its "xterm's escape hatch" premise
  now describes this drag as well.
- A program whose own drags a person uses (vim's visual mode, tmux's pane borders) gets
  both actions from one drag now. That is the trade this record takes deliberately, and
  it is the reason the modifier above is rejected rather than merely deferred.
- Nothing else about mouse reporting moved: clicks, the wheel, and what the program
  receives for them are the pane's input renderer's, unchanged.
