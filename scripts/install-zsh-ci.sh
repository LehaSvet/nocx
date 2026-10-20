#!/usr/bin/env bash
# Install zsh on an ubuntu CI runner, configured the way a user's zsh is.
#
# Two jobs in ci.yml need it and for different reasons, which is why it lives
# here rather than inline in either. ci-backend runs the shell-integration
# launcher tests; ci-linux runs internal/pty, whose TestClose_EndsAnInteractiveShell
# opens a real login+interactive zsh to prove a closed tab does not leak the
# process. Both hard-fail when zsh is absent rather than skipping — a skip there
# reports a leaked process as a pass — so "the runner happens to have it" is not
# a thing either job may rely on.
#
# It was inline in ci-backend alone, and ci-linux was created beside it without
# it (nocx-9527464a split one job into two). internal/pty travelled into the new
# job and its zsh subtest failed on every run from the split onwards. One owner
# for "how CI gets zsh" is what stops the next split from re-splitting it.
set -euo pipefail

sudo apt-get update -qq
sudo apt-get install -y -qq zsh

# ubuntu's zsh ships completion directories that are group-writable, so compinit
# refuses them and asks "Ignore insecure directories and continue [y] or abort
# compinit [n]?" — on a tty, interactively. Tests that drive a real PTY have
# nothing to answer with, so they hang until their own timeout and report
# whatever they were waiting for instead of the question they were being asked.
#
# Fixed rather than suppressed: ZSH_DISABLE_COMPFIX=true would silence the
# prompt and leave CI running a zsh configured unlike any user's.
zsh -c 'autoload -Uz compaudit; compaudit 2>/dev/null || true' \
  | while read -r d; do [ -n "$d" ] && sudo chmod -R go-w "$d"; done

echo "=== compaudit after fix (empty is correct) ==="
zsh -c 'autoload -Uz compaudit; compaudit 2>/dev/null || true'


# THE WIDGET-WRAPPING ZSH PLUGINS (nocx-1ebrs.27), for the composition test.
#
# `fast-syntax-highlighting`, `zsh-syntax-highlighting` and
# `zsh-autosuggestions` replace zle widgets, and internal/shellintegration has
# to compose with that convention in either order. The test that pins it fails,
# not skips, when they are absent — so this is provisioning, not a convenience,
# and it belongs beside zsh itself: one owner for "how CI gets the shells and
# the plugins the shell tests prove".
#
# PINNED CHECKOUTS, not the distribution packages: see the installer for why a
# package that drifts cannot say what wrapping it measured. It writes
# NOCX_ZSH_PLUGINS to $GITHUB_ENV when that is set, so every later step in the
# job inherits it without repeating the path.
"$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)/install-zsh-zle-plugins.sh" \
    "${HOME:-/root}/.nocx-ci/zsh-plugins"
