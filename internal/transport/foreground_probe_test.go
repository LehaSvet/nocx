package transport

import (
	"fmt"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/shady2k/nocx/internal/helper/client"
	"github.com/shady2k/nocx/internal/helper/proto"
	"github.com/shady2k/nocx/internal/log"
	"github.com/shady2k/nocx/internal/log/logtest"
	"github.com/shady2k/nocx/internal/session"
)

// THE PROBE'S ERROR IS NOT PROOF (nocx-01um8).
//
// groupGone polls one process group with `sig == 0` and used to return true on
// ANY error — so every error that is not the guard read as "the execution is
// gone", which is the one sentence this ladder exists not to say without
// proof.
//
// Over a HELPER-hosted session the probe cannot succeed at ALL. kill(2) is not
// what carries it: the helper's signal op carries it, and the helper REFUSES a
// non-positive signal by construction —
//
//	if p.Signal <= 0 || p.Signal > maxSignal {
//		return fmt.Errorf("%w: %d is outside 1..%d", ErrSignal, p.Signal, maxSignal)
//	}
//
// (internal/helper/session/service.go, maxSignal = 64) — so the FIRST poll
// answered "gone", stopProcessGroup returned delivered after its FIRST rung,
// and INT → TERM → KILL was never walked. Every LOCAL pane is such a session
// (ADR-0057): its helper observes the host's own foreground group, the
// transport names it, and the ladder is what a Stop takes.
//
// helperHostedSession is the seam that produced this, modelled refusal and
// all: a non-positive signal never reaches the group, a signal to a group that
// is gone comes back as the same refusal a failed kill gives, and what the
// transport sees is a *client.RefusalError with no unwrap to pty.ErrNoForeground
// — the helper erases the sentinel on the way out
// (`fmt.Errorf("%w: %v", ErrSignal, err)`, and `%v` is the erasing half). So
// "the group is gone" and "the helper would not take that signal" are TWO
// FACTS WEARING ONE ANSWER over this transport, and neither of them is proof.
type helperHostedSession struct {
	mu sync.Mutex
	// live is every group the helper's kernel still has.
	live map[int]bool
	// group is what the helper's observation names in front.
	group int
	// diesOn is the one rung the command obeys; 0 is a command that ignores
	// every rung.
	diesOn syscall.Signal
	// got is EVERY call the ladder made, probes included, in order.
	got []signalledGroup
}

func newHelperHostedSession(group int) *helperHostedSession {
	return &helperHostedSession{live: map[int]bool{group: true}, group: group}
}

func (h *helperHostedSession) ForegroundJob() (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.group, nil
}

func (h *helperHostedSession) SignalForeground(syscall.Signal) error {
	panic("a stop names its addressee; it never signals whatever is in front")
}

func (h *helperHostedSession) SignalProcessGroup(pgid int, sig syscall.Signal) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.got = append(h.got, signalledGroup{pgid: pgid, sig: sig})
	if sig <= 0 {
		return &client.RefusalError{
			Code:    proto.ErrCodeBadParams,
			Message: fmt.Sprintf("session: signal is invalid or unavailable: %d is outside 1..64", sig),
		}
	}
	if !h.live[pgid] {
		return &client.RefusalError{
			Code:    proto.ErrCodeBadParams,
			Message: "session: signal is invalid or unavailable: pty: no foreground process to signal",
		}
	}
	if h.diesOn != 0 && sig == h.diesOn {
		delete(h.live, pgid)
	}
	return nil
}

// rungs is every SIGNAL the ladder tried, in order, refusal or not. A rung
// that FAILED is never delivery (this package's own rule) and it is still
// evidence the ladder moved.
func (h *helperHostedSession) rungs() []syscall.Signal {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []syscall.Signal
	for _, s := range h.got {
		if s.sig != 0 {
			out = append(out, s.sig)
		}
	}
	return out
}

// probes counts the existence polls the ladder made.
func (h *helperHostedSession) probes() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, s := range h.got {
		if s.sig == 0 {
			n++
		}
	}
	return n
}

