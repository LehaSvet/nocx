package agenttyping_test

// The refusal is logged ONCE PER CHANGE OF REASON, not once per poll
// (nocx-xn63t.6.52).
//
// A coordinator's pane is read once a second by the pane sweep
// (transport.DefaultPaneObserverSweep), and every reading that finds a wake
// still owed re-attempts it. When the attempt is refused — a box that is not
// empty, a menu, the pane's own state — the refusal is logged. Before this,
// that was one WARN per second for as long as the situation stood: measured on
// the owner's stand of 2026-10-18, about sixty-five identical lines while a
// single worker's mail stayed unread.
//
// A repeating alarm is how a person learns to skip the one line that mattered.
// What is worth a line is the change: the same pane refusing for the same
// reason again is the same fact, and a different reason is news.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/shady2k/nocx/internal/agenttyping"
	"github.com/shady2k/nocx/internal/log"
	"github.com/shady2k/nocx/internal/log/logtest"
	"github.com/shady2k/nocx/internal/paneview"
)

// refusalWarns counts the lines the typist wrote about NOT typing.
//
// It counts the MESSAGE and the level, because the count is the whole subject:
// a test that matched a substring could be satisfied by any other line the
// package grows.
func refusalWarns(t *testing.T, sl *slog.Logger) int {
	t.Helper()
	n := 0
	for _, r := range logtest.RecordsSlog(sl) {
		if r.Level >= slog.LevelWarn && r.Message == "nocx refused to type into a pane" {
			n++
		}
	}
	return n
}

// typistLogging builds the production wiring over a logger this test can read.
func typistLogging(t *testing.T, f paneview.Frame, sl *slog.Logger) (*agenttyping.Typist, *screens, *queue) {
	t.Helper()
	sc := &screens{frame: f}
	q := &queue{}
	ty := agenttyping.New(log.NewSlogAdapter(sl), sc, rulesOf(t), verifiedFor(t),
		enrolled{agent: map[string]string{pane: agent}}, q)
	return ty, sc, q
}

// A poll that refuses for the SAME reason is one line, however many times it
// happens; a poll that refuses for a DIFFERENT reason is a second line, and
// going back to the first reason is a third.
//
// The reasons here are the two the gate actually produces on the owner's stand:
// the box holding a text nocx did not put there (the shape of a person's draft
// AND of a drawing nocx cannot tell from one), and a pane showing a menu.
// The whole situation is driven through Submit rather than through an
// unexported seam, so what is counted is the line a person's log would hold.
func TestARefusalIsWarnedOncePerChangeOfReason(t *testing.T) {
	sl := logtest.Slog(t)
	draft := replay(t, "claude-2.1.266-turn", 47000)
	menu := replay(t, "claude-2.1.266-permission", 49000)

	ty, sc, q := typistLogging(t, draft, sl)

	// THE SWEEP: one attempt per second, and the same answer every time.
	for i := 0; i < 5; i++ {
		got := ty.Submit(context.Background(), pane, "wake up")
		if got.Outcome != agenttyping.OutcomeRefused {
			t.Fatalf("attempt %d: outcome = %q, want a refusal — this test needs the refusal it counts", i, got.Outcome)
		}
	}
	if got := refusalWarns(t, sl); got != 1 {
		t.Fatalf("%d refusals were warned about an unchanged reason, want 1 — a line per poll is an alarm clock, not a report", got)
	}

	// A DIFFERENT reason is news.
	sc.frame = menu
	if got := ty.Submit(context.Background(), pane, "wake up").Outcome; got != agenttyping.OutcomeRefused {
		t.Fatalf("outcome with a menu up = %q, want a refusal", got)
	}
	if got := refusalWarns(t, sl); got != 2 {
		t.Fatalf("%d lines after the reason changed, want 2", got)
	}
	// ...and not news twice: the menu stands, and the sweep keeps polling.
	for i := 0; i < 3; i++ {
		ty.Submit(context.Background(), pane, "wake up")
	}
	if got := refusalWarns(t, sl); got != 2 {
		t.Fatalf("%d lines while the SAME menu stood, want 2", got)
	}

	// Back to the first reason: that is a change again.
	sc.frame = draft
	ty.Submit(context.Background(), pane, "wake up")
	if got := refusalWarns(t, sl); got != 3 {
		t.Fatalf("%d lines after the reason changed back, want 3", got)
	}

	if len(q.jobs) != 0 {
		t.Fatalf("%d bytes reached the pane across %d refused attempts", len(q.jobs), 9)
	}
}

