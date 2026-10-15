# ADR-0086 — A third-party Homebrew tap may clear quarantine for the unsigned cask

- **Status:** Accepted (owner decision, 2026-09-30)
- **Date:** 2026-10-15
- **Related:** [ADR-0003](0003-distribution-without-a-developer-id.md) (distribution without a Developer ID), [ADR-0052](0052-a-code-identity-without-a-publisher-identity.md) (code identity without publisher identity)
- **Supersedes in part:** ADR-0003's claim that no Homebrew route exists; ADR-0052's claims that Homebrew remains closed and the manual quarantine step is the only first-install route. All other decisions in those records stand.

## Context

The 2026-09-01 Homebrew rule disables unsigned casks in the official `homebrew/cask` tap. ADR-0003 and ADR-0052 treated it as a ban on every tap. That reading is too broad: Homebrew's maintainers state that third-party taps may add casks and are not checked for signing status ([Homebrew discussion #6482](https://github.com/orgs/Homebrew/discussions/6482)). A third-party cask can therefore distribute the same unsigned release bundle. Homebrew still applies quarantine to downloaded files; the tap can remove that attribute after installation.

This is not a Gatekeeper bypass hidden from the person installing. The person explicitly chooses a third-party tap whose post-install action removes quarantine, the same trust decision as running the documented `xattr -dr com.apple.quarantine` command manually. The tap README must say this plainly. This behavior has not yet been measured on a clean Mac; the cask's install and launch behavior remains subject to the feature's recorded manual acceptance pass.

## Decision

Publish a cask named `nocx` in our own third-party tap, `shady2k/homebrew-nocx`. It installs the release bundle and removes `com.apple.quarantine` from `/Applications/nocx.app` using Homebrew's supported post-install step (`postflight_steps` in Homebrew 6; raw `postflight` is deprecated). The tap's README states that it strips quarantine and explains the consequence. Mark the cask `auto_updates true`: nocx's signed in-app updater owns updates, and `brew upgrade` must not replace or downgrade the app.

The official `homebrew/cask` tap remains out of scope. The cask uses the same release bundle and leaves its code identity unchanged. It neither signs nor notarizes the app, and does not change the update-integrity design.

## Consequences

- A person may install nocx through `brew install --cask shady2k/nocx/nocx`; the tap's post-install action clears quarantine, so the first launch should not require a separate manual `xattr` command.
- Installing from this tap means trusting its maintainers and accepting that its cask removes macOS quarantine. The tap documentation must make this action explicit.
- Direct downloads from GitHub Releases still require the manual quarantine step. The app remains unsigned and unnotarized; Gatekeeper does not attest to its publisher.
- The signed manifest continues to protect update integrity. The cask does not alter the bundle, the self-signed code identity, or the in-app updater.
- The release workflow needs a credential limited to the tap repository to update the cask after a release. Its exact GitHub secret name and setup are implementation details to be confirmed before that workflow step is enabled.

## Revisit when

A Developer ID becomes available, or Homebrew changes the third-party-tap policy or supported post-install mechanism. In either case, reassess the trust and install path rather than silently changing the cask's behavior.
