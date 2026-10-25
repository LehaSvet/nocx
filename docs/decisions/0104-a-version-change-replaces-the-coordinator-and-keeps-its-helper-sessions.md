# ADR-0104 — A version change replaces the coordinator and keeps its helper sessions

- **Status:** Accepted (owner decision, 2026-10-25; `nocx-1kejz.1`)
- **Date:** 2026-10-25
- **Supersedes:** only ADR-0101's cross-Version coordinator reuse clause and the
  consequences that depend on it. Same-Version/different-Commit nightly reuse
  and the updater's pair-certification guarantees remain unchanged.

## Context

ADR-0101 allowed a window to reuse any protocol-compatible coordinator, including
an older release. The update health gate refuses final certification of a mixed
version pair, but a cask upgrade does not pass through that gate. A new window can
therefore keep running the old coordinator and send every new pane to its old
embedded helper and shell integration. Discovery `ProtocolVersion` stayed at 4
between 0.7.0 and 0.7.1 despite shell-integration and lifecycle changes.

The helper, not the coordinator, owns the PTYs and SSH sessions. Content-addressed
helper generations already have independent endpoints. Orderly coordinator
shutdown detaches its helper channels. Persisted session bindings and existing
readoption retain the generation needed to recover the sessions.

AD-1 keeps the binary and control planes separate. AD-7 keeps session identity
server-authoritative. AD-8 gives generation routing to the existing helper opener
and readoption owners; this decision adds no migration or alternate discovery
owner. ADR-0101's updater rollback and held-pair handling remain defensive checks,
not permission for a new launcher to select an older-Version coordinator.

## Decision

A different `Version` is a coordinator takeover reason whether discovery
`Protocol` matches or differs. Compare Version before Protocol and ignore Commit:
a compatibility bump shipped with an upgrade must not route that upgrade through
a notice that falsely claims its helper sessions ended. Stop the old coordinator, wait for it to stop, then start and
verify the current coordinator. Keep the single profile-keyed `srv.sock`,
`srv.lock` and launcher `spawn.lock`: there is one coordinator owner, not two
version-keyed coordinators. The current launcher's compatibility check verifies
both `Version` and `Protocol`, including the answer after spawning or losing a
spawn race. It does not compare `Commit`.

Do not stop helper daemons or send `close-session` because of a version takeover.
The coordinator holds attachments, not the sessions themselves. Log the takeover.
There is **no user-facing notice**: nothing was lost. Never emit
`NoticeSessionsLost` on this path. The launch result distinguishes a takeover
from the existing protocol-incompatibility replacement path, which retains its
existing notice semantics.

A matching `Version` with a different `Commit` continues to reuse a
protocol-compatible coordinator. Nightly policy is unchanged. A protocol mismatch
with the same Version remains a loud incompatibility, not permission to accept
the peer. Every newly started coordinator must match both Version and Protocol.

New panes use the helper generation installed by the serving coordinator.
Carried-over bindings use existing reconciliation and attachment to their recorded
helper generation. One coordinator can serve old reattached panes and
new-generation panes together. Do not migrate running processes or replace their
scripts. A failed attachment keeps its actual refusal/unknown state; it is not
evidence that a process ended. Only persisted bindings can be recovered by this
path; this decision invents no recovery for unrecorded tabs.

Old helpers retain their current lifecycle: `OnDrained` stops the daemon after
the last session record is removed and admitted spawns finish. A shell exit alone
retains its record. Exited unclaimed records retain the existing bounded TTL
(default 24 hours); live shells are never swept.

## Compatibility coverage and limits

Extend the protocol-surface fingerprint to include embedded shell-integration
scripts and lifecycle wire sources. Mutation assertions must prove that changing
either moves the fingerprint and cannot refresh the same-version golden. Such a
change requires a deliberate discovery compatibility bump and review of old
helper-generation readoption. This extends a compatibility signal; it does not
make helper generation identity interchangeable with protocol compatibility.

The current `endpoint.Path` uses the **compiled current helper `proto.Version`**,
and helper dialing requires a matching helper protocol. A recorded content hash
is not universal old-protocol attachment support. Both 0.7.0 and 0.7.1 use helper
protocol 23, so that limit does not block the reported upgrade. Future helper
protocol bumps need an explicit old-session recovery policy and implementation.
Bumping the discovery surface version does not itself bump the helper protocol.

## Consequences and acceptance

The updated coordinator can reattach a live session on its old helper generation
and open a fresh pane on the current generation without ending the old command.
The launcher's stop failure refuses to start a second coordinator. A spawned
coordinator with the wrong Version or Protocol is refused rather than silently
accepted. Existing helper drain behavior needs no new mechanism.

Acceptance must prove:

1. Different Version stops the old coordinator, starts and verifies the current
   one, and produces no session-loss notice, with matching or differing Protocol.
2. A live helper session keeps its identity and usable I/O across coordinator
   shutdown and reattachment; a new pane reaches the new helper generation.
3. Same Version plus different Commit reuses the running coordinator.
4. Script and lifecycle mutations change the fingerprint and cannot overwrite a
   same-version golden, including when the working golden is missing.

Source inspection is not an execution result. Check reports must distinguish
written acceptance tests, checks that actually ran, and CI evidence.
