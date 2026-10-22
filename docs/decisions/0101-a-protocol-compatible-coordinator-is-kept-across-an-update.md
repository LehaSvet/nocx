# ADR-0101 — A protocol-compatible coordinator is kept; the build difference is not a replacement reason

- **Status:** Accepted
- **Date:** 2026-10-21
- **Decided by:** the owner, 2026-10-21, on `nocx-1ebrs.53` (the accepted update-coordinator
  compatibility decision), drafted by the worker from the launcher's measured behaviour and
  the nightly scheme ADR-0097 had already fixed.
- **Related:** [ADR-0007](0007-cross-platform-auto-update.md) (the updater whose health gate
  this record keeps), [ADR-0097](0097-trunk-based-releases-with-a-nightly-channel.md) (the
  nightly scheme that makes the commit-only update the ordinary case), the nocx-server design
  (`.internal/specs/2026-08-28-the-nocx-server-design.md`, D4 and §4),
  `internal/coordinator/launcher.go`, `internal/update/pair.go`.
- **Supersedes:** only the commit-difference replacement rule — design D4's sentence "A1 must
  therefore compare **build** and protocol versions" as `Launcher.incompatibility` implemented
  it: a coordinator whose `Version` or `Commit` differs from the window's was stopped and
  replaced, loudly, with its sessions. Nothing else in D4 moves: a protocol mismatch still
  replaces loudly and honestly (`NoticeSessionsLost`), and the update health gate
  (`update/pair.go`, `certifyPair`) still refuses to certify a pair whose coordinator is not
  the version the update installed. ADR-0007, ADR-0052 and ADR-0097 stand untouched.

## Context

A coordinator is a detached daemon that outlives its windows, so an update swaps the bundle
on disk while the daemon keeps running the code it was started from. Until this record, the
launcher treated that as a defect to undo: any window whose `Version` or `Commit` differed
from the running daemon's stopped it — killing every session it was running — and raised its
own. D4 chose that in A1 deliberately, as the crude half of a two-part answer, with graceful
session-preserving handover deferred to A2.

ADR-0097 made the cost of that rule immediate. Nightlies stamp **the next stable version**
and differ from each other only by commit, so a person on the nightly channel updates to a
new commit every day — and under the old rule every one of those restarts killed the
coordinator and every live session in it, with a dialog saying so. A daily update that
destroys the terminal's core promise (work survives the window) is not a channel anyone
stays on. `nocx-1ebrs.53` was filed to settle this before the first nightly publish, and the
owner accepted the rule below.

The fact the old rule leaned on is also weaker than it looked. The handshake
(`internal/coordinator/handshake.go`) carries exactly one compatibility signal a window can
act on: the discovery `ProtocolVersion`. There is no separate version for the WS JSON-RPC
control plane; a window cannot detect an RPC difference at launch today, whatever it
compares. Version and Commit are identity, not compatibility.

## Decision

**The discovery protocol version is the one compatibility boundary between a window and a
coordinator. A coordinator that speaks this build's protocol is used as it is, whatever its
`Version` and `Commit`. A coordinator that speaks a different protocol is stopped and
replaced exactly as before — announced first, `NoticeSessionsLost`, sessions honestly lost.**

- `Launcher.incompatibility` compares the protocol only. The build fields are logged (the
  existing "found a running coordinator" line carries version, commit and protocol) and
  never acted on.
- One owner: that comparison is the single place a window decides whether a running
  coordinator is usable. The discovery client states facts and decides nothing
  (`client.go`); the updater's pair certification answers a different question — "did the
  update's own server run?" — and keeps its own rule (below).
- What "compatible" means, stated once: `ProtocolVersion` covers the whole window↔coordinator
  surface — the discovery socket's shapes **and** the loopback WS control plane that the
  token it hands out opens. Any change to either that one side of an update cannot serve to
  the other — a removed JSON-RPC method, a changed frame, a token handover that no longer
  fits — bumps the constant. Until it is bumped, an old coordinator and a new window are
  definitionally compatible, commit difference included.
- Takeover needs no new mechanism. A kept coordinator serves until it exits — a stop, a
  reboot; exiting after the last client detaches is the design's §4 "when the daemon exits",
  still future work. When nothing is serving, the next launch raises the new build's
  coordinator on the ordinary cold-start path. That is the whole of "when no window holds
  it, the new coordinator may take over".

## Consequences

**Nightly-to-nightly — the case this record exists for.** Same version, new commit: the
restarted window reuses the running coordinator, its address and its token, so every live
session survives the update. The update's health gate certifies the pair immediately,
because `certifyPair` compares the coordinator's `Version` with the installed one — equal —
and deliberately never the commit (`update/pair.go`: the release manifest names a version
and no commit). `TestReportHealthy_SameVersionDifferentCommit_Certifies` already pins this.

**A version-bumping update with a surviving coordinator.** The window still keeps the old,
protocol-compatible coordinator and its sessions; `certifyPair` refuses to finalise
(coordinator version ≠ installed version), so the rollback journal stays pending. The
existing `Reconcile` rule then governs honestly: within three launches the old daemon
usually turns over (a stop or reboot), the new build's coordinator serves, the pair
certifies and the update finalises; if it never does, the bundle rolls back to the version
the coordinator is running. Either way no session is lost, and the shipped pair is never
mixed on version. This is the accepted trade — the alternative was the old rule's certain
session loss on every update.

**A protocol bump.** Unchanged: announced first, stopped, replaced, `Replaced: true` on the
launch, `NoticeSessionsLost` in words a person reads. The honesty D4 demanded survives
exactly where D4's kill rule does not.

**What a green suite cannot see.** A commit that changes the control plane without bumping
`ProtocolVersion` produces a window and a coordinator that agree to talk and then disagree
about the conversation — a defect this record makes possible and the old rule made
impossible. The mitigation is the one already stated: the constant is the contract; a
reviewer who changes a wire shape bumps it in the same commit, and the protocol-mismatch
test is the tripwire that keeps replacement itself honest.
