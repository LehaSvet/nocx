#!/usr/bin/env bash
# Run one command, keep its output and its exit status for the quarantine gate,
# and leave the step green either way.
#
#   NOCX_TEST_LOG_DIR  the directory the job's gate step reads (required)
#   scripts/ci-run-logged.sh <name> <command> [args...]
#
# WHY THIS EXISTS. A pull request's CI has to decide, at the end of a job,
# whether the job failed only on tests `docs/testing/quarantine.json` holds under
# an open bead (ADR-xf4p9fzd, ADR-0bawnmfg). It cannot read its own job log —
# GitHub serves that only to the API, and the API wants a token the job's own
# `permissions: contents: read` does not give — so the job keeps the output it
# already has. Tee'd here, `<name>.log` is byte-for-byte what
# `GET /repos/{repo}/actions/jobs/{id}/logs` would return for the same step,
# which is why the promotion and this gate parse it with one function.
#
# WHY IT NEVER FAILS THE STEP. The verdict is taken once, at the end, by
# scripts/quarantine-gate.mjs, which sees every step's evidence at once. A step
# that failed on a quarantined test must not fail the job on its own, and a step
# that stops here on the first failure would hide the second step's failures
# from that verdict.
#
# The status is the FIRST command in the pipeline, not tee's: `pipefail` is set
# for exactly that, and PIPESTATUS is read on the very next line so nothing can
# overwrite it. bash 3.2 — still /bin/bash on the macOS runner — has both.
set -uo pipefail

name=${1:?usage: ci-run-logged.sh <name> <command> [args...]}
shift
: "${NOCX_TEST_LOG_DIR:?NOCX_TEST_LOG_DIR must name the directory the gate reads}"

mkdir -p "$NOCX_TEST_LOG_DIR"
"$@" 2>&1 | tee "$NOCX_TEST_LOG_DIR/$name.log"
status=("${PIPESTATUS[@]}")
printf '%s\n' "${status[0]}" >"$NOCX_TEST_LOG_DIR/$name.status"
exit 0
