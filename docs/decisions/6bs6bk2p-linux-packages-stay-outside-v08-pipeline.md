# ADR-6bs6bk2p — Linux packages stay outside the v0.8 release pipeline

- **Status:** Proposed (owner approval required)
- **Date:** 2026-10-27
- **Related:** [ADR-0007](0007-cross-platform-auto-update.md) (AppImage-only
  self-update), [ADR-0035](0035-appimage-carries-webkits-helper-processes.md)
  (host WebKitGTK versus bundled helper path),
  [ADR-0087](0087-linux-user-installs-use-the-signed-release-manifest.md)
  (signed-manifest AppImage installer),
  [ADR-0097](0097-trunk-based-releases-with-a-nightly-channel.md) (nightly and
  promotion pipeline), task `nocx-1ebrs.66.1.2`, feature `nocx-j3prm`

## Context

The v0.8 outcome is a daily changed-main build and a byte-identical promotion,
not a catalogue of install formats. ADR-0097 removes a measured 40-plus minute
CI queue from release iteration by building and publishing only the artifacts
that support its update proof. Every additional artifact adds build inputs,
platform dependencies, validation, signing or provenance, publication, retention,
and a failure mode that can stop the nightly or promotion. It also lengthens the
path whose purpose is to make building and publishing cease to be the bottleneck.

The AppImage remains the one Linux artifact in that outcome and the only one
updated in-app. Its signed release manifest authenticates the exact bytes
(ADR-0003, ADR-0007, ADR-0087); it is self-contained because host WebKitGTK
versions and helper paths are not a stable runtime contract. ADR-0035's byte
patch is specific to the bundled library and must never touch a build linked to
the host's WebKitGTK.

A native deb, rpm, or Arch package takes the opposite dependency model: it
requires the host GTK/WebKit libraries and lets their own packages provide their
matching helpers. That avoids the AppImage helper-path failure, but it does not
make the artifact self-updating. The owning package manager or store must deliver
updates. A standalone release asset is not a maintained package repository, and
a signed nocx manifest does not itself establish a package manager's trust
chain.

There is no Flatpak packaging, repository, signing, or sandbox policy in the
current tree. Flatpak adds a sandbox permission and runtime-maintenance surface
as well as another build. It is not merely another package extension.

## Decision

**Keep the v0.8 nightly and promotion artifact set unchanged: Linux AppImage only.
Do not add Flatpak, deb, rpm, or AUR builds to the v0.8 nightly or promotion
pipeline.** The format-by-format recommendation and intended later boundary are:

| Format      | Recommendation                                                                                                                                                                                                                                           | Hosting and signing                                                                                                                                                                                                                                                                                                                                 | Update owner                                                                        | Pipeline                                                                                                                                                                 |
| ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Flatpak** | Do not ship. Reconsider only if users require sandboxed distribution and nocx has a reviewed permission model, a maintained runtime, and a demonstrated store route.                                                                                     | No repository or signer is adopted. If reconsidered, target Flathub; Flathub's build/review and repository signing would be the distribution trust path, not the nocx AppImage updater.                                                                                                                                                             | Flatpak/Flathub update mechanism. No in-app update.                                 | Not v0.8; a separate later decision and implementation, only after the conditions above.                                                                                 |
| **deb**     | Recommend a later release, not v0.8. A Debian package is a useful native path, but publishing a lone `.deb` does not provide unattended package-manager updates or a maintained security channel.                                                        | If admitted later, publish through a nocx-owned apt repository with repository metadata signed by a dedicated apt signing key; do not imply that the AppImage manifest signs apt metadata. Host and key custody must be approved before implementation.                                                                                             | `apt`/the configured system package manager. No in-app update.                      | Later, after an owner-approved repository, key custody/rotation, supported Debian/Ubuntu baseline, and package install/upgrade tests exist.                              |
| **rpm**     | Recommend a later release, not v0.8, for the same reason as deb; adding a second repository ecosystem now multiplies maintenance and release failure paths without advancing the AppImage update outcome.                                                | If admitted later, publish through a nocx-owned RPM repository with signed repository metadata and a dedicated signing key, separate from the AppImage manifest key. Host and key custody must be approved first.                                                                                                                                   | `dnf`/the configured system package manager. No in-app update.                      | Later, after repository/key ownership, supported distribution baseline, and install/upgrade tests are agreed. Do not block v0.8 promotion on it.                         |
| **AUR**     | Recommend a later, bounded first native-package experiment, ahead of expanding to deb and rpm. It directly serves the Arch use case already recorded in `nocx-j3prm`, while avoiding a project-operated binary repository. It is not a v0.8 deliverable. | A project-maintained PKGBUILD in the AUR points to a versioned GitHub Release source artifact and checks its pinned SHA-256. AUR Git commits are not a nocx publisher signature, and the PKGBUILD is not a signed binary package; the checksum protects the fetched release bytes only if the PKGBUILD itself is trusted. Do not claim AUR signing. | `makepkg`/`pacman` via AUR helper or the user's package workflow. No in-app update. | Later, after a maintainer and review/update cadence are assigned, and package build/install/upgrade checks prove host WebKitGTK use and exclusion of the AppImage patch. |

