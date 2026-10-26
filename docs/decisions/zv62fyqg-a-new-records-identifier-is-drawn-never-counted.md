# ADR-zv62fyqg — A new decision record's identifier is drawn, never counted

- **Status:** Accepted
- **Date:** 2026-10-26
- **Supersedes:** the numbering practice stated by "Numbering, as it actually is" in
  [INDEX.md](INDEX.md) and by "Adding one" there — "take the next free number" — and the
  `ADR-NNNN` spelling of a supersession that [AGENTS.md](../../AGENTS.md) prescribes for new
  records. It supersedes the PRACTICE and nothing already written: records 0001–0105 keep
  their numbers, their filenames and their citations, and INDEX.md's account of the four
  pairs repaired on 2026-08-28 is kept as the history it is.
- **Related:** `nocx-hyh2p` (this record). `nocx-yjvg5`, which repaired four numbers that
  named two records each on 2026-08-28, and `nocx-jp0h3`, which repaired three more and
  found "278 citations pick one by coin flip". Renumbers to 0045 (`c373794a4`), 0097
  (`ae8775567`), 0101 (`aaea73f81`) and 0105 (`788ac29a1`) are the same defect caught
  later.

## Context

Every identifier in this directory came from a counter: the next free integer after the
last file, chosen by whoever wrote the record, on their own branch. That is a number only
one side of a merge can see. Two branches that each add a record to the same base therefore
agree on the answer with CERTAINTY — both take the next number — and the merge has to
renumber one of them. By then citations exist: `ADR-0101` in a commit body, in a bead, in a
code comment, in a later record's `Supersedes:`. Renumbering repairs the file and strands
every citation, and the person reading one cannot tell which record it meant. That is what
`nocx-jp0h3` measured as 278 citations deciding by coin flip.

Renumbering is the repair, not the defect. The defect is that allocation needs coordination
and the system has none: branches do not talk, and a pull request does not reserve a number
until it merges, which is the moment too late.

The owner reports contributors hitting this in October 2026 and asked whether identifiers
could be assigned the way beads assigns its own. A beads id is short, random, and needs to
know nothing about what anybody else chose. That is the property the counter lacks, and it
is the whole reason to change the scheme.

## Decision

**A decision record's identifier is drawn at creation, and no counter takes part.** The file
is `docs/decisions/<identifier>-<slug>.md`, and the heading is `# ADR-<identifier> — <title>`.

**The shape is eight symbols of Crockford base32, lower case:**
`0123456789abcdefghjkmnpqrstvwxyz`. Forty bits. `i`, `l`, `o` and `u` are out, so an
identifier read off a screen and typed back is the identifier that was meant, and no
identifier collides with a digit or with a vowel it resembles.

**An all-digit draw is discarded and drawn again.** Eight symbols can spell `01234567`, and
that is the one shape a person cannot tell from the numbered era in a citation; the rule
that a drawn identifier never looks like a legacy number is enforced inside the one
function that draws (`isNumberLike`), not left to the reader. Nine draws in 100,000 are all
digits, so the redraw costs nothing.

**One function owns the draw: `drawIdentifier()` in `.githooks/adr-identifier.mjs`.** A
person runs `node .githooks/adr-identifier.mjs` and is printed an identifier. Every other
reader of the scheme — the check, the test, this record — reads that function or its
pattern; none of them draws its own.

**Records 0001–0105 keep their numbers and their filenames.** An accepted record is never
edited, and every citation written against a number must keep resolving. `0106` is the
updater pull request's record and lands as `0106-….md`: the numbers are RETIRED, not banned,
and a check that refused a unique `0106` would refuse the record already on its way. Nothing
in `docs/decisions/` is renamed or rewritten by this decision.

**A new record's supersession is spelled `Superseded by ADR-<identifier>`**, and a new
record's status line names the identifier it supersedes the same way. A record written
before the draw keeps the numeric spelling it was written with, because a citation to it is
what has to keep resolving.

