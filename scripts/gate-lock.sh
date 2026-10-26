#!/bin/sh
# One heavy run at a time, machine-wide. Sourced by
# .githooks/containerized-tests.sh, scripts/ci-linux.sh, scripts/ci-frontend.sh,
# e2e/run-in-container.sh and .githooks/pre-commit.
#
# WHY IT IS SHARED RATHER THAN PER-CALLER: the first version of this lock lived
# in the pre-push hook alone, and within the hour a `make ci-full` walked
# straight past it and put four jobs on the machine while five agents were
# working. A lock that only one of several entry points takes is not a lock. The
# heavy paths are the ones that start containers, so every one of them takes it.
#
# TWO NAMESPACES, because one lock for everything is wrong in the direction that
# costs most: a `make ci-full` holds it for half an hour, and a commit that waits
# half an hour behind a containerized suite is a commit made with --no-verify —
# the reflex AGENTS.md most wants not to form. So the directory name carries a
# namespace and there are two resources:
#
#   (no name)    the container jobs. Unchanged for every existing caller.
#   frontend     the pre-commit hook's typed eslint and tsc (nocx-owbpb): two
#                worktrees committing at once measured 2.1 GB and 2.0 GB RSS on
#                this shared 12 GB box, 707 MB left, and the hook's own history
#                records the gate child that was OOM-killed and reported as
#                `FAIL: eslint (frontend)` (nocx-y6d9j). Two commits contend
#                with each other; a commit does not contend with a container run.
#
# THE NAME IS THE MACHINE, NOT THE WORKTREE, and that is the whole point: each
# worktree checks out its own copy of this script, so a lock inside one would be
# invisible to the others, which is the entire case being defended against. It
# lives in /tmp, keyed by uid so two accounts on one machine do not block each
# other. NOCX_GATE_LOCK_DIR is a seam for tests, so a test never queues behind a
# real commit or blocks one.
#
# The lock is a directory because mkdir is atomic on every POSIX filesystem and
# macOS ships no flock(1).
#
# It never blocks forever. A holder that died leaves a stale directory,
# reclaimed by checking whether its pid is still alive; a holder that is merely
# slow eventually exhausts the wait, and then we warn and run anyway. A gate
# that can hang a commit is a gate people disable with --no-verify, which costs
# more than the contention it avoided.
GATE_LOCK_NAME=${GATE_LOCK_NAME:-}
GATE_LOCK_DIR=${NOCX_GATE_LOCK_DIR:-}
if [ -z "$GATE_LOCK_DIR" ]; then
    GATE_LOCK_DIR="/tmp/nocx-hook-gate-$(id -u)"
    if [ -n "$GATE_LOCK_NAME" ]; then
        GATE_LOCK_DIR="$GATE_LOCK_DIR-$GATE_LOCK_NAME"
    fi
    GATE_LOCK_DIR="$GATE_LOCK_DIR.lock"
fi
GATE_LOCK_WAIT=${NOCX_GATE_LOCK_WAIT:-1800}
GATE_LOCK_HELD=0

# gate_lock_acquire [what this run is about to do]
#
# The argument is what a WAITING run is told, so a silent wait never happens. A
# wait with no explanation is a hang nobody can diagnose, and a hang is what
# makes somebody reach for --no-verify.
gate_lock_acquire() {
    _purpose=${1:-a heavy containerized run}
    _waited=0
    _announced=0
    while :; do
        if mkdir "$GATE_LOCK_DIR" 2>/dev/null; then
            printf '%s\n' "$$" > "$GATE_LOCK_DIR/pid" 2>/dev/null || true
            printf '%s in %s\n' "$_purpose" "$PWD" > "$GATE_LOCK_DIR/holder" 2>/dev/null || true
            GATE_LOCK_HELD=1
            return 0
        fi

        # Stale? The holder is gone if its pid no longer exists. An unreadable
        # or empty pid file is treated as stale too: the only way to get one is
        # a holder that died between the mkdir and the write above.
        _holder=$(cat "$GATE_LOCK_DIR/pid" 2>/dev/null || echo "")
        if [ -z "$_holder" ] || ! kill -0 "$_holder" 2>/dev/null; then
            rm -rf "$GATE_LOCK_DIR" 2>/dev/null || true
            continue
        fi

        if [ "$_announced" = 0 ]; then
            _doing=$(cat "$GATE_LOCK_DIR/holder" 2>/dev/null || echo "another run")
            printf 'WAIT: %s (pid %s) holds the machine-wide %s lock.\n' \
                "$_doing" "$_holder" "${GATE_LOCK_NAME:-gate}" >&2
            printf '      Waiting for it to finish, so the two do not compete for this machine.\n' >&2
            _announced=1
        fi

        if [ "$_waited" -ge "$GATE_LOCK_WAIT" ]; then
            printf 'WARN: the machine-wide %s lock is still held after %ss — running anyway.\n' \
                "${GATE_LOCK_NAME:-gate}" "$GATE_LOCK_WAIT" >&2
            return 0
        fi
        sleep 2
        _waited=$((_waited + 2))
    done
}

gate_lock_release() {
    [ "$GATE_LOCK_HELD" = 1 ] || return 0
    GATE_LOCK_HELD=0
    rm -rf "$GATE_LOCK_DIR" 2>/dev/null || true
}
