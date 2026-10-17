package workers

// What removing a checkout answers (nocx-xn63t.1.5).
//
// Removal is an explicit call, separate from close by the owner's decision
// of 2026-09-18: closing a worker never touches its checkout, and a checkout
// that outlives its worker is removed by a second, deliberate act. This is
// that act's vocabulary — the ask (one or more checkouts, named by path or
// branch), and one answer per asked checkout that says what became of it.
//
// THE REFUSALS ARE NAMED, not a message: a coordinator that is told "no"
// has to decide what to do next, and the next step differs by refusal —
// commit or discard the work behind an `uncommitted`, close the worker
// behind a `held`, and accept that the thing asked about was never nocx's
// behind a `not-ours`. A checkout whose state nobody could read is
// `unresolved`, which is a different fact from clean and refuses for the
// same reason the git seam refuses one it could not read: unreadable is
// never an answer of safe.
//
// The branch is not part of any answer's fate: removal takes the checkout
// and always leaves the branch. Work no commit keeps — a tracked change, an
// untracked file — refuses the removal unless the caller says Discard on that
// checkout, and then the row that removed it names what went with it, counted
// (nocx-xn63t.6.40, ADR-0088). The discard is a CALLER'S WORD and never a
// default: nothing here decides for itself that somebody's work may be thrown
// away, and a call that did not ask cannot lose any.

// CheckoutRef names one checkout a caller asks to remove: by its path, by
// the branch checked out in it, or by both. A branch resolves to the one
// working tree of the repository that holds it; a path must be a working
// tree of the caller's repository. Both together must agree — a path whose
// checkout holds a different branch than the one named is refused, not
// guessed about.
type CheckoutRef struct {
	Path   string
	Branch string
	// Discard is the caller saying it wants the work no commit keeps gone
	// WITH this checkout: a tracked change, an untracked file, and whatever
	// ignored files the directory holds. It is false unless it was asked
	// for, and false is the whole of the old behaviour — a checkout holding
	// such work is refused `uncommitted` and left exactly as it was.
	//
	// It reaches nothing else. A checkout a live worker holds is still
	// refused `held`, anything that is not one of nocx's checkouts of the
	// caller's repository is still refused `not-ours`, and a state nobody
	// could read is still `unresolved` — the last one especially, because a
	// discarded directory cannot be handed back, and "unreadable" is the
	// one answer that is not an answer of safe. The branch always stays.
	Discard bool
}

// CheckoutRefusal is WHY one checkout was not removed. The set is closed:
// each member is a distinct fact a caller acts on differently, which is why
// they are separate names and not one "cannot".
type CheckoutRefusal string

const (
	// CheckoutRefusalUncommitted — the checkout holds work no commit keeps
	// (a tracked change or an untracked file; ignored files do not count).
	// Nothing was removed; the work is exactly where it was.
	CheckoutRefusalUncommitted CheckoutRefusal = "uncommitted"
	// CheckoutRefusalHeldByWorker — a live worker holds this checkout. It is
	// not left over, and offering it up as abandoned is the one mistake a
	// removal must never make; close the worker first.
	CheckoutRefusalHeldByWorker CheckoutRefusal = "held"
	// CheckoutRefusalNotOurs — the checkout is not one of nocx's checkouts
	// of the caller's repository: not a working tree of it at all, the
	// repository's main checkout, or a linked worktree a person made
	// outside nocx's worktrees directory. The owner's own worktrees are
	// never this act's to touch.
	CheckoutRefusalNotOurs CheckoutRefusal = "not-ours"
	// CheckoutRefusalUnresolved — nocx could not read what it needed in
	// order to decide, so nothing is claimed: not that the checkout is
	// dirty, and NOT that it is clean. A read that failed or a removal
	// whose outcome could not be established answers this, never a guess.
	CheckoutRefusalUnresolved CheckoutRefusal = "unresolved"
)

// RemovedCheckout is one asked checkout and what became of it. Path and
// Branch are the checkout as nocx resolved it on disk — a branch-named ask
// answers the path it resolved to, and the branch as git reported it — not
// merely the words of the ask; when nothing could be resolved they are the
// ask itself. Removed true means the checkout directory is gone and the
// branch is still there. Refusal is "" exactly when Removed is true, and
// Detail then says what is true on disk, because "what is on disk now is in
// the answer" is the property that lets a coordinator act on a partial
// removal instead of guessing about it.
type RemovedCheckout struct {
	Path    string
	Branch  string
	Removed bool
	// Discarded is the work that went with the checkout: what no commit
	// kept, counted. It is present exactly when the checkout was removed
	// AND something was actually thrown away — a removal that asked to
	// discard a clean checkout discarded nothing, and a refused row removed
	// nothing at all, so neither has anything to name.
	Discarded *DiscardedWork
	Refusal   CheckoutRefusal
	Detail    string
}

// DiscardedWork is what a removal took away that no commit keeps, counted the
// way git listed it: Modified is every status record that is not an untracked
// file (staged, unstaged, conflicted, rename and all), Untracked is the files
// git did not know about at all. Ignored files are not in either count —
// git's status does not list them — and they ride out with the directory
// either way.
//
// The counts are the read taken immediately before the removal, which is the
// only moment they can be taken: afterwards there is nothing left to count.
type DiscardedWork struct {
	Modified  int
	Untracked int
}

// CheckoutRemoval is the answer to one removal ask: one row per asked
// checkout, in the order asked. A removal that could not even establish the
// repository answers every row refused rather than a shorter list — a
// removal that said nothing about a checkout it was asked to remove would
// leave the caller counting rows to learn what happened.
type CheckoutRemoval struct {
	Items []RemovedCheckout
}
