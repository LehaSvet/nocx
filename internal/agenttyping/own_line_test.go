package agenttyping_test

// A LINE NOCX ITSELF LEFT IN THE BOX MUST NOT BLOCK THE NEXT WAKE
// (nocx-xn63t.6.53).
//
// A wake pastes its line and the SUBMIT KEY is then refused — the screen moved
// between the paste's own frame and the key's, so the typist answers
// OutcomeTyped and sends nothing more. What it leaves behind is its own line
// standing in the coordinator's input box, and the box gate nocx-xn63t.6.44
// added — a wake never types over a person's draft — then refuses every later
// wake for as long as the line stands. Nothing ends it: a Claude suggestion
// disappears when the agent draws the next prompt, but nocx's own bytes stand
// until something types or clears them.
//
// So the wake owns what it wrote. When the box holds EXACTLY the bytes nocx
// pasted and could not submit, the next grant is admitted and what is pressed
// is the one key that was refused — the submit key — rather than a second line
// pasted on top of the first. The person's protection is unchanged and is
// asserted beside it: a box holding anything else, their own sentence or their
// sentence with nocx's line in front of it, is still refused and nothing is
// written.

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/agentdriver"
	"github.com/shady2k/nocx/internal/agenttyping"
	"github.com/shady2k/nocx/internal/log"
	"github.com/shady2k/nocx/internal/log/logtest"
)

// ownLineStand is one pane wired the way the composition root wires it, with a
// screen this test can move between the reads one submission makes.
type ownLineStand struct {
	ty  *agenttyping.Typist
	sc  *screens
	q   *queue
	own string
}

// newOwnLineStand builds the production wiring over a corpus frame whose box is
// EMPTY, and reads the text a DIFFERENT corpus frame holds in the same box.
//
// That reading is what "nocx's own line" is for every test below, and it is a
// real one rather than a sentence written here: which bytes they are does not
// matter to the mechanism — it compares what it pasted with what the rule reads
// back — and a capture of nocx's own wake line cannot exist, because nocx is
// the one typing it.
func newOwnLineStand(t *testing.T, sl *slog.Logger) *ownLineStand {
	t.Helper()
	draft := replay(t, "claude-2.1.266-turn", 47000)
	own, ok := rulesOf(t).Observe(agent, draft).InputText()
	if !ok || own == "" {
		t.Fatalf("the corpus frame must hold a text in the box; got (%q, %v)", own, ok)
	}
	cleared := replay(t, "claude-2.1.266-turn", 84000)
	if text, ok := rulesOf(t).Observe(agent, cleared).InputText(); !ok || text != "" {
		t.Fatalf("the frame this stand starts on must be an empty box; got (%q, %v)", text, ok)
	}
	sc := &screens{frame: cleared}
	q := &queue{}
	ty := agenttyping.New(log.NewSlogAdapter(sl), sc, rulesOf(t), verifiedFor(t),
		enrolled{agent: map[string]string{pane: agent}}, q)
	return &ownLineStand{ty: ty, sc: sc, q: q, own: own}
}

// leaveOurOwnLine drives the ONE partial state a wake can reach: the paste is
// accepted and the submit key is refused because the screen moved under it. It
// leaves the stand on the frame that box reading came from.
func (s *ownLineStand) leaveOurOwnLine(t *testing.T) agenttyping.Result {
	t.Helper()
	// Read 1 mints the permit and read 2 admits the paste, both on the empty
	// box; read 3 gates the submit key, and the dialog is up by then.
	s.sc.changeAfter(2, replay(t, "claude-2.1.266-permission", 49000))
	got := s.ty.Submit(context.Background(), pane, s.own)
	if got.Outcome != agenttyping.OutcomeTyped {
		t.Fatalf("outcome = %q (%s), want %q: this test needs the text landed and the key refused",
			got.Outcome, got.Reason, agenttyping.OutcomeTyped)
	}
	if len(s.q.jobs) != 1 {
		t.Fatalf("%d writes reached the pane, want the paste alone: %q", len(s.q.jobs), s.q.all())
	}
	// The pane is waiting for input again and the box holds the line nocx
	// pasted: the state the defect leaves, and nothing else in it.
	s.sc.frame = replay(t, "claude-2.1.266-turn", 47000)
	if box := s.box(t); box != s.own {
		t.Fatalf("the box reads %q, and this test needs it to hold nocx's own line %q", box, s.own)
	}
	return got
}

// box is what the shipped rule reads in the box of the stand's CURRENT frame,
// read off the frame already in hand so the submission's own read count is
// untouched.
func (s *ownLineStand) box(t *testing.T) string {
	t.Helper()
	text, ok := rulesOf(t).Observe(agent, s.sc.frame).InputText()
	if !ok {
		t.Fatalf("the shipped rule cannot read this stand's input box")
	}
	return text
}