The recommendations are intentionally not equivalent to admission. The ADR
chooses no additional v0.8 artifact. Separate implementation work must be filed
from this decision, and no later format enters the nightly or promotion artifact
set merely because this record recommends reconsidering it.

### Updater refusal for packaged installs

A non-AppImage Linux install must never fetch or apply an AppImage update. The
current Linux updater refuses when `APPIMAGE` is unset; its refusal text already
says package-manager updates are responsible for deb/rpm, but the implementation
does not establish reliable identification for every packaged form. The product
must present a visible, actionable message for recognized Flatpak, deb, rpm, and
AUR installs: updates come from the named store/package manager, and in-app
updates are unavailable. It must not silently log and continue offering an
in-app update. An unrecognized unpacked or development binary must retain a
separate refusal, not be mislabeled as a package. Detection must not rely on
`APPIMAGE` absence alone to claim a specific package type.

These refusals preserve the boundary: the in-app updater remains AppImage-only;
package managers and stores own native-package updates; and no native build may
run the AppImage helper-path patch or bundle WebKitGTK as though it were an
AppImage.

## Consequences

- v0.8 has no new Linux publication destination, package signing secret, key
  rotation duty, repository metadata, or package-specific promotion transaction.
  Its nightly and promotion continue to build, verify, sign, and publish only the
  AppImage on Linux.
- This keeps the pipeline's critical path and release failure surface small. The
  cost is that users who require distro-native installation do not get it in
  v0.8, and they must continue using the signed AppImage installer or manual
  AppImage path.
- Native formats remain technically plausible and useful, but each brings
  ongoing package compatibility, install/upgrade testing, and maintenance. AUR
  has no project-controlled signing boundary; apt and rpm require repository
  operations and key custody; Flatpak requires sandbox/runtime ownership.
- The updater's recognized-package messages are a product requirement for any
  future package implementation, not evidence that current code recognizes all
  four formats. The package implementation task must include the user-visible
  refusal and prove the AppImage patch is unreachable from native packaging.
- No build task is created by this document. Once the owner accepts the
  recommendations, the admitted format's build and updater-recognition work is
  filed separately; no format is assumed admitted before that decision.

## Revisit when

- Users demonstrate a concrete need for native Linux installs and a maintainer
  accepts the continuing work, rather than only the one-time package build.
- The owner accepts who operates and signs apt/rpm repositories, how keys are
  backed up and rotated, and which distributions are supported.
- A Flatpak permission model and maintained runtime can be reviewed against the
  app's desktop, filesystem, terminal, and update needs.
- AUR maintenance has an assigned owner and a tested release-source/checksum
  process.
- The package build, install/upgrade proof, and publication can join the
  nightly/promotion path without making a daily release depend on an unowned
  repository or signing service.
