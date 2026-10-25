# ADR-0104 — A connection's delivery mode is not the machine's consent

- **Status:** Accepted
- **Date:** 2026-10-25
- **Supersedes:** the sentence in [ADR-0034](0034-consent-belongs-to-the-machine-not-the-connection.md)
  that lets a saved connection stand in for the machine's permission — "what a saved
  connection can still say about the binary is on the delivery axis: `relay` means 'allow
  it on this connection without asking'" — and the resolver arm that implemented it ("an
  explicit helper choice is the consent for the binary", `internal/app/consent.go`).
  ADR-0034's decision that the answer belongs to the machine, keyed by its host-key
  fingerprint, stands unchanged, and so do ADR-0069's ask and its rationale. Neither record
  is edited.
- **Related:** [ADR-0033](0033-auto-is-the-name-for-not-yet-answered.md) (`auto` is "not yet
  answered"), [ADR-0068](0068-the-helper-is-decided-by-the-connection-never-by-a-feature.md).
  Bead `nocx-1kejz.3`.

## Context

A saved connection has a Delivery mode (`auto|raw|script|helper`) and, beside it, a
persisted helper-consent value (`unknown|granted|denied`). The consent field was added in
2026-08-05 with a hint promising a rule — "a `helper` selection without granted consent
behaves as raw" — and on 2026-08-16 ADR-0034 deleted its owner: consent to deploy a binary
is the MACHINE's answer, keyed by the destination's host-key fingerprint, and a profile is
not a machine.

The field was never removed from the tree, and the resolver kept a third reading of it. Its
`helper` arm returned `DesiredHelper` without consulting either store or field — an explicit
helper choice _was_ the consent — so a connection set to `helper` deployed the binary
whatever its consent value said, and the editor's consent control governed nothing while
telling the person it governed everything.

The regression the owner met on a real machine: a connection at `helper` with consent
`Unknown — not asked` behaved like `raw` — nothing published, nothing logged, nothing on the
pane — and the consent was never asked. Three answers existed for one question and none of
them was the machine's.

The choice this record settles, and the alternative it rejects. The bead that owns it
offered two: ask consent when a helper-delivery connection opens and the machine has not
answered, or fall back to script delivery and say so on the pane. The second was rejected
because stating it honestly needs a surface the product does not have: `conventional` plus a
reason is how the pane says "integrated delivery did not happen", and a fallback that _does_
integrate scripts cannot use it without claiming the pane is a plain terminal. It would also
leave the person with no door — the machine grant is written by the connect-time ask and by
the footprint screen, and a silent fallback gives neither a reason to appear.

## Decision

**A connection's Delivery mode says which method it wants. It does not say the machine has
permitted anything.** `helper` joins `auto` in waiting for the machine's answer, and the
answer is obtained at the connect-time ask (ADR-0069) — the one ask, raised once per
machine, and nothing on the pane is left to imply the deployment happened.

Concretely, `internal/app/consent.go`'s ladder asks one store for one answer:

- `raw` and `script` never reach it — an answer, not a gap.
- `auto` and `helper` both do: a recorded `granted` serves the helper silently, `denied` is
  never asked again and never upgraded, and a machine nobody has answered is a
  `ConsentRequired` the connect-time caller raises. A machine with nothing to offer — no
  artifact for its platform — is refused, because there is nothing to ask about.

**The connection editor offers no consent control.** The per-connection value it wrote is
not a decision's input anywhere, and a field a person can set that governs nothing is the
soft degrade AGENTS.md forbids: it says "Granted" and is asked anyway, or says nothing and
believes the helper is blocked. What replaces it is a sentence saying where the answer is
given and that it is remembered per machine.

## Rationale

- **Two questions, two owners** (AD-8). Which method a connection uses is the connection's;
  whether a binary may be deployed on a machine is the machine's. Merging them let one
  answer stand for the other, which is how a deployment happened with nobody's permission
  recorded.
- **The ask already existed and was already reachable.** `ConsentRequired` was raised,
  transported, rendered and answered on every open path (ADR-0069); the only thing missing
  was that a `helper` connection never produced one. Asking costs no new surface and no new
  word.
- **"Answered" is a state the product can show.** An ask answered at connect leaves a
  recorded answer, and every later open of every connection to that machine is silent —
  which is the "asked once" property the field was trying to express per profile.
- **Nothing silently degrades.** A helper connection that is not answered cannot open at
  all: the open refuses, the pane says the connection needs an answer, and the ask is what
  the person sees. That is the opposite of a mode that produces neither its behaviour nor a
  sentence.

## Consequences

- A machine's grant is now required for `helper`, so an e2e spec or a person whose connection
  declares `helper` answers the ask once per machine. `e2e/ssh-helper-happy-path.spec.ts`
  and `e2e/ssh-helper-vault-key.spec.ts` do exactly that, and the second connection to the
  same host in the vault-key spec asserts the ask does not come again.
- `git.open` on a `helper` connection whose machine has not answered no longer installs a
  binary: it treats `ConsentRequired` as `Refused` (ADR-0068), and the next connect asks.
- A carried-over session is not re-adopted over a helper channel while its machine has not
  answered, because re-adopting one opens that channel (ADR-0034). The pane reads
  unreconciled and the refusal names the truth — "nocx has not been told", not "this machine
  no longer consents", which would have been the opposite of what happened to every session
  opened before this rule.
- The per-connection `helperConsent` field itself is now unreachable from any surface and is
  read by no decision. Removing it — profile, patch path, backup projection, contracts and
  its tests — is ADR-0034's own unfulfilled consequence and is tracked as `nocx-jxcrt`
  rather than smuggled into this change.
- `refusedHelperReason` stops telling a person that setting the Delivery mode to Helper
  "allows it outright": no mode does.