// THE CRITERION: the submit key is refused once, and the next grant submits.
func TestTheNextWakeSubmitsTheLineNocxItselfCouldNotSubmit(t *testing.T) {
	sl := logtest.Slog(t)
	s := newOwnLineStand(t, sl)
	typed := s.leaveOurOwnLine(t)

	// THE REFUSAL NAMED THE LINE AS NOCX'S OWN. A person reading the record of
	// a wake that stopped here has to know that the box is holding nocx's bytes
	// and not their draft.
	if !strings.Contains(typed.Reason, "nocx") {
		t.Fatalf("the refusal reads %q, and it does not say whose the standing line is", typed.Reason)
	}

	before := len(s.q.jobs)
	got := s.ty.Submit(context.Background(), pane, s.own)
	if got.Outcome != agenttyping.OutcomeSubmitted {
		t.Fatalf("outcome = %q (%s), want %q: nocx's own line blocked the next wake, which is the defect",
			got.Outcome, got.Reason, agenttyping.OutcomeSubmitted)
	}
	if got.State != agentdriver.StateFreeText {
		t.Fatalf("state = %q, want %q", got.State, agentdriver.StateFreeText)
	}
	// ONE write, and it is the submit key. A second paste would append a second
	// line to the one already in the box and submit both.
	wrote := s.q.jobs[before:]
	if len(wrote) != 1 {
		t.Fatalf("%d writes reached the pane for a line already in its box, want the submit key alone: %q",
			len(wrote), s.q.all())
	}
	if string(wrote[0]) != "\r" {
		t.Fatalf("write = %q, want the submit key on its own", wrote[0])
	}

	// AND IT SAYS SO, naming the bytes as its own: "nocx pressed the key it was
	// refused" has to be tellable from "nocx typed a fresh line".
	if !saysOwnLine(sl) {
		t.Fatalf("no log line names the bytes nocx submitted as its own: %v", said(sl))
	}
}

// A PERSON'S DRAFT IS STILL REFUSED, and this is the half that must not move.
//
// The box holds a text nocx did not leave — the person's own sentence, recorded
// in a different capture of the same agent — and the pane is free_text, so only
// the box gate stands between the wake and their question.
func TestAPersonsDraftIsStillRefusedAfterNocxLeftItsOwnLine(t *testing.T) {
	sl := logtest.Slog(t)
	s := newOwnLineStand(t, sl)
	s.leaveOurOwnLine(t)

	theirs := replay(t, "claude-2.1.272-wrapped-echo", 50000)
	text, ok := rulesOf(t).Observe(agent, theirs).InputText()
	if !ok || text == "" || text == s.own {
		t.Fatalf("the frame must hold a person's text, and not nocx's; got (%q, %v)", text, ok)
	}
	s.sc.frame = theirs

	before := len(s.q.jobs)
	got := s.ty.Submit(context.Background(), pane, s.own)
	if got.Outcome != agenttyping.OutcomeRefused {
		t.Fatalf("outcome = %q (%s), want %q: nocx appended to a person's draft",
			got.Outcome, got.Reason, agenttyping.OutcomeRefused)
	}
	if !strings.Contains(got.Reason, "input box") {
		t.Fatalf("reason = %q, want it to name the input box that refused", got.Reason)
	}
	if wrote := s.q.jobs[before:]; len(wrote) != 0 {
		t.Fatalf("%d writes reached a box holding the person's text: %q", len(wrote), s.q.all())
	}
	// AND IT KEEPS REFUSING: the sweep retries on every settled reading, so one
	// refusal would not be enough.
	got = s.ty.Submit(context.Background(), pane, s.own)
	if got.Outcome != agenttyping.OutcomeRefused {
		t.Fatalf("outcome on the second attempt = %q (%s), want a refusal", got.Outcome, got.Reason)
	}
	if wrote := s.q.jobs[before:]; len(wrote) != 0 {
		t.Fatalf("%d writes reached the person's box across two attempts: %q", len(wrote), s.q.all())
	}
}

// THE RECORD IS CONSUMED BY THE SUBMISSION, so a box that later holds those
// same characters is a PERSON'S and not ours.
//
// Without this a wake would go on believing the box it left is its own for the
// rest of the process's life, and the next person who typed that sentence —
// or, far more plausibly, anything that made the frame read the same — would
// have their turn started by us.
func TestTheOwnLineIsForgottenOnceItIsSubmitted(t *testing.T) {
	sl := logtest.Slog(t)
	s := newOwnLineStand(t, sl)
	s.leaveOurOwnLine(t)

	if got := s.ty.Submit(context.Background(), pane, s.own); got.Outcome != agenttyping.OutcomeSubmitted {
		t.Fatalf("outcome = %q (%s), want the submission this test then asserts is forgotten",
			got.Outcome, got.Reason)
	}
	// The same characters in the box again, and nothing of nocx's left there.
	before := len(s.q.jobs)
	got := s.ty.Submit(context.Background(), pane, s.own)
	if got.Outcome != agenttyping.OutcomeRefused {
		t.Fatalf("outcome = %q (%s), want a refusal: the box holds text nocx did not leave",
			got.Outcome, got.Reason)
	}
	if wrote := s.q.jobs[before:]; len(wrote) != 0 {
		t.Fatalf("%d writes reached a box holding a text nocx had already submitted: %q", len(wrote), s.q.all())
	}
}

