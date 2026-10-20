# Heavy runs go to the SkyPilot cluster

Owner decision, 2026-10-19 (bead `nocx-360js`). This page is for every agent
working in this repository: what to run where, and the tools that make it so.

## The brief paragraph

Paste this into a worker brief, or say it in your own words - the substance is
the one thing that matters:

> Run tests through `nocx-sky-test`, not on this machine. `nocx-sky-test
<worktree> '<command>'` runs the command in a fresh stage of your worktree on
> that worktree's own SkyPilot cluster, and exits with the command's own status.
> The first call starts the cluster (about two minutes); later calls reuse it and
> take about half a minute. `--down` when you are done. This machine (vm-agents)
> is 12 GB of SHARED memory: a vitest or `go test -race` run here fails for want
> of memory rather than for a defect, and three load-shaped failures were filed
> from exactly that (`nocx-wsgrt`, `nocx-u8saf`, `nocx-n05m6`). The heavy local
> spellings are also refused by a guard; if you must run one here, the refusal
> names the override.

Why it is in `AGENTS.md` as well as here: `AGENTS.md` is what a worker actually
reads before writing code, and a rule that lives only in a page nobody opens is
not a rule. This page carries the detail; `AGENTS.md` carries the sentence and
points here.

## What is installed, and where

All of it is OUTSIDE the repository, by design: the recipes are the owner's and
live on the SkyPilot API server, and these are this machine's clients for them.
A checkout cannot carry them, and a reader of a pull request cannot see them -
which is why the PR that added them quotes their contents.

| Path                                                      | What it is                                                                                  |
| --------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `~/.local/bin/nocx-sky-test`                              | the client: stage, launch/exec, exit marker, `--fetch`/`--down`/`--status`                  |
| `~/.local/bin/nocx-sky-janitor`                           | takes down idle `nocx-*` clusters; `--dry-run`, `--status`, `--selftest`, `--install-timer` |
| `~/.local/bin/nocx-local-guard`                           | the refusal shim, reached as `go`, `npm`, `npx`, `vitest`, `tsc` (symlinks beside it)       |
| `~/.config/systemd/user/nocx-sky-janitor.{service,timer}` | the 5-minute schedule, installed by `nocx-sky-janitor --install-timer`                      |

The recipes they use, on the SkyPilot API server:

| Recipe             | Image                                                                         | Size         |
| ------------------ | ----------------------------------------------------------------------------- | ------------ |
| `recipes:nocx`     | `nocx-runtime` = this repo's `.githooks/images/ci-linux` image + Node 24.22.0 | 2 CPU / 4 GB |
| `recipes:nocx-e2e` | `nocx-e2e-runtime` = this repo's `e2e/Dockerfile` image                       | 4 CPU / 8 GB |

`nocx-sky-test` picks `recipes:nocx`; `--e2e` picks `recipes:nocx-e2e`. One
cluster per worktree per recipe, named from the worktree's own path, so two runs
of one worktree share it and two worktrees never do. A per-cluster `flock`
serializes two runs of the same worktree - the janitor takes the same lock, so a
takedown cannot cross a run.

## What runs where

```bash
# the ordinary case: the tests for what you touched
nocx-sky-test "$PWD" 'go test -count=1 ./internal/transport/...'
nocx-sky-test "$PWD" 'cd frontend && npm ci && npm test'

# the e2e image (Playwright), coordinator-sized
nocx-sky-test --e2e "$PWD" 'bash e2e/container-entry.sh'

# bring a result back before the cluster goes
nocx-sky-test --fetch "$PWD" test-results /tmp/test-results
nocx-sky-test --down "$PWD"
nocx-sky-test --status
```

A run's exit status is the command's own. A run that never finished - an
OOMKilled pod, a lost connection, a setup failure - has no `NOCX_RUN_EXIT`
marker, and that is exit 3 with the log's tail, never a pass. An unreachable
SkyPilot API is exit 4 with what the CLI said; nothing falls back to this
machine, deliberately, because a fallback would put the run back on the shared
12 GB at exactly the moment the cluster is broken.

### Two files the stage does not carry, and what to do about each

The stage is a filtered copy of the worktree, so two gitignored inputs behave
differently, and a run that ignores this looks like a build defect in the pod:

- **`frontend/dist` is staged** (about 3 MB) when the worktree has it, because
  the ROOT package embeds it (`all:frontend/dist`). Without it `go test .` fails
  at build time on the pod while passing locally - which is a setup failure, not
  a test result. Build the frontend once (`make frontend-build`, or any target
  that produces `dist`) before running the root package there.
- **`build/libghostty-vt` is NOT staged** (about 280 MB). Any package that links
  `internal/emulator/ghostty` needs the archives present in the pod's own tree,
  so prefix the command with `make vt-archives &&`:

  ```bash
  nocx-sky-test "$PWD" 'make vt-archives && go test -count=1 ./internal/emulator/...'
  ```

