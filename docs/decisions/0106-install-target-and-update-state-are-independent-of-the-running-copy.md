# ADR-0106: The install target and update state are independent of the running copy

- **Status:** Accepted (owner-delegated placement decision, 2026-10-26)
- **Task:** `nocx-1ebrs.66.1.1`; also addresses `nocx-azxe.8`
- **Supersedes:** The update placement refusals attributed to ADR-0007 §7.7 in the worker brief: translocation and a read-only running location are not themselves reasons to refuse an update. ADR-0007's platform abstraction and integrity checks remain binding.

## Context

The executable may be running from a downloaded, translocated macOS application or from the read-only filesystem mounted by AppImage. Neither location identifies a writable installation destination. A lock or journal beside that executable prevents even startup reconciliation in those cases. Conversely, moving staging into profile data would lose the same-filesystem guarantee required by atomic installation.

## Decision

Three paths have separate owners:

1. The running executable identifies the copy currently executing, not the destination to overwrite.
2. The install target identifies the copy to replace and subsequently launch. On macOS, probe `/Applications` and select `/Applications/nocx.app` when writable; otherwise create and probe `~/Applications` and select `~/Applications/nocx.app`. Refuse with actionable guidance if neither location is writable. On Linux, use nonempty `$APPIMAGE`; only when unset use the executable path.
3. The active profile's `storage.AppPaths.DataDir()` owns `.nocx-update.lock` and `.nocx-update-journal.json`. Reconciliation of an idle updater does not require writing the install directory.

The desktop composition root constructs the updater and retains its selected install target for restart. The transaction journal retains its own install target: subsequent reconciliation, rollback, and health finalization operate on that recorded destination even if selection has changed since the transaction began.

Platform preflight precedes transaction-state, lock, download, and staging mutation in `Apply`. Extraction and the swap/backup paths stay next to the install target. Existing targets use atomic exchange; a first installation uses a same-filesystem rename into the previously absent target. There is no previous target to restore after a failed first installation, so rollback explicitly directs the person to the original copy or a known-good release rather than pretending that a backup exists.

The coordinator compatibility, pair-health certification, and helper-session lifetime rules of ADR-0101 and ADR-0104 do not change.

## Delegated assumption

The owner delegated the choice of a standard writable macOS destination. The decision uses system Applications first and user Applications second, without elevation or an installation-location picker. Writability is established with a temporary-file probe, not inferred from permission bits. The fallback directory may be created during target selection; a platform preflight refusal during `Apply` makes no transaction mutation.

## Consequences and verification

A translocated or mounted copy can install elsewhere without modifying its own running files. Profile state remains writable independently of that copy. Atomic exchange remains on the destination filesystem; moving all updater files into profile storage was rejected because it would break this invariant.

Linux tests inject Darwin directory probes, exercise actual creation of the home Applications directory, cover read-only startup placement and mutation-free refusal, and run real AppImage replacement through the desktop constructor and restart seam. These checks do not prove macOS translocation or Launch Services behavior; the owner's macOS pass remains required before accepting the full task.
