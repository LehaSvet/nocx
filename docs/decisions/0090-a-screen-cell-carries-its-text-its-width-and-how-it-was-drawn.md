# ADR-0090 — A screen cell carries its text, its width and how it was drawn

- **Status:** Accepted
- **Date:** 2026-10-19
- **Owner's decision:** 2026-10-19, verbatim "Добавляй" — take the faint-cell work
  into v0.6 as this option rather than the additive one, chosen over the
  alternative on `nocx-xn63t.6.52`.
- **Supersedes:** no numbered record. It takes back a declaration that lived
  only in the FROZEN helper ABI's own description of `screenCell`
  (`contracts/helper/identities.schema.json`), in `contracts/helper/README.md`'s
  framing of that freeze, and in five code comments — `internal/paneview`'s
  `Cell`, `internal/helper/proto`'s `ScreenCell`,
  `internal/helper/session`'s `screenFrame` ("a Frame is text, width and
  caret"), `internal/agentcapture`'s `Paint` ("A Frame is text, width and
  cursor") and `internal/agentdriver`'s own note on the Claude rule. The index
  row for this record carries no `Superseded by`, because there is nothing
  numbered to mark: ADR-0073 says in its own "What stands" that it does NOT
  decide the document's shape ("This record is the carrier, not the cargo"),
  and ADR-0072 decides the client's cell model and the order of the cutover,
  neither of which is a claim about what a cell contains.
- **Related:** `nocx-xn63t.6.56` (this decision), `nocx-xn63t.6.52` (where the
  defect and this option were measured), `nocx-xn63t.6.32` (the same defect
  first seen on a working day), ADR-0066 (one emulator, and the coordinator has
  none), ADR-0073 (the screen carrier), `contracts/helper/README.md` (what
  "frozen" means on this wire).

## Context

Claude Code draws a **grey prompt suggestion** into its input box after a turn
ends — `Try "fix lint errors"` — in SGR 2, the terminal's faint attribute, and
erases it the moment anybody types. nocx cannot tell that drawing from a
person's typing, so every caller that gates on "the input box is empty" reads
an empty box as occupied: a coordinator's queued wake waited out the suggestion
instead of delivering, and a worker's mail stayed unread for about 65 seconds
with a refusal written once a second (measured on the owner's stand,
2026-10-18).

The emulator has the fact and the wire did not. `internal/emulator` fills
`AttrFaint` from SGR 2 through its port, and `paneview.Cell` — the ONE
projection from an emulator row to the frame every reader acts on — was `{Text,
Width}`: the rendering was dropped where the frame is made. Every frame a RULE
reads then crosses the helper socket, and that shape's `screenCell` was
`{text, width}` under `additionalProperties: false`, declared by its own
description as the FROZEN helper ABI. The coordinator holds no emulator to
re-derive the attribute from (ADR-0066, `cmd/nocx-server` is
`CGO_ENABLED=0`).

What the rule did instead was state the agent's own **form**: the input-box
extractor matched `Try "…"` and captured nothing, so the box read empty. Both
directions of that cost were measured by the `nocx-xn63t.6.52` worker on the
corpus: the recorded SGR 2 drawing and the same characters painted WITHOUT
SGR 2 both read `BoxEmpty TRUE`, with `InputText ""` and `ok=true`.

## Decision

> **A screen cell is its text, the columns it occupies AND how it was drawn.
> The frame carries whether each cell was drawn faint, the helper ABI's
> `screenCell` requires that field, and the helper protocol version moves to 23
> so that a peer which cannot state a rendering refuses a peer which needs one
> — rather than decoding a shape it does not know.**

The attribute rather than the words, because the words are the agent's spelling
and the sentence is the person's. Faint is one BOOLEAN on a cell, not the
emulator's attributes bitset: one reader needs one decoration, and a bitset
would spend the wire's width on seven facts nothing reads while making the next
decoration a wire change again.

What the field costs is measured rather than hoped, on a fully blank grid, which
is the worst case for a fixed per-cell key: the JSON a screen read answers with
grows by 60.8% — 44,303 B to 71,183 B at 80x24, 110,576 B to 177,776 B at
120x40, 230,196 B to 370,196 B at 200x50, against the helper's `MaxFrameBytes`
of 1 MiB. A screen of real text grows less, because the characters it carries
are bytes the blank case does not have. It is paid once per screen read, on the
control plane only — the frame the RENDERER paints is a different socket and a
binary layout, and it is untouched.

## Why this rather than the obvious alternative

**Matching the agent's own words** is what the rule did, and it fails in both
directions at once — which is why the measurement above is the argument. A
Claude build that spells its suggestion differently blocks the wake again, and
a person who types that sentence is read as an empty box and has a wake pasted
into their draft. No pattern can close that, because the pattern is being asked
a question only the rendering answers.

**A second helper op that reports rendering beside the frame** (the additive
option) was declined by the owner. It gives one screen two readings that can
disagree — the frame the rule reads and the rendering read beside it — and every
consumer then has to know which of them is authoritative for which cell. One
field on the frame states the fact once.

**An optional field, or a decoder that reads a missing `faint` as false**,
would be a compatibility shim, and this is greenfield: a 22 helper and a 23
coordinator refusing each other at hello is the honest outcome, and it is what
the version number is for. `internal/helper/deploy` keys the install directory
by that number, so a newer generation installs beside an older one rather than
over it, and a generation still holding a session keeps the binary it was
started with.

**Reading the whole emulator style onto the wire** loses to the same argument
one level up: a frame is read by a rule and painted by a cell model that has
its own, richer vocabulary on its own socket (`contracts/session.frame.schema.json`),
and merging those two would make the frozen helper ABI carry a rendering
vocabulary its one reader does not use.

## What stands

- **The invariant, with both ends:** from the moment the emulator's row holds a
  faint cell to the moment a rule reads a frame, a reader either has the
  rendering or is refused. The mapping is `paneview.FromRows` and nothing else;
  the conversions are `internal/helper/session`'s `screenFrame` (which
  `readScreenCapture` shares) and `internal/helper/client`'s `frameFromWire`;
  the fence is `proto.Version`, checked by the host on the hello and by the
  client on the hello-ok echo.
- **What the box gate reads.** `Observation.BoxEmpty` still decides whether a
  writer may paste or wake; what changed is what "empty" means, and only in the
  direction the attribute supports: a row every one of whose captured values was
  drawn faint yields nothing, exactly as a row that matched with nothing
  captured does. A person's draft is read as text and refuses the write.
- **A frame's rendering survives a round trip through BYTES.** The capture
  format stores a frame by painting it (`agentcapture.Paint`), so the paint
  writes faint runs and the replay reads them back: a rule verified against a
  recorded mark is verified against the same rendering the live frame carried.
- **What this record does NOT decide.** The renderer's own frame and its attrs
  bitset (ADR-0072's cell model, `contracts/session.frame.schema.json`) are
  untouched. Nothing but FAINT crosses: colours, underlines and the other
  decorations stay in the emulator. The delivery gate's rule is unchanged — it
  still refuses a box that has something in it; this only makes "empty" mean
  what a person sees.
