package capability

import (
	"context"
	"errors"

	"github.com/shady2k/nocx/internal/git"
	"github.com/shady2k/nocx/internal/git/registry"
	"github.com/shady2k/nocx/internal/session"
	"github.com/shady2k/nocx/internal/transport/control"
)

// GitOpenFactory resolves the repo factory git.open uses for a resolved
// session — the composition root's answer to "which git runs for this
// session" (AD-8; the files domain's ProviderFactory is the same shape).
// Local sessions resolve to the local factory; SSH sessions resolve to the
// helper-backed factory when one is available, and to nil when the
// selection refuses the session (remote-helper design §6). It must be
// side-effect-free: git.open consults it twice (the handler's refusal
// decision, then the open), and the two calls must agree.
type GitOpenFactory func(sess session.Session) git.RepoFactory

// GitOpenService is the git.open surface: resolve the session, open the
// repository through the factory and register the binding, and hold the
// binding surface for the inline first status. It is what a
// GitOpenOperation hands its callback.
type GitOpenService interface {
	// Get resolves the session the connection owns (D15) — the handler
	// decides noCwd and the §6 refusal states from its Kind before
	// opening.
	Get(id session.ID) (session.Session, error)
	// OpenBinding opens the repository at cwd through the wired factory
	// and registers it for the session, returning the minted binding id
	// and the open outcome. It owns the ownership-transfer rule (spec
	// §5.1): a live repo on a refusing outcome is closed before the
	// refusal is returned, and a Register failure closes the repo.
	OpenBinding(ctx context.Context, sess session.Session, cwd string) (bindingID string, outcome git.OpenOutcome, err error)
	// Acquire takes the use-guard for one binding — the same per-call
	// authorisation every git.* method applies (D15).
	Acquire(id string, caller registry.Caller) (registry.Handle, func(), error)
	// Close closes one binding — the opener's own cleanup when the session
	// it opened for closed while the open was in flight.
	Close(id string) error
}

// GitOpenOperation is the typed operation for git.open, and it is two-phase
// for the reason OpenOperation is (nocx-l21ib.1, nocx-zg3k3.14.17): Resolve
// runs under the session gate alone, Open under the git gate and the
// execution lane. On an ssh session the open is a helper round trip — a
// lease, a uname, the open, the first status — and the session gate is
// capacity one for every session request of every pane. Held across that, a
// reloaded window's few ssh panes filled its wait queue and every resize,
// sessions.live and shell request on the server was refused "Control plane
// busy" for as long as the opens took. Resolving is a registry read; it is
// the only part the session gate has any business excluding.
//
// What the gate used to buy, and what replaces it: under it an open could
// not interleave with a session close, so it never registered a binding for
// a session already torn down. Now one can, and the handler closes it: the
// registry forgets a session before its bindings are swept, so an opener
// that finds its session gone after recording the binding is the one that
// must close it, and one that finds it present can leave it to the sweep.
type GitOpenOperation interface {
	AssistantOperation
	// Resolve runs fn under the session gate and nothing else.
	Resolve(context.Context, func(context.Context, GitOpenService) error) error
	// Open runs fn under [git] and the execution lane, no session gate held.
	Open(context.Context, func(context.Context, GitOpenService) error) error
}

// NewGitOpenOperation builds the git.open operation. Its phases acquire the
// session gate, then — after releasing it — the git gate and the execution
// lane, so the canonical order holds and no phase waits on a domain gate
// while holding the lane.
func NewGitOpenOperation(
	sessionGate, gitGate, lane control.Admission,
	registry session.Registry,
	factory GitOpenFactory,
	reg *registry.Registry,
) GitOpenOperation {
	g := &guard{}
	svc := newGitOpenService(g, registry, factory, reg)
	disposition := Adapted("git.open", "opening a repository resolves a session and selects its helper or local binding")
	return &gitOpenOperation{
		resolve: newOperation[GitOpenService](disposition, control.NewComposite(sessionGate), g, svc),
		open:    newOperation[GitOpenService](disposition, control.NewComposite(gitGate, lane), g, svc),
	}
}

// gitOpenOperation is the two admissions over ONE guard and ONE service, as
// openOperation is: a handle captured in one phase is refused outside every
// in-flight phase.
type gitOpenOperation struct {
	resolve, open *operation[GitOpenService]
}

func (o *gitOpenOperation) Disposition() Disposition {
	return o.open.Disposition()
}

func (o *gitOpenOperation) Resolve(ctx context.Context, fn func(context.Context, GitOpenService) error) error {
	return o.resolve.Run(ctx, fn)
}

func (o *gitOpenOperation) Open(ctx context.Context, fn func(context.Context, GitOpenService) error) error {
	return o.open.Run(ctx, fn)
}

// newGitOpenService builds the concrete git.open service bound to guard g.
func newGitOpenService(g *guard, registry session.Registry, factory GitOpenFactory, reg *registry.Registry) *gitOpenService {
	return &gitOpenService{guard: g, registry: registry, factory: factory, reg: reg}
}

type gitOpenService struct {
	guard    *guard
	registry session.Registry
	factory  GitOpenFactory
	reg      *registry.Registry
}

func (s *gitOpenService) Get(id session.ID) (session.Session, error) {
	if err := s.guard.check(); err != nil {
		return nil, err
	}
	return s.registry.Get(id)
}