// A WRITE THAT WENT THROUGH ENDS THE EPISODE, so the same refusal after it is
// news again.
//
// Without this half a later stall would be silent for the rest of the process's
// life: the reason text is the same, so a memory that only ever compared
// strings would swallow it. The clearing is on the paste the pane ACCEPTED,
// which is the moment the refusal stopped being true.
func TestASuccessfulWriteMakesTheSameRefusalNewsAgain(t *testing.T) {
	sl := logtest.Slog(t)
	draft := replay(t, "claude-2.1.266-turn", 47000)
	cleared := replay(t, "claude-2.1.266-turn", 84000)

	ty, sc, q := typistLogging(t, draft, sl)
	if got := ty.Submit(context.Background(), pane, "wake up").Outcome; got != agenttyping.OutcomeRefused {
		t.Fatalf("outcome with the draft in the box = %q, want a refusal", got)
	}
	if got := refusalWarns(t, sl); got != 1 {
		t.Fatalf("%d lines, want 1", got)
	}

	sc.frame = cleared
	if got := ty.Submit(context.Background(), pane, "wake up").Outcome; got != agenttyping.OutcomeSubmitted {
		t.Fatalf("outcome on the emptied box = %q, want the submission this test needs to have happened", got)
	}

	sc.frame = draft
	if got := ty.Submit(context.Background(), pane, "wake up").Outcome; got != agenttyping.OutcomeRefused {
		t.Fatalf("outcome with the draft back = %q, want a refusal", got)
	}
	if got := refusalWarns(t, sl); got != 2 {
		t.Fatalf("%d lines after a stall that follows a delivered wake, want 2 — the episode ended when the write went through", got)
	}
	// And the second stall is still one line, not a resumed flood.
	ty.Submit(context.Background(), pane, "wake up")
	if got := refusalWarns(t, sl); got != 2 {
		t.Fatalf("%d lines, want 2", got)
	}
	if len(q.jobs) != 2 {
		t.Fatalf("%d writes reached the pane, want the one submission the middle attempt made: %q", len(q.jobs), q.all())
	}
}

// TWO PANES DO NOT SILENCE EACH OTHER. The memory is per pane, because the
// same sentence from two panes is two situations and a person reading the log
// has to be able to tell which one stopped.
func TestTheSameReasonInTwoPanesIsTwoLines(t *testing.T) {
	sl := logtest.Slog(t)
	draft := replay(t, "claude-2.1.266-turn", 47000)
	other := "fedcba9876543210fedcba9876543210"

	ty, _, q := typistOnLogged(t, draft, sl, other)
	const first = "0123456789abcdef0123456789abcdef"
	if got := ty.Submit(context.Background(), first, "wake up").Outcome; got != agenttyping.OutcomeRefused {
		t.Fatalf("outcome = %q, want a refusal", got)
	}
	if got := ty.Submit(context.Background(), other, "wake up").Outcome; got != agenttyping.OutcomeRefused {
		t.Fatalf("outcome = %q, want a refusal", got)
	}
	if got := refusalWarns(t, sl); got != 2 {
		t.Fatalf("%d lines for two panes refusing for the same reason, want 2", got)
	}
	if len(q.jobs) != 0 {
		t.Fatalf("%d bytes reached the panes: %q", len(q.jobs), q.all())
	}
}

// typistOnLogged is typistLogging with more than one enrolled pane.
func typistOnLogged(t *testing.T, f paneview.Frame, sl *slog.Logger, also ...string) (*agenttyping.Typist, *screens, *queue) {
	t.Helper()
	sc := &screens{frame: f}
	q := &queue{}
	agents := map[string]string{pane: agent}
	for _, id := range also {
		agents[id] = agent
	}
	ty := agenttyping.New(log.NewSlogAdapter(sl), sc, rulesOf(t), verifiedFor(t), enrolled{agent: agents}, q)
	return ty, sc, q
}
