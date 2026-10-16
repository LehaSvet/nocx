# ADR-0087 — Linux user installs verify the signed release manifest

- **Status:** Accepted (owner decision, 2026-10-16)
- **Date:** 2026-10-16
- **Related:** [ADR-0003](0003-distribution-without-a-developer-id.md) (signed manifest integrity), [ADR-0052](0052-a-code-identity-without-a-publisher-identity.md) (release identity), [ADR-0007](0007-cross-platform-auto-update.md) (AppImage update-in-place), [ADR-0086](0086-a-third-party-homebrew-tap-may-clear-quarantine-for-the-unsigned-cask.md) (macOS Homebrew path), task `nocx-c8ydx`

## Context

Linux distributes nocx as one x86_64 AppImage. ADR-0007 makes that same file the in-app updater's replacement target. A distro package or root-owned install would conflict with that updater. The installer therefore needs a user-owned, stable path, while first-install integrity remains the signed manifest's responsibility under ADR-0003 and ADR-0007.

A `curl | sh` installer cannot rely on a previously installed nocx binary. It must verify the exact downloaded manifest bytes before interpreting their JSON, then verify the artifact against the authenticated digest and size. The stated application support floor does not guarantee an OpenSSL CLI version; the owner explicitly accepts OpenSSL 3+ as the installer prerequisite. No Python, jq, or unsigned fallback is accepted.

## Decision

Provide `scripts/install/install.sh`, served at `https://nocx.dev/install.sh`. It supports x86_64 Linux only, checks the platform and OpenSSL 3+ before network access, and fails with install guidance if the verifier is missing or too old. It fetches `manifest.json` and `manifest.json.sig` from the latest GitHub release. OpenSSL verifies the signature over the raw manifest bytes with the exact Ed25519 public key encoded by `internal/update.ReleaseKeyring`; parsing occurs only after that succeeds.

The POSIX awk parser accepts the signed release manifest's compact or indented JSON shape. It requires exactly one `linux`/`amd64`/`AppImage` artifact, a 64-hex SHA-256, a positive size, and a URL under `https://github.com/shady2k/nocx/releases/download/`. The downloaded AppImage must match both signed size and digest before installation. Parse, verification, download, or digest errors fail closed without installing files.

The verified AppImage is installed as the current user's `~/.local/bin/nocx`; the desktop entry and icon are installed under user data directories. The stable user-owned AppImage path is required for the updater's in-place replacement and avoids root. Re-running replaces that same path. The installer refuses macOS and points to `brew install --cask shady2k/nocx/nocx`; other operating systems and architectures are refused.

## Consequences

- OpenSSL 3+ is an explicit prerequisite for this installer; missing or older versions produce guidance before download or writes.
- The installer trusts the same release key as the in-app updater. Key rotation must keep the script's embedded key and updater keyring aligned.
- The parser is deliberately limited to the signed manifest's artifact shape; tests cover compact and indented release-tool JSON and malformed/ambiguous selection.
- README and the site list the installer first and the manual AppImage path second; installer copy must not imply that OpenSSL is bundled.
- No Linux package manager, root install, ARM build or Linux Homebrew formula is introduced.