**`INDEX.md` lists the records by date, oldest first, and says so.** A number was the
ordering key only because it was the allocation key. With a draw there is no order in the
identifier at all, so the order comes from the only thing that has one: the date each record
gives for itself — its `Date:` line, or the date it accepts when it carries none.

**A check refuses an identifier that already names another record**, in
`.githooks/pre-commit` for a staged change under `docs/decisions/` and in CI's `ci-backlog`
job. It names EVERY file that holds the identifier, because the person who needs the message
has seen only their own side of the collision.

### Why this rather than the alternatives

- **A reservation file, or a lock.** It replaces one coordination problem with another: the
  lock has to live on `main`, so a branch reserves against a `main` that moves underneath it,
  and the reservation is invisible until it is pushed. `AGENTS.md` already records what a
  merge slot cost and why it was removed.
- **Reading the remote before choosing a number.** It makes the number correct at the moment
  it is chosen and wrong at the moment it merges, which is the interval that matters — and it
  needs a network round trip to write a file.
- **A date prefix, `2026-10-26-<slug>.md`.** It is unique to the DAY. Two records on one day
  collide, which is exactly when two branches tend to add one.
- **A four-character draw**, which is the smallest that would fit `ADR-k3f9a`-shaped ids.
  Twenty bits is 1,048,576 identifiers, and ten thousand records give about 48 collisions
  expected among them — a collision about every other time this repository reaches ten
  thousand records, each one a renumber of the kind this record exists to end. Eight symbols
  is short enough to type from a citation and long enough to stop mattering.
- **Refusing a unique legacy number.** It would refuse `0106`, the updater pull request's
  record, which is a record on the same footing as `0001`. Retroactive refusal is also
  exactly the renumbering this record exists to end.

### What randomness does and does not prove

Two independent draws collide with probability 2^-40 per pair. Over N records that is about
N²/2^41: 10,000 records give 4.5 × 10^-5 — a collision is expected once in about 22,000
repositories' worth of this one's history. The identifier's own document says that, because
"it is random, so it is safe" is not an argument.

What the draw PROVES is narrower and is the thing that matters: neither branch consults
anything the other can see, so the two cannot agree by construction the way they did on the
counter. A residual collision is possible and the check is the backstop — the second pull
request to reach `main` is refused, names both files, and re-draws, which costs a rename
before anything cites it. A collision after a merge is repaired by renaming the newer
record and following its citations, and no such case has happened.

## What a person does now

1. `node .githooks/adr-identifier.mjs` prints an identifier.
2. Write `docs/decisions/<identifier>-<slug>.md`, headed `# ADR-<identifier> — <title>`.
3. Add the row to `INDEX.md` in the date slot its `Date:` line belongs in, in the same
   commit.
4. Commit. Pre-commit refuses the commit if that identifier already names a record, and
   names the file that holds it.

## What the next person inherits

- Two shapes of identifier, and no third: a four-digit legacy number and an eight-symbol
  draw. A name that is neither is refused by the check, because it is a record no citation
  can resolve.
- `.githooks/adr-records.mjs` is the one place that decides what a record's file name is,
  and it is what both the check and the tests read.
- `.githooks/adr-citations.test.mjs` reads every tracked document and refuses a citation
  that names no record, so renaming a record — the one thing this decision promised never to
  do — fails rather than quietly stranding the citations written against it.
- `INDEX.md` is sorted by date and one test holds it there, including the eight rows this
  decision moved to their date slot (0007, 0010, 0011, 0026, 0039, 0046, 0057, 0060) and
  0009, which moved later because its own `Date:` line says 2026-09-05 rather than the July
  2026-07-24 the decision it records was taken on.
- The four repaired numbers, the renumbers and the two files that disagreed with their own
  filenames stay written down in `INDEX.md`. They are history, not procedure, and the next
  person should read them as the reason this record exists rather than as an instruction.