// AND THE STATE GATE STILL DECIDES FIRST: a box that holds nocx's own line is
// not a licence to press a key into a pane showing a dialog, which is the
// hazard the whole package exists for.
func TestTheOwnLineIsNotSubmittedIntoAPaneThatIsNotWaitingForInput(t *testing.T) {
	sl := logtest.Slog(t)
	s := newOwnLineStand(t, sl)
	s.leaveOurOwnLine(t)

	menu := replay(t, "claude-2.1.266-permission", 49000)
	s.sc.frame = menu
	if got := rulesOf(t).Classify(agent, menu); got != agentdriver.StatePermissionChoice {
		t.Fatalf("the frame classifies as %q, and this case needs a menu", got)
	}

	before := len(s.q.jobs)
	got := s.ty.Submit(context.Background(), pane, s.own)
	if got.Outcome != agenttyping.OutcomeRefused {
		t.Fatalf("outcome = %q (%s), want %q: a submit key into a dialog answers it",
			got.Outcome, got.Reason, agenttyping.OutcomeRefused)
	}
	if got.State != agentdriver.StatePermissionChoice {
		t.Fatalf("state = %q, want the state that refused", got.State)
	}
	if wrote := s.q.jobs[before:]; len(wrote) != 0 {
		t.Fatalf("%d writes reached a pane showing a dialog: %q", len(wrote), s.q.all())
	}
}

// TYPE IS NOT SUBMIT, and a box holding nocx's own line must take no bytes from
// it: agent.type is the person-facing call, its text would be APPENDED to the
// standing line, and nocx never sent a submit key on that path to be owed.
func TestTypingIntoABoxHoldingNocxSOwnLineStillWritesNothing(t *testing.T) {
	sl := logtest.Slog(t)
	s := newOwnLineStand(t, sl)
	s.leaveOurOwnLine(t)

	before := len(s.q.jobs)
	got := s.ty.Type(context.Background(), pane, s.own)
	if got.Outcome != agenttyping.OutcomeRefused {
		t.Fatalf("outcome = %q (%s), want %q: the text would be appended to the line already there",
			got.Outcome, got.Reason, agenttyping.OutcomeRefused)
	}
	if wrote := s.q.jobs[before:]; len(wrote) != 0 {
		t.Fatalf("%d writes reached a box holding nocx's own line: %q", len(wrote), s.q.all())
	}
}

// AND THE BOX END OF THE INTERVAL IS RE-TAKEN FOR THIS WRITE TOO: the grant
// admits the key on a box holding exactly nocx's line, and the frame that
// AUTHORISES the key then shows the person's sentence — so the key is not
// pressed, and the record survives so the next attempt can still finish it.
//
// It is the counterpart, on the submit key, of what the paste's own box check
// already does: a decision that is stale by the time it is acted on is exactly
// the keystroke this package exists to refuse.
func TestABoxThatChangesBeforeTheSubmitKeyStopsIt(t *testing.T) {
	sl := logtest.Slog(t)
	s := newOwnLineStand(t, sl)
	s.leaveOurOwnLine(t)

	// Read 4 is the grant's, and it still sees nocx's own line; the person's
	// sentence lands by read 5, which is the frame the key is gated on.
	theirs := replay(t, "claude-2.1.272-wrapped-echo", 50000)
	s.sc.changeAfter(4, theirs)

	before := len(s.q.jobs)
	got := s.ty.Submit(context.Background(), pane, s.own)
	if got.Outcome != agenttyping.OutcomeRefused {
		t.Fatalf("outcome = %q (%s), want %q: the key was pressed on a box the person had reached",
			got.Outcome, got.Reason, agenttyping.OutcomeRefused)
	}
	if wrote := s.q.jobs[before:]; len(wrote) != 0 {
		t.Fatalf("%d writes reached a box that no longer held only nocx's line: %q", len(wrote), s.q.all())
	}

	// A REFUSAL IS A WAIT AND NOT A LOSS: with the box reading as nocx's line
	// again, the next attempt finishes the submission it owed.
	s.sc.frame = replay(t, "claude-2.1.266-turn", 47000)
	if got := s.ty.Submit(context.Background(), pane, s.own); got.Outcome != agenttyping.OutcomeSubmitted {
		t.Fatalf("outcome after the box came back = %q (%s), want the submission", got.Outcome, got.Reason)
	}
	if wrote := s.q.jobs[before:]; len(wrote) != 1 || string(wrote[0]) != "\r" {
		t.Fatalf("writes after recovery = %q, want the submit key alone", wrote)
	}
}

// saysOwnLine reports whether the log names the bytes nocx submitted as its
// own, which is the "says so" half of the criterion.
func saysOwnLine(sl *slog.Logger) bool {
	for _, r := range logtest.RecordsSlog(sl) {
		if r.Level >= slog.LevelInfo && strings.Contains(r.Message, "its own line") {
			return true
		}
	}
	return false
}

// said is every line the logger wrote, for the failure message.
func said(sl *slog.Logger) []string {
	out := make([]string, 0)
	for _, r := range logtest.RecordsSlog(sl) {
		out = append(out, r.Level.String()+" "+r.Message)
	}
	return out
}
