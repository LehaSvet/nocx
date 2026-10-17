# ADR-0088 — A leftover checkout's work goes with it only when the caller says discard

- **Status:** Accepted
- **Date:** 2026-10-17
- **Beads:** `nocx-xn63t.6.40` (the owner's decision of 2026-10-17); relates to `nocx-xn63t.1.5`, which set the refusal this changes, and `nocx-xn63t.1.6`, whose sweep keeps it.
- **Scope:** the explicit checkout removal — the `workers.removeCheckout` tool, its params and result contracts, `internal/app`'s checkouts service, and the git seam's `RemoveWorktree`. No change to `workers.close`, to the sweep's judgement, or to any effect class.

## Context

Removal is an explicit, separate act from closing a worker (owner's decision, 2026-09-18, `nocx-xn63t.1.5`). That act refuses any checkout holding work no commit keeps, and the sentence it shipped with is "There is no override: a person who wants the work gone has a shell."

The owner's working day of 2026-10-17 measured what that sentence costs. A coordinator removed a leftover checkout and git refused it: two untracked files an unrelated tool (`.repowise`) had written into the directory. The coordinator's only route was to delete those files by hand in a shell and ask again — a second, unaccounted-for mutation of a directory nocx owns, performed by the agent the refusal was protecting the work from. `herdr`, the tool being replaced, removes a worktree together with its files.

The refusal is right about the default and wrong about the door. What it must keep is that nothing throws a person's work away unless somebody who is answerable said so, by name, about that checkout. What it must stop doing is making "there is no door" out of "there is no accidental door".

## Decision

`workers.removeCheckout` takes an optional `discard` flag **per checkout**, default false. With it, a nocx-owned leftover checkout is removed even though it holds modified, untracked and ignored files; without it, nothing changes at all.

1. **Per checkout, not per call.** The ask is already one row per checkout and the refusals are already per checkout, so a coordinator that wants one leftover gone with its scratch files and another merely gone makes one call. A per-call flag would put a destructive word over a list a caller may not have re-read.
2. **The flag moves exactly one refusal.** `uncommitted` is the only answer it changes. `held` (a live worker holds it), `not-ours` (the main checkout, a person's own linked worktree, a path that is no working tree, a branch nothing holds) and `unresolved` (a read that failed, or a state nobody could read) are checked before the removal is ever reached, in the order the walk already used, and answer the same with the flag as without it. `unresolved` in particular: a discarded directory cannot be handed back, so "I could not read this" must never be converted into "I removed it".
3. **The branch still always stays.** Nothing here deletes a branch, so committed work survives every row, discarded or not.
4. **The answer says what went, counted.** A removed row carries `discarded: {modified, untracked}` — the work no commit kept, split the way the bead names it — and carries it only when there was something to discard. A clean removal and every refusal carry nothing, and the result schema forbids a refused row from claiming it. The counts come from the same status read the refusal is taken on, made immediately before the removal, so the refusal's own account of "2 uncommitted change(s)" and the counts are one reading of one disk.
5. **The seam takes it as an ask, never as a default.** `git.Repo.RemoveWorktree` gains `RemoveWorktreeOptions{Discard bool}` and answers `git.WorktreeDiscard`; its zero value is the old behaviour exactly. It passes git's own `--force` only when the ask says so, which keeps git's refusals — including a locked worktree, which one `--force` does not override — as git's to give.
6. **The effect class does not change.** Removing a checkout was already `mutate-destructive` (ADR-0053), the coordinator's grant already permits that class, and the admitted tool set is byte-for-byte what it was: the flag changes _which work_ this call may take, never the authority to take a checkout. The person's control over it stays where it was, in the class.
7. **The sweep never discards.** `internal/app/worker_checkout_sweep.go` builds its refs without the flag and still refuses a dirty checkout, naming it in the holdings answer. A schedule is nobody looking, and the one thing it must never do is take work away that no person was asked about. The explicit ask is a coordinator looking at a repository it named.

## Alternatives rejected

- **One `--force`-style parameter on `RemoveWorktree` with no per-checkout flag** (a call-wide discard). It is fewer moving parts, and it makes one word destructive over a list: the checkout a caller did not re-read loses its work because a sibling did.
- **A second tool** (`workers.forceRemoveCheckout`). It doubles the surface the model is offered, its schema, its catalogue entry and its admission pins, for one boolean — and the refusal that makes the first tool safe would have to be restated in the second. One tool with one flag keeps one owner of "which checkouts are nocx's".
- **Discarding when the checkout is ignored-looking, empty-looking or unreferenced by any live worker** — an implicit force. Every one of those is a proxy for "the caller probably does not mind", and the case this bead exists for (two untracked files from an unrelated tool) is exactly a case a proxy would misjudge in the other direction. The refusals stay explicit.
- **Keeping the shell as the only route.** That is today's behaviour and the measured defect: it drives a destructive hand-edit of a directory nocx owns, outside the record, the log and the answer, which is the one place the work cannot be counted afterwards.
- **Deleting the files and retrying inside the removal** (discard untracked files, then ask git again). The removal would then be two acts with a failure between them, and the row could not say what it took.
- **Splitting the counts from the retained status lists.** The lists stop at the retention cap while the totals do not, so a split taken from them would report an untracked file as a modified one on a checkout with more than `MaxStatusEntries` records. The untracked count is therefore made at the stream, beside `Total`.
- **A `git worktree remove -f -f`** (the lock override). A lock is a person's deliberate marker on a worktree; passing it here would convert this flag into a key to somebody's lock as a side effect of wanting scratch files gone.

## Consequences

A coordinator that means it says `discard: true` on the checkout it means, and reads back the counts; one that does not gets the refusal it always got, and the work is exactly where it was. The tool's description, the params schema and the result schema changed together, and the result schema is enforced at dispatch (`validateToolResult`), so a row cannot claim discarded work on a refusal. No generated artifact derives from `contracts/tools/*` — the OpenRPC manifest and the renderer's generated types cover the JSON-RPC surface, not the agent tools — so nothing regenerates and `npm run contracts:check` is unaffected.

What is inherited: the discard is the caller's word and there is still no ambient one; a live worker's checkout and anything that is not nocx's remain out of this act's reach; a state nobody could read is still a refusal, because discard is irreversible; the branch still survives; the counts describe the read taken before the removal; and the sweep is still the caller that never discards.