A run that fails on either of these has tested nothing. Read the first error
line before concluding anything about the code.

## The guard, and what it does not do

`nocx-local-guard` is a symlink farm in `~/.local/bin` over the tool names:
`go`, `npm`, `npx`, `vitest`, `tsc`. It refuses `go test`, `npm test`/`npm run
test`, `npx vitest`, `npx tsc`, `vitest` and `tsc` - and only inside a nocx
checkout, recognised by an ancestor directory whose `go.mod` says `module
github.com/shady2k/nocx`. Every other project runs exactly as before; a scratch
module is what proves it (`nocx-local-guard --selftest` runs both directions).

`NOCX_ALLOW_LOCAL_TESTS=1` runs locally anyway, and says so in one line first.
The coordinator's merged-tree gate is the reason the override exists:

```bash
NOCX_ALLOW_LOCAL_TESTS=1 make ci-full
```

It is not a sandbox for the shell's own children, and the make rule is the other
half of that: **a command started by `make` is never refused**, because a
build-path target is not a heavy run an agent chose - `make dev-web`,
`make dev`, `make build` and `make require-local-helper` all pass. The rule
cannot be an argument test: the Makefile's `go test
./internal/helper/deploy/artifacts -count=1` and an agent's `go test
./internal/app` are the same tool with the same kind of argument. The caller is
the only signal, and make exports `MAKELEVEL` into every recipe's shell;
`MAKEFLAGS` can be empty and must never be the test. The guard was first
installed WITHOUT this rule and stopped `make dev-web` at `require-local-helper`

- a build needing an override to build - which is why the rule is written into
  the guard's header with the measurement beside it.

`nocx-local-guard --selftest` and `nocx-sky-janitor --selftest` are the tests for
both, and each runs with no cluster and no network.

## `make ci-full` on SkyPilot: the proposal (bead item 5)

Not wired as a whole yet; the last subsection says what is. The owner's words -
"you just hand over the right image" - are literally true here: `recipes:nocx`
IS the `ci-linux` job's image, and `recipes:nocx-e2e` IS the `e2e` job's image.

### 1. Which target calls what

A new `ci-full-sky`, never a rewrite of `ci-full`:

| `ci-full` prerequisite | on SkyPilot?           | how                                                        |
| ---------------------- | ---------------------- | ---------------------------------------------------------- |
| `ci-os-split`          | no, stays local        | a static derivation from build constraints, seconds        |
| `ci-local-ssh-split`   | no, stays local        | the same, for the `nocx_local_ssh` partition               |
| `ci`                   | no, stays local        | host-side: lint, build, root and frontend gates, `test-ci` |
| `ci-mac`               | no, CANNOT             | `macos-latest` is the target OS; no Linux pod is macOS     |
| `ci-backend`           | yes, `ci-backend-sky`  | `recipes:nocx`, one command                                |
| `ci-linux`             | yes, `ci-linux-sky`    | the same cluster, its half of the package list             |
| `ci-frontend`          | yes, `ci-frontend-sky` | `recipes:nocx` (Node 24.22.0 is in the image)              |
| `ci-e2e`               | yes, `ci-e2e-sky`      | `recipes:nocx-e2e`, one pod per browser                    |

The commands are the jobs' OWN commands, not a second copy of them:
`scripts/ci-linux.sh` and `scripts/ci-frontend.sh` gain an in-place mode
(`NOCX_CI_IN_PLACE=1`) that skips Docker and runs the same inner command it
would otherwise have handed to `docker run`. One owner per job definition.

### 2. What the images already contain, and what must be reproduced

`scripts/ci-linux.sh` builds `nocx-ci-linux:ubuntu-24.04` from
`.githooks/images/ci-linux` and then `docker run`s into it. On the pod that image
IS the pod, so the `docker build`, the `docker run` flags, the `/src:ro` bind
mount, the `gomod`/`gobuild` named volumes, the `chown /cache`, the `useradd`
and the `setpriv` privilege drop all disappear - and each disappears as a
DIFFERENCE, not as a no-op (see section 5). What must be reproduced is exactly
the inner command: the two `go test -race -count=1 -p $TEST_P` passes, the
tagged `nocx_local_ssh` pass, and the alloc-budget pass, with the keyring
variant wrapped in `dbus-run-session` + `gnome-keyring-daemon`, both of which
the image carries.

`scripts/ci-frontend.sh` `docker run`s `node:24-bookworm` with two
worktree-keyed `node_modules` volumes and the git common directory mounted. The
recipe image carries Node 24.22.0 - the same major - so the runtime matches; the
volumes are meaningless on a fresh pod (a cold `npm ci`, which is what CI does
anyway), and the git common directory must be handled as section 5 says. Its
inner command reaches the repo as `/work`, so the in-place mode makes `/work` a
symlink to the staged tree rather than editing the command.

`e2e/run-in-container.sh` runs the e2e image with `PW_PROJECTS` per browser, a
6 GiB `--memory` cap, three cache volumes and the same `node_modules` volumes,
calling `bash -euo pipefail /work/e2e/container-entry.sh`. The pod IS that
image, so the command is `bash e2e/container-entry.sh`, which already installs
its dependencies and takes its caches from `$HOME`; the memory cap becomes the
pod's own limit, and one pod per browser is what keeps the two projects out of
each other's way - the reason the local script already runs one container per
browser.

### 3. What must be passed in

Every value a job reads from its environment, and all of it inside `RUN_CMD`
rather than through `sky --env`: `sky` validates an `--env` against the task
YAML's declared `envs:` block, and the recipes declare `RUN_CMD` alone, so an
undeclared name is a launch error rather than an extra variable. Inside the
command there is no such limit, and the quoting hazard disappears with it.

| Value                    | Where it comes from                                                                      |
| ------------------------ | ---------------------------------------------------------------------------------------- |
| `PKGS`                   | the Makefile's `PORTABLE_PKGS_CMD` / `OS_PKGS` for each half                             |
| `NOCX_LOCAL_SSH_PKGS`    | `make -s print-local-ssh-pkgs`, or the half the target owns                              |
| `NOCX_ALLOC_BUDGET_PKGS` | `make -s print-alloc-budget-pkgs` (whole-tree runs only)                                 |
| `NOCX_CI_TEST_P`         | `GO_TEST_P`, default 2                                                                   |
| `NOCX_CI_CPUS`           | nothing on the pod: the pod's own size is the cap                                        |
| the tags                 | `gtk3,nocx_framecheck` and `gtk3,nocx_local_ssh,nocx_framecheck`, as the runner has them |
| `PW_PROJECTS`            | `chromium` or `webkit`, one pod each                                                     |

`make -s print-...` works on the pod because `make` is in the ci-linux image
(build-essential). The package lists are passed as environment, so no list is
ever split by a shell.

### 4. What stays local, always

- `ci-mac`. The macOS job runs on `macos-latest`, which is the target OS; no
  Linux pod can be it. `make ci` keeps covering the host side.
- The static gates: `ci-os-split`, `ci-local-ssh-split`, `lint-ci`, the
  ratchets, the wire contracts, `pre-commit`. They read the tree in seconds and
  need nothing a bigger machine offers.
- Anything needing this checkout's Git metadata, the developer's `$HOME`, the
  login keychain or the OS keystore. The stage carries no `.git` at all, so
  `go build`'s VCS stamping and `git config safe.directory` have nothing to read
  - which is a difference the proposal keeps rather than papers over.
- The `gate-lock`: on SkyPilot two worktrees have two clusters, so the lock that
  serializes heavy containerized runs on THIS machine has nothing to serialize.
  The per-cluster `flock` takes its place for one worktree; what replaces
  nothing is the memory ceiling, which is what the lock was really protecting.
- Results a developer wants on disk: `nocx-sky-test --fetch <worktree> <path>
<local folder>` before `--down`.

### 5. What a person loses

This is the honest section. The local container IS byte-for-byte the CI job;
a pod is the same IMAGE and not the same machine, and these are the differences:

- **Root instead of a dropped uid.** `scripts/ci-linux.sh` deliberately runs the
  suite as a non-root user, because root bypasses mode bits and a
  permission-sensitive test then passes where no developer and no runner would
  let it pass. On the pod the suite runs as root unless the in-place mode
  recreates the drop - measured 2026-10-19, `id -u` inside the pod says `0`.
- **The repo is read-write.** The container mounts `/src:ro`; the pod's stage is
  writable. A test that writes into the tree therefore succeeds on the pod and
  fails locally - measured the same day, a `touch` at the stage root succeeds.
- **Core count, and the number is not the one on the tin.** Measured in a pod on
  2026-10-19: `nproc` says **1**, `/sys/fs/cgroup/cpu.max` says `200000 100000` (a
  2-CPU quota), `/sys/fs/cgroup/memory.max` says `3999997952` (about 4 GB).
  `free -m` inside the pod reports the NODE's memory, not the pod's limit: 15970
  MB on the node a second pod landed on the same afternoon, and the 19.9 GB the
  first reading recorded. The number is the cluster's, so it moves with the node,
  and a test that reads `free` is being told about a machine it cannot use. The
  local container is a 4-CPU cap with no memory limit (6 GiB for e2e), and both
  are documented as "a third machine". The contract is unchanged: no test may
  depend on timing. Expect a slower pod, not a different verdict.
- **Cold caches.** A fresh cluster has no Go module cache, no npm cache and no
  Playwright caches, so the first run on a new cluster fetches what CI's
  `actions/cache` would have restored, and a network wobble looks like a test
  failure until the log says otherwise. Later runs on the same cluster reuse the
  pod's own caches; `--down` throws them away.
- **No git common directory.** The frontend and e2e containers mount it because
  `go build` stamps VCS info and a worktree's `.git` is a file pointing outside
  the bind mount. The stage has no `.git` at all, so `go build` either skips the
  stamp or fails on it - the in-place modes must pass `-buildvcs=false` or give
  the pod the git dir, and the proposal is to do the former explicitly rather
  than let the stamp differ silently.
- **New failure modes.** A pod can be OOMKilled by the cluster's own limit (the
  local e2e container's 6 GiB cap becomes the pod's 8 GB, and an overshoot dies
  with no exit marker and no log tail until `sky logs` is read), a cluster can be
  lost mid-run, and the API can fail after the preflight. Each is exit 3 with the
  log's tail, never a pass - and none of them is a test result.
- **What a green `ci-full-sky` proves, and what it does not.** It proves the
  same commands passed on the same images on a machine sized for them. It does
  NOT prove CI's machine: the runner's core count, the Actions caches, the
  macOS job and the platform CI actually runs are all different. **CI remains the
  source of truth**, and `ci-full-sky` is a way to get a fast, honest answer
  without starving this machine - not a replacement for the push gate.

### 6. The wiring, and what it must not do

The in-place mode is small and it is what makes the wiring safe: no job
definition is duplicated, and the Docker path is the same code with one branch
that the SkyPilot invocation takes and nothing else does. The targets:

```make
# The container jobs of `ci-full`, on the owner's SkyPilot cluster (nocx-360js).
# Explicit and never a prerequisite of ci-full: the local Docker path is what CI
# is reproduced by, and it must keep working exactly as it does today.
ci-full-sky: ci-os-split ci-local-ssh-split ci ci-mac ci-backend-sky ci-linux-sky ci-frontend-sky
	@echo "=== the SkyPilot half of ci-full; ci-e2e-sky runs it too, on the e2e image ==="

