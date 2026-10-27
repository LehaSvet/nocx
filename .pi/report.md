# Linux artifact recommendation — `nocx-1ebrs.66.1.2`

The proposed ADR recommends:

- **Flatpak:** do not ship. Revisit only with a user need, reviewed sandbox permissions, maintained runtime, and Flathub route.
- **deb:** do not join v0.8; recommend later only after a nocx-owned apt repository, separately approved signing-key custody, supported baseline, and upgrade tests are established.
- **rpm:** do not join v0.8; recommend later under the equivalent repository, signing, support, and upgrade-test conditions.
- **AUR:** do not join v0.8; recommend it as the first later native-package experiment, through a maintained PKGBUILD pointing to a versioned GitHub Release artifact with a pinned checksum. AUR is not described as signed by nocx.

All native packages update through their store/package manager, never nocx's
in-app AppImage updater. The updater must visibly explain this for a recognized
packaged install and must distinguish an unknown unpacked/development binary.
The AppImage remains the only Linux artifact in the v0.8 nightly and promotion
pipeline; no extra signing keys, repositories, validation jobs, or publication
failure paths are added to the pipeline whose goal is to remove release
bottlenecks.

## Not established from the repository

- No Flatpak build, Flathub submission, or sandbox permissions are present.
- No deb/rpm repository, metadata signer, host, signing-key owner, or rotation
  policy is present; the repository does not establish which Debian/Ubuntu or
  RPM distributions nocx will support.
- No AUR account/maintainer, PKGBUILD, or publishing cadence is established.
- The updater currently refuses Linux self-update when `APPIMAGE` is absent and
  mentions package-manager updates for deb/rpm, but no source found proves
  reliable format detection or a user-visible message for Flatpak and AUR.
- The bead and existing deferred feature establish the package intent and
  host-WebKit boundary, not the operational cost or user's demonstrated demand
  for each format. The relative costs and conditions in the ADR are
  recommendations for owner approval, not measured build timings.
