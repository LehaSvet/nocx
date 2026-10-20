# ADR-0094 — A program's synchronized output withholds the frame, and the hold fails open

- **Status:** Accepted
- **Date:** 2026-10-20
- **Beads:** `nocx-1ebrs.33` (owner finding 55)
- **Related:** AD-6 (single-owner state ownership), [ADR-0081](0081-recovery-and-prompt-boundaries-are-backend-owned-non-authoritative-effects.md)
  (the runtime owns VT state; observations cross only as typed contracts),
  [ADR-0066](0066-one-emulator-and-it-is-the-backends.md) (one emulator, and it is the
  backend's), [ADR-0008](0008-command-blocks-as-a-keyboard-first-ledger.md) (no frontend-derived
  fact may carry or reconstruct terminal output),
  [ADR-0073](0073-the-screen-frame-is-keyed-by-its-session-and-its-reader-and-continues-on-its-own-carrier.md)
  (the frame is a full snapshot on its own carrier),
  [ADR-0074](0074-a-command-boundary-is-settled-by-an-event-never-by-a-timer.md)
  (a command boundary is settled by an event, never by a timer).
- **Supersedes:** nothing. It decides when a frame is PUBLISHED, which ADR-0073 left open.

## Context

DEC private mode 2026 — synchronized output, `BSU` (`CSI ? 2026 h`) to `ESU`
(`CSI ? 2026 l`) — is a full-screen program saying "do not show what I am drawing until I
say so". Without it, a redraw is visible in pieces: the screen is cleared, the rows are
written one at a time, and the cursor is moved. ratatui and its kind wrap every redraw in
the pair precisely so a person never sees those steps.

Nothing on our side implemented it. The session runtime published a frame at the end of
every ingest, so a redraw that arrived as several reads became several frames — each a
half-drawn screen — and a scroll in a full-screen program looked jerky while the same
program in another terminal looked smooth (`nocx-1ebrs.33`). The renderer was not the
cause: it already keeps the newest snapshot and paints at most once per animation frame
(`frontend/src/terminal-content.ts`). What reached it was several distinct states, and it
painted each one it was given.

## Decision

### The mode is the emulator's, and it is read, never scanned for

The session runtime asks its emulator whether the program is holding the screen. The
emulator tracks mode 2026 as terminal state — as it tracks bracketed paste and the mouse
modes — and the port exposes it beside them (`emulator.Modes.SynchronizedOutput`,
`GHOSTTY_TERMINAL_DATA_MODE` in the pinned `libghostty-vt`). The runtime does not look for
`CSI ? 2026 h` in the bytes it feeds in: that would be a second parser of the stream
(AD-6), and the parser it already drives has the answer.

The read is a query and not a bit kept beside the terminal, because the terminal ends a
hold itself — on a full reset, and on a resize — and a copy in the caller would go on
believing in a hold nobody is in.

### A held ingest publishes nothing, and the release publishes the whole screen

`Session.publishFrameLocked` checks the mode before it reads the screen: while a hold is
open the frame is withheld and no cell is read or encoded. When the program releases it,
the next publish sends the screen the program finished. Nothing is lost by the wait: a
frame is the WHOLE screen (ADR-0073), so the frame that ends a hold carries every revision
the withheld ones would have.

An ingest that opens and closes the pair publishes once, and that is correct rather than a
missed hold: a publish happens at the end of an ingest, so the state it reads is the one
the program finished drawing. The emulator's own render-hold callback is not used for the
same reason it exists: it fires from inside `vt_write`, on the thread of the ingest, and a
runtime called back from inside an ingest cannot act without re-entering a terminal whose
lock the caller is holding (the same reasoning `Terminal.Effects` records).

What a hold withholds is the FRAME. Effects (a bell, a title, a clipboard write), the
departed-row stream and the program's own replies are untouched: none of them is a picture
of a half-drawn screen.

### The hold fails open after a bound

Mode 2026 carries no clock. A program that opens a hold and then crashes, blocks, or simply
never sends the `ESU` would otherwise leave a person looking at a screen nothing will ever
update — worse than the jerkiness the hold exists to fix. So a hold that has withheld a
frame for the whole bound publishes it anyway, and the next ingest that still sees the mode
open arms a fresh window. The rule is the one the integration hello already follows: a
protocol fact that never arrives degrades to the visible behaviour rather than to a stall.

The bound is 150 ms. It is nine paints of a 60 Hz display, and one paint per animation
frame is the cadence the renderer already keeps; nine is comfortably longer than a
full-screen redraw and short enough that a person reads the fail-open as a hiccup rather
than as a pane that has stopped. A runtime may be given a bound of its own
(`Config.SynchronizedOutputHoldBound`), which exists so a test can prove what a RELEASE
publishes without racing a fail-open it is not about, and can prove the fail-open at the
product's own value.

This timer decides no command boundary and settles no interval: ADR-0074's rule — an
event, never a duration — is about the lifecycle, and it stands. What the bound decides is
when an undelivered picture is shown.

### An attacher mid-hold is shown the last finished frame

The frame a mid-session attacher is handed, and the one a resend is served from, is the
frame the screen last PUBLISHED — never the half-drawn screen a hold is hiding. That is
what the terminal itself shows during a hold, and it is what the runtime's own `latest`
therefore keeps. A resend is the one publish a hold does not defer: a client that asked for
state is waiting for it, and a hold is a program asking not to be SHOWN drawing, not a
reason to answer nobody.

## Alternatives rejected

- **Scan the byte stream for `CSI ? 2026 h` and `l`.** A second parser, and the one AD-6
  forbids: the emulator owns the modes, and a marker scan would have to reassemble
  sequences across ingest boundaries to answer a question the emulator has already
  answered.
- **Let the renderer withholds paints instead.** The renderer already paints at most once
  per animation frame. The problem is what reaches it: several distinct states, each of
  which it is entitled to paint. Fixing that there would need the client to know the
  program's intent, which needs a parser and a fact carried to a client — ADR-0008's
  prohibition — and it would leave every other consumer of the frame (the helper, a second
  window) with the same half-drawn states.
- **Wait for the `ESU` with no bound.** One crashed program freezes its pane forever.
- **Publish on the next ingest, wherever the mode stands.** That helps a program that keeps
  writing and does nothing for the one that opened a hold and went quiet, which is exactly
  the case a bound exists for.
- **Take the emulator's render-hold callback.** A callback from inside the ingest path
  cannot publish: the ingest holds the session lock, and the mode's transition is a fact
  about a draw, while the publisher's unit is the ingest.

## Consequences

- A held ingest costs no cell reads and no encoding, so a program that redraws constantly
  inside one hold is cheaper than it was, not more expensive.
- A frame's revision is the revision it was read at at publication. It is no longer always
  the last ingest's: consumers already treat every frame as a coalescable full snapshot
  (ADR-0073), and the loss a hold causes is a coalescing the class already permits.
- Outside mode 2026 a frame is published per ingest, exactly as before; the mode is read
  once per publish whether or not a program ever uses it.
- `internal/sessionruntime` now holds a timer where it held none. It is the first, and the
  rule it does not break is stated above; a second one owes the same argument.