// A PROBE ERROR KEEPS THE LADDER ESCALATING (nocx-01um8).
//
// The command takes its SIGINT and goes, and the probe cannot say so — the
// ordinary case on every local and ssh pane. The ladder must therefore walk
// the rungs that are left rather than claim after the first one that the
// execution is gone: a Stop that reports delivered over a live command is the
// defect this ladder exists to prevent, and one rung is not evidence when the
// probe that was supposed to supply the evidence refused to answer.
//
// Disproved by a Stop that sends one rung and returns delivered, which is what
// this asserted before the fix.
func TestStopForeground_AProbeErrorIsNotProofTheGroupIsGone(t *testing.T) {
	const group = 5100
	ctx, _ := logtest.New(t)
	sess := newHelperHostedSession(group)
	sess.diesOn = syscall.SIGINT

	outcome := stopForeground(log.From(ctx), session.ID("sid"), sess, 20*time.Millisecond, nil)

	want := []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL}
	got := sess.rungs()
	if len(got) != len(want) {
		t.Fatalf("the ladder sent %v, want %v: a probe that could not answer is not proof the command ended", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the ladder sent %v, want %v", got, want)
		}
	}
	// The SIGINT did reach the group, so the ladder's promise is kept — it is
	// the RUNG COUNT that says whether it was kept on evidence or on a guess.
	if outcome != foregroundDelivered {
		t.Fatalf("outcome = %q, want %q", outcome, foregroundDelivered)
	}
	if sess.probes() == 0 {
		t.Fatalf("the ladder never polled the group it addressed")
	}
	// AND IT SAYS SO, under the request's own trace: the per-test failure
	// context filters the backend log by trace, and a Stop that took two extra
	// graces with no line explaining why is a Stop nobody can diagnose
	// (nocx-zg3k3.14.16).
	if !logtest.WaitFor(ctx, time.Second, func(r logtest.Record) bool {
		return r.Message == "foreground signal: the probe could not say whether the execution's group is gone"
	}) {
		t.Fatalf("the ladder did not log that its probe could not answer")
	}
}

// The other end of the same rule: a probe that ANSWERS must still end the Stop
// where it stands. A fix that made the probe's answer irrelevant would walk the
// whole ladder over a command that already went, which is the escalation onto
// somebody else's process this package spent two beads removing.
func TestStopForeground_AProbeThatAnswersGoneEndsTheLadderWhereItStands(t *testing.T) {
	const group = 5200
	sess := newHandoffSession(group)
	// The command cooperates, its group goes, and the probe sees that: the
	// kernel keeps answering for the group until it exits.
	sess.diesOn = syscall.SIGINT

	outcome := stopForeground(log.NewSlogAdapter(nil), session.ID("sid"), sess, 200*time.Millisecond, nil)

	if outcome != foregroundDelivered {
		t.Fatalf("outcome = %q, want %q", outcome, foregroundDelivered)
	}
	if got := sess.signalsTo(group); len(got) != 1 || got[0] != syscall.SIGINT {
		t.Fatalf("the ladder sent %v, want exactly [SIGINT]: a probe that proved the group gone leaves nothing to escalate to", got)
	}
}

// And the rung that FAILED is never turned into a promise: when the group is
// already gone over the helper, every rung is refused with the same
// bad_params refusal the probe gets, and none of them is the guard. The honest
// answer is that nocx could not prove it stopped anything — never
// nothing-running, which is the sentence shown beside a block that is still
// running (nocx-7l4ex.12).
func TestStopForeground_EveryRungRefusedIsNotACommandStopped(t *testing.T) {
	const group = 5300
	sess := newHelperHostedSession(group)
	sess.live = map[int]bool{} // the group is gone: the helper refuses every rung

	outcome := stopForeground(log.NewSlogAdapter(nil), session.ID("sid"), sess, 20*time.Millisecond, nil)

	if outcome != foregroundUnreconciled {
		t.Fatalf("outcome = %q, want %q: no rung was delivered and none of them was the guard", outcome, foregroundUnreconciled)
	}
	want := []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL}
	if got := sess.rungs(); len(got) != len(want) {
		t.Fatalf("the ladder sent %v, want %v", got, want)
	}
}