func (s *gitOpenService) OpenBinding(ctx context.Context, sess session.Session, cwd string) (string, git.OpenOutcome, error) {
	if err := s.guard.check(); err != nil {
		return "", git.OpenOutcome{}, err
	}
	f := s.factory(sess)
	if f == nil {
		// No factory for this session. The transport answers the §6
		// refusal states and the not-available error from the session's
		// origin before opening, so this is the service's honest answer
		// if the two ever disagree.
		return "", git.OpenOutcome{}, errNoFactoryForSession
	}
	repo, outcome, err := f.Open(ctx, cwd)
	if err != nil {
		return "", git.OpenOutcome{}, err
	}
	if outcome.State != git.OpenOK {
		if repo != nil {
			// Refusing outcome with a live repo: the repo is still ours,
			// and it must not leak.
			if cerr := repo.Close(); cerr != nil {
				return "", git.OpenOutcome{}, &openOutcomeCloseError{err: cerr}
			}
		}
		return "", outcome, nil
	}
	if repo == nil {
		// The other direction of the same lie: ok with no repository.
		return "", git.OpenOutcome{}, &openOutcomeNilRepoError{}
	}
	bid, err := s.reg.Register(repo, sess.ID())
	if err != nil {
		// Always a typed error, close outcome or not: the transport's
		// git.open answers every register failure with the "git.open:"
		// prefix, and the raw registry error alone (close succeeded) must
		// not lose that classification.
		re := &openRegisterError{err: err}
		if cerr := repo.Close(); cerr != nil {
			re.closeErr = cerr
		}
		return "", git.OpenOutcome{}, re
	}
	return bid, outcome, nil
}

func (s *gitOpenService) Acquire(id string, caller registry.Caller) (registry.Handle, func(), error) {
	if err := s.guard.check(); err != nil {
		return nil, nil, err
	}
	return s.reg.Acquire(id, caller)
}

func (s *gitOpenService) Close(id string) error {
	if err := s.guard.check(); err != nil {
		return err
	}
	return s.reg.Close(id)
}

// GitBindingService is the per-binding git surface: every git.* method
// after git.open. A binding id is validated per call by Acquire — bindings
// close at any moment, so a construction-time check would be a lie.
type GitBindingService interface {
	Acquire(id string, caller registry.Caller) (registry.Handle, func(), error)
	Close(id string) error
	CloseSession(sessionID session.ID)
}

// GitBindingOperation is the typed operation for the git-binding domain.
// Its gate is [git].
type GitBindingOperation interface {
	AssistantOperation
	Run(context.Context, func(context.Context, GitBindingService) error) error
}

// NewGitBindingOperation builds the git-binding operation, acquiring the
// git gate before the execution lane.
func NewGitBindingOperation(gitGate, lane control.Admission, reg *registry.Registry) GitBindingOperation {
	g := &guard{}
	return newOperation[GitBindingService](Adapted("git.refresh", "git calls require an assistant-owned repository lease"), control.NewComposite(gitGate, lane), g, newGitBindingService(g, reg))
}

// newGitBindingService builds the concrete git-binding service bound to
// guard g.
func newGitBindingService(g *guard, reg *registry.Registry) *gitBindingService {
	return &gitBindingService{guard: g, reg: reg}
}

type gitBindingService struct {
	guard *guard
	reg   *registry.Registry
}

func (s *gitBindingService) Acquire(id string, caller registry.Caller) (registry.Handle, func(), error) {
	if err := s.guard.check(); err != nil {
		return nil, nil, err
	}
	return s.reg.Acquire(id, caller)
}

func (s *gitBindingService) Close(id string) error {
	if err := s.guard.check(); err != nil {
		return err
	}
	return s.reg.Close(id)
}

func (s *gitBindingService) CloseSession(sessionID session.ID) {
	if !s.guard.ok() {
		return
	}
	s.reg.CloseSession(sessionID)
}

// errNoFactoryForSession reports a git.open that reached the factory
// selector with no factory for the session. The transport answers the §6
// refusal states and the not-available error from the session's origin
// before opening, so this is the service's honest answer if the two ever
// disagree.
var errNoFactoryForSession = errors.New("git.open: no repo factory for this session")

// openOutcomeCloseError reports a refusing git.open outcome whose live repo
// could not be closed. The repo must not leak, and a failed close is a fact
// the operator should see, not a silently dropped error.
type openOutcomeCloseError struct{ err error }

func (e *openOutcomeCloseError) Error() string {
	return "git.open: close repo after refusing outcome: " + e.err.Error()
}
func (e *openOutcomeCloseError) Unwrap() error { return e.err }

// openOutcomeNilRepoError reports a factory that answered ok without a
// repository.
type openOutcomeNilRepoError struct{}

func (e *openOutcomeNilRepoError) Error() string {
	return "git.open: factory answered ok without a repository"
}

// openRegisterError reports a Register failure. closeErr is set only when
// the owned repo could also not be closed — a live repo must not leak, and a
// failed close is a fact the operator should see, not a silently dropped
// error. The "git.open:" prefix is the transport's wire vocabulary for the
// whole open error class.
type openRegisterError struct {
	err      error
	closeErr error
}

func (e *openRegisterError) Error() string {
	if e.closeErr != nil {
		return "git.open: register: " + e.err.Error() + "; close repo: " + e.closeErr.Error()
	}
	return "git.open: register: " + e.err.Error()
}
func (e *openRegisterError) Unwrap() error { return e.err }