ci-backend-sky:
	nocx-sky-test "$(CURDIR)" 'NOCX_CI_IN_PLACE=1 NOCX_LOCAL_SSH_PKGS=... NOCX_CI_TEST_P=$(GO_TEST_P) ./scripts/ci-linux.sh -- $$(make -s print-portable-pkgs)'
```

They must never be invoked by `pre-push`, by `pre-commit`, or from `ci-full`;
and `nocx-sky-test` itself must never fall back to a local run. All three hold
in what is on this machine today: the hooks call neither, `ci-full` is untouched
and `nocx-sky-test` exits 4 before staging anything when the API is unreachable.

### 7. Open questions for the owner

1. `ci-e2e-sky` is two pods (one per browser) or one pod running the projects in
   sequence - two pods is CI's shape, one pod is cheaper and slower.
2. Should `ci-full-sky` gate a push, or is a green CI run still what a push
   waits on?
3. Who may set `NOCX_ALLOW_LOCAL_TESTS=1` on a merged-tree gate - the
   coordinator alone, or any worker the owner names?
4. The pod is root. Recreate `ci-linux.sh`'s privilege drop on the pod, or take
   the root difference as one more thing `ci-full-sky` does not prove?

### What is wired today, and what is not

NOTHING in this proposal is wired - there is no `ci-full-sky`, no `*-sky`
target, and `scripts/ci-linux.sh`, `scripts/ci-frontend.sh` and
`e2e/run-in-container.sh` are untouched. The reason is the yardstick the bead
sets: wiring goes in only if it is small and safe ONCE PROPOSED, and the
in-place mode is small but not yet safe.

- It is small: about twelve lines in each runner, and the Docker path is the
  same code with one branch. That is the proposal's one good property and it is
  what makes it worth doing next.
- It is not safe yet, because each job needs a pod run before a target can be
  trusted: the pod is root where `ci-linux.sh` deliberately drops privilege,
  its caches are cold, its repo is writable where the container mounts `/src:ro`,
  and `go build` stamps VCS info against a stage that has NO `.git` at all. A
  gate wired in on a guess is a gate that reports green for the wrong machine,
  which is the failure this repository's testing rules are mostly about.

What IS usable today, with no wiring at all, is `nocx-sky-test` running a job's
own command:

```bash
nocx-sky-test "$PWD" 'go test -race -count=1 -p 2 -tags gtk3,nocx_framecheck ./internal/...'
nocx-sky-test "$PWD" 'cd frontend && npm ci && npm run typecheck && npm test'
nocx-sky-test --e2e "$PWD" 'bash e2e/container-entry.sh'
```

and that is enough for what the bead is for: a worker's heavy run leaves
vm-agents.
