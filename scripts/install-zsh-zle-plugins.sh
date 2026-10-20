#!/usr/bin/env bash
# Install the three widget-wrapping zsh plugins, PINNED, for the
# shell-integration plugin-composition test (nocx-1ebrs.27).
#
#   scripts/install-zsh-zle-plugins.sh <dir>
#
# WHY THESE THREE, AND WHY BUILT FROM A PIN RATHER THAN FROM A PACKAGE MANAGER.
# `fast-syntax-highlighting`, `zsh-syntax-highlighting` and
# `zsh-autosuggestions` are the widget-wrapping plugins an ordinary zsh has,
# and every one of them REPLACES zle widgets — it saves the implementation it
# found and registers its own function that calls through. That convention is
# what `internal/shellintegration` has to compose with, in either order, so the
# test that pins the composition must run against the real upstream sources and
# not against whatever a package manager happens to have installed today: one
# of these plugins changes its wrapping when it feels like it, and then a green
# run no longer says which wrapping it measured.
#
# Homebrew's three formulae are the same three, but the bottles move. A
# checkout at a pinned commit cannot. Git is the only tool this needs.
#
# WHERE THE TEST FINDS THEM. Each plugin lands in <dir>/<plugin-name>, and the
# test resolves <dir> from NOCX_ZSH_PLUGINS. Under CI this script writes that
# variable to $GITHUB_ENV, so the test steps that follow see it without every
# job repeating it. Locally, pass the directory yourself:
#
#   scripts/install-zsh-zle-plugins.sh ~/.cache/nocx-zsh-plugins
#   NOCX_ZSH_PLUGINS=~/.cache/nocx-zsh-plugins go test -race -count=1 \
#     -run 'TestLocalZshPane_ThePluginStackIntegratesInEitherOrder' \
#     ./internal/shellintegration/
#
# The test FAILS, never skips, when they are absent — so this script is not an
# optional convenience for that test; it is its provisioning.
set -euo pipefail

DEST="${1:-}"
if [ -z "$DEST" ]; then
  echo "usage: $0 <destination directory>" >&2
  exit 2
fi

# THE PINS. Update one of these and the measured wrapping changes with it, on
# purpose and in one reviewable line.
FAST_SYNTAX_URL="https://github.com/zdharma-continuum/fast-syntax-highlighting"
FAST_SYNTAX_SHA="4672ad5dd9ad68a7effc1476d65afb7c584ce2b3"
ZSH_SYNTAX_URL="https://github.com/zsh-users/zsh-syntax-highlighting"
ZSH_SYNTAX_SHA="e6683f20e5c38d3e5447f45845d04ddda75d0f23"
AUTOSUGGESTIONS_URL="https://github.com/zsh-users/zsh-autosuggestions"
AUTOSUGGESTIONS_SHA="85919cd1ffa7d2d5412f6d3fe437ebdbeeec4fc5"

if ! command -v git >/dev/null 2>&1; then
  echo "install-zsh-zle-plugins: git is required to fetch the pinned checkouts" >&2
  exit 1
fi

mkdir -p "$DEST"

# install_one <url> <sha> <name> <entry-file>
# Idempotent: an existing checkout at the right commit is left alone, one at
# another commit is fetched and forced onto the pin, because a stale checkout
# is a measurement of something else.
install_one() {
  local url="$1" sha="$2" name="$3" entry="$4" dir="$DEST/$3"
  if [ -d "$dir/.git" ] && [ "$(git -C "$dir" rev-parse HEAD 2>/dev/null || true)" = "$sha" ]; then
    echo "  $name: already at ${sha:0:12}"
  else
    rm -rf "$dir"
    git init -q "$dir"
    git -C "$dir" remote add origin "$url"
    # A shallow fetch of the exact commit: cheap, and it cannot drift onto a
    # newer branch head between two runs.
    if ! git -C "$dir" fetch -q --depth 1 origin "$sha" 2>/dev/null; then
      echo "install-zsh-zle-plugins: $name: cannot fetch $sha from $url" >&2
      echo "  (a commit that is no longer reachable is a pin to re-decide, not to work around)" >&2
      exit 1
    fi
    git -C "$dir" checkout -q FETCH_HEAD
    local got
    got="$(git -C "$dir" rev-parse HEAD)"
    if [ "$got" != "$sha" ]; then
      echo "install-zsh-zle-plugins: $name: checked out $got, pinned $sha" >&2
      exit 1
    fi
    echo "  $name: ${sha:0:12}"
  fi
  if [ ! -f "$dir/$entry" ]; then
    echo "install-zsh-zle-plugins: $name: $entry is missing from the checkout" >&2
    exit 1
  fi
}

echo "installing the widget-wrapping zsh plugins into $DEST"
install_one "$FAST_SYNTAX_URL" "$FAST_SYNTAX_SHA" fast-syntax-highlighting fast-syntax-highlighting.plugin.zsh
install_one "$ZSH_SYNTAX_URL" "$ZSH_SYNTAX_SHA" zsh-syntax-highlighting zsh-syntax-highlighting.zsh
install_one "$AUTOSUGGESTIONS_URL" "$AUTOSUGGESTIONS_SHA" zsh-autosuggestions zsh-autosuggestions.zsh

# The test resolves this first, ahead of any Homebrew or system prefix, so a
# job that uses the pin measures the pin.
if [ -n "${GITHUB_ENV:-}" ]; then
  printf 'NOCX_ZSH_PLUGINS=%s\n' "$DEST" >> "$GITHUB_ENV"
  echo "wrote NOCX_ZSH_PLUGINS=$DEST to \$GITHUB_ENV"
fi
echo "NOCX_ZSH_PLUGINS=$DEST"
