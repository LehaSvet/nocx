#!/usr/bin/env bash
# The tracker export at a revision imports into br itself (nocx-8t2lo).
#
#   .githooks/check-tracker-import.sh [<rev>]     default :0, the staged export
#
# The backlog gate reads .beads/issues.jsonl through adapter.mjs, which parses
# leniently, so an export br refuses as a whole passed every check: from
# 6d3011922 to 8c09ef8cd main carried 46 rows in `br show --json` shape, and
# after a pull every br command in the main checkout failed and a fresh clone's
# `make init` could not build its database (nocx-dpbgx). So this asks br, the
# only reader whose answer matters: an empty tracker, the export, and br's own
# plan for importing it. `br sync --reconcile --dry-run` deserializes every row
# as `make init`'s import does and writes nothing, in 0.14 s on 4433 rows
# where the import itself takes 18 s (measured 2026-10-26, br 0.7.0); on the
# broken export both refuse with the same error. A second parser of br's
# format here would agree with br until the day it did not.
#
# The scratch tracker is a temporary directory outside every checkout: br run
# inside one resolves the main checkout's database instead.
set -euo pipefail

rev=${1:-:0}

if ! command -v br >/dev/null 2>&1; then
    printf 'br is not installed; see README.md, Agent tooling\n' >&2
    exit 2
fi

scratch=$(mktemp -d "${TMPDIR:-/tmp}/nocx-tracker-import.XXXXXX")
trap 'rm -rf "$scratch"' EXIT

if [ "$rev" = ":0" ]; then
    spec=":.beads/issues.jsonl"
else
    spec="$rev:.beads/issues.jsonl"
fi
if ! git show "$spec" >"$scratch/export.jsonl" 2>/dev/null; then
    printf 'no tracker export at %s\n' "$rev" >&2
    exit 2
fi

(
    cd "$scratch"
    br init --prefix nocx >/dev/null
    cp export.jsonl .beads/issues.jsonl
    if ! out=$(br --no-color sync --reconcile --dry-run 2>&1); then
        printf 'br cannot import the tracker export at %s:\n%s\n' "$rev" "$out" >&2
        printf 'Write the export through br (br sync --flush-only), never by hand or from br show --json.\n' >&2
        exit 1
    fi
)
