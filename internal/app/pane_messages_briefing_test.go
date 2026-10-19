package app

// The briefing's own delivery: the rules and the task, as ONE message
// (nocx-xn63t.4.16; the text is nocx-luqz9.5 and design §6).
//
// ONE MESSAGE, ONE PASTE, ONE ENTER. The briefing is enqueued in one call and
// delivered by the SAME "when=free" path every other queued message takes
// (design §8) — it is not a second write mechanism, and it is not typed by the
// caller either: the caller hands the queue one text and the queue's own
// delivery is what puts it in the pane.
//
// WHAT WAS A GATE IS NOW THE TEXT'S OWN ORDER. The rules and the task were two
// messages once, submitted as two turns, and the queue REFUSED a task whose
// rules had not landed — a worker that read the rules alone reported finding no
// task, which is the defect this bead was filed from (the owner's live run,
// 2026-09-18). With one message there is no second outcome left to wait on: the
// rules are written FIRST inside the very text the worker is given. What is
// asserted here is that shape — exactly one submission, rules before task — and
// the phase the queue records for it.
//
// Every test below runs against the REAL paneMessages over the fakes at the
// paneKeysReader/PaneKeys seam (pane_messages_test.go's own stand), because a
// text that reaches a pane is not something a fake of this package's own queue
// could prove.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shady2k/nocx/internal/agentdriver"
	"github.com/shady2k/nocx/internal/assistant"
	"github.com/shady2k/nocx/internal/helper/proto"
	"github.com/shady2k/nocx/internal/log"
	"github.com/shady2k/nocx/internal/log/logtest"
	"github.com/shady2k/nocx/internal/sessionruntime"
	"github.com/shady2k/nocx/internal/workers"
)

// briefingStand is the fixture every test below shares: one registered
// participant, the real queue over it, and the fake pane seam. The briefing
// names the coordinator the way a registration's own does — the text comes
// from workers.Preamble and never from a copy written here, so a test cannot
// pass on rules no worker was ever given.
type briefingStand struct {
	hub                *paneAccessHub
	access             *DescendantPaneAccess
	sessionID          string
	coordinatorSession string
	participant        workers.Participant
	briefing           workers.Briefing
	reader             *fakeMsgReader
	keys               *fakeMsgKeys
	pm                 *paneMessages
}

func newBriefingStand(t *testing.T, suffix string, keys *fakeMsgKeys) *briefingStand {
	t.Helper()
	return newBriefingStandWith(t, suffix, func(*fakeMsgReader) *fakeMsgKeys { return keys })
}

// newBriefingStandWith is newBriefingStand for a test whose keys have to be
// built OVER the stand's own reader — a keys fake that repaints that pane, or
// one whose answer changes after its first call. newBriefingStand passes a
// keysFor that ignores the reader and hands back the caller's own fake.
func newBriefingStandWith(t *testing.T, suffix string, keysFor func(*fakeMsgReader) *fakeMsgKeys) *briefingStand {
	t.Helper()
	hub, access, _, sessionID := newTestPaneAccess(t, "briefing-"+suffix, &fixedResultHelper{})
	coordinatorSession := access.Controller()
	reader := newFakeMsgReader("")
	var keys *fakeMsgKeys
	if keysFor != nil {
		keys = keysFor(reader)
	}
	if keys == nil {
		keys = happyKeys(reader, "")
	}
	return &briefingStand{
		hub: hub, access: access, sessionID: sessionID,
		coordinatorSession: coordinatorSession,
		participant: workers.Participant{
			ID:       workers.ParticipantID(sessionID),
			Liveness: workers.Liveness{SessionID: sessionID},
		},
		briefing: workers.Briefing{
			Preamble: workers.Preamble(coordinatorSession),
			Task:     "read AGENTS.md and report what it says about workers",
		},
		reader: reader, keys: keys,
		pm: newTestPaneMessages(t, hub, reader, keys),
	}
}

// nocxPending is every namespace "nocx" message the queue holds for this
// participant, in queue order — the readback a coordinator reads it by. A
// briefing is ONE of them, and the COUNT is asserted rather than assumed: a
// regression that enqueued two messages would otherwise be read as whichever
// one a lookup happened to find first.
func (s *briefingStand) nocxPending() []assistant.MessageView {
	var out []assistant.MessageView
	for _, m := range s.pm.Pending(s.sessionID) {
		if m.Namespace == "nocx" {
			out = append(out, m)
		}
	}
	return out
}

// phase answers the briefing's own record — the one namespace "nocx" message,
// which this helper insists on there being exactly one of, so a phase can never
// be read off a half of something that should not be in halves.
func (s *briefingStand) phase() (assistant.MessagePhase, bool) {
	pending := s.nocxPending()
	if len(pending) != 1 {
		return "", false
	}
	return pending[0].Phase, true
}

// exactlyOneBriefing asserts the queue holds this participant's briefing and
// nothing else in namespace "nocx". It is called straight after an enqueue,
// BEFORE anything is delivered, so a regression that queued two messages fails
// here with the shape named — rather than as a wait on phase(), which refuses
// to read a half of something that should not be in halves and would time out
// instead.
func (s *briefingStand) exactlyOneBriefing(t *testing.T) assistant.MessageView {
	t.Helper()
	pending := s.nocxPending()
	if len(pending) != 1 || pending[0].ID != briefingID {
		t.Fatalf("the queue holds %+v, want exactly one namespace \"nocx\" message with id %q", pending, briefingID)
	}
	return pending[0]
}

// pastedTexts is every text this stand's pane was actually written, in the
// order it was pasted — not the order anything was queued.
func (s *briefingStand) pastedTexts() []string {
	s.keys.mu.Lock()
	defer s.keys.mu.Unlock()
	var out []string
	for _, call := range s.keys.calls {
		if call.Text != nil {
			out = append(out, *call.Text)
		}
	}
	return out
}

// Criterion (acceptance 1): a spawned mock worker's pane receives exactly ONE
// submission, carrying the rules and then the task, driven through the real
// queue. The order asserted is the order inside that one message — the rules
// open it, the task follows them — which is the only ordering a worker can
// observe, and the phase is read off the queue's own record rather than
// inferred from the calls.
func TestABriefingReachesTheWorkerAsOneSubmissionWithTheRulesFirst(t *testing.T) {
	s := newBriefingStand(t, "one", nil)

	if err := s.pm.EnqueueBriefing(context.Background(), s.coordinatorSession, s.participant, s.briefing); err != nil {
		t.Fatalf("EnqueueBriefing: %v", err)
	}
	// THE SHAPE, BEFORE ANYTHING IS DELIVERED: one enqueue puts ONE message in
	// namespace "nocx" on the queue. A regression to the two-message briefing
	// this bead removed fails here, naming what is queued.
	s.exactlyOneBriefing(t)
	waitForCondition(t, "the briefing to be submitted", func() bool {
		phase, ok := s.phase()
		return ok && phase == assistant.PhaseSubmitted
	})

	pending := s.nocxPending()
	if len(pending) != 1 || pending[0].ID != briefingID {
		t.Fatalf("the queue holds %+v, want exactly one namespace \"nocx\" message with id %q", pending, briefingID)
	}
	pasted := s.pastedTexts()
	if len(pasted) != 1 {
		t.Fatalf("pastes = %d (%q), want exactly one message carrying the rules and the task", len(pasted), pasted)
	}
	one := pasted[0]
	if one != s.briefing.Text() {
		t.Fatalf("the pasted message is not the briefing's own text:\n%q\nwant:\n%q", one, s.briefing.Text())
	}
	if !strings.HasPrefix(one, s.briefing.Preamble) {
		t.Fatalf("the pasted message does not open with the rules:\n%q", one)
	}
	// The task is looked for in what FOLLOWS the rules, not anywhere in the
	// message: a task that appeared before them would be a briefing whose two
	// halves came out in the wrong order, which is the whole of what this
	// criterion is about.
	if rest := one[len(s.briefing.Preamble):]; !strings.Contains(rest, s.briefing.Task) {
		t.Fatalf("the task does not follow the rules in the pasted message:\n%q", one)
	}
	// ONE ENTER: the submission is the whole briefing's — a second Enter would
	// mean a second message, which is the shape this bead removed.
	if enters := s.keys.enterCalls(); enters != 1 {
		t.Fatalf("Enter calls = %d, want exactly one for one message", enters)
	}
}

// Criterion (acceptance 2, the queue's half): a briefing the pane refuses to
// take types nothing into the worker — no Enter, no half a message — and it
// stays QUEUED rather than ending refused, because a refusal that wrote
// nothing is a verdict about the attempt and not about the message
// (nocx-xn63t.6.59). What bounds it is the delivery's own retry budget: this
// helper never stops refusing, so the delivery stops trying after
// zeroWriteRefusalBudget attempts and the record waits for a cancel or for the
// participant's own incarnation to end — the two things that already end a
// queued message.
//
// The other half of that criterion — that the SPAWN says so — is the
// registration's own: internal/workers/registrar_briefing_test.go's
// TestARegistrationWhoseBriefingCouldNotBeQueuedSaysSoAndQueuesNoTask asserts
// the delivery it hands back carries BriefingQueued false.
func TestABriefingThePaneKeepsRefusingStaysQueuedAndIsNeverSubmitted(t *testing.T) {
	keys := &fakeMsgKeys{pasteResult: assistant.KeysResult{
		State: "refused", Refusal: &proto.IntentRefusal{Cause: "no_read_barrier"},
	}}
	s := newBriefingStand(t, "refused", keys)
	clock := newRecheckClock(t, s.pm)
	testLog := logtest.Slog(t)
	ctx := log.WithLogger(context.Background(), log.NewSlogAdapter(testLog))

	if err := s.pm.EnqueueBriefing(ctx, s.coordinatorSession, s.participant, s.briefing); err != nil {
		t.Fatalf("EnqueueBriefing: %v", err)
	}
	s.exactlyOneBriefing(t)

	// One attempt, then one settled reading between each of the rest: the
	// delivery paces its retries by the free-message safety interval, never
	// by a hot loop, and the test fires each interval itself.
	waitForCondition(t, "the first paste attempt", func() bool { return s.keys.callCount() >= 1 })
	for attempt := 2; attempt <= zeroWriteRefusalBudget; attempt++ {
		want := attempt
		clock.tick(t)
		waitForCondition(t, "the paced retry", func() bool { return s.keys.callCount() >= want })
	}
	// And then nothing more: one further settled reading buys no attempt.
	clock.tick(t)

	if calls := s.keys.callCount(); calls != zeroWriteRefusalBudget {
		t.Fatalf("keys calls = %d after %d settled readings, want the budget's %d and no attempt behind it: a refusal that will never succeed must not be retried without bound",
			calls, zeroWriteRefusalBudget+1, zeroWriteRefusalBudget)
	}
	if enters := s.keys.enterCalls(); enters != 0 {
		t.Fatalf("Enter calls = %d, want none: a briefing that was not pasted must not be submitted", enters)
	}
	phase, ok := s.phase()
	if !ok || phase != assistant.PhaseQueued {
		t.Fatalf("phase = %q, want queued: a paste that wrote nothing does not end the message (nocx-xn63t.6.59)", phase)
	}
	// THE PANE'S TARGET BUDGET (nocx-xn63t.4.1): each attempt spends one
	// one-shot target, and the helper releases a slot only after a minute of
	// token life plus five of result retention, so the number of slots a
	// refusal can ever cost is the budget — never "as many as the clock
	// gives it".
	if live, refusals := s.reader.bookState(); live != zeroWriteRefusalBudget || refusals != 0 {
		t.Fatalf("refusals held %d target slots (and %d capacity refusals), want exactly the budget's %d", live, refusals, zeroWriteRefusalBudget)
	}
	if !logtest.WaitFor(ctx, watchdogBound, func(r logtest.Record) bool {
		return r.Message == deliveryRetriesExhaustedMessage
	}) {
		t.Fatalf("no %q line: a delivery that stops trying has to say so, or the message waits in silence (log was:\n%s)",
			deliveryRetriesExhaustedMessage, logtest.TextSlog(testLog))
	}
}

// nocx-xn63t.6.59'S OWN SCENARIO, at the seam this package can reach: a spawned
// worker's pane answers the queued briefing's paste step `refused` with nothing
// written — what a target that went stale under the dialog's redraw answers
// (the owner's log: op=resize, then op=target and op=intent, then "the paste
// step refused, so nothing was written") — and the briefing must NOT end there.
// It stays queued and arrives by itself on the next settled reading, with no
// second session.message from the coordinator.
func TestABriefingRefusedWithNothingWrittenStaysQueuedAndArrivesByItself(t *testing.T) {
	s := newBriefingStandWith(t, "refused-stale", refusedOnceThenHappyKeys)
	clock := newRecheckClock(t, s.pm)
	testLog := logtest.Slog(t)
	ctx := log.WithLogger(context.Background(), log.NewSlogAdapter(testLog))

	if err := s.pm.EnqueueBriefing(ctx, s.coordinatorSession, s.participant, s.briefing); err != nil {
		t.Fatalf("EnqueueBriefing: %v", err)
	}
	s.exactlyOneBriefing(t)

	// The refusal itself: one paste step answered `refused` with zero bytes.
	waitForCondition(t, "the first paste attempt to be refused", func() bool {
		return s.keys.callCount() >= 1
	})
	// THE DEFECT, stated where it can fail on its own: the record is still
	// queued. Ending it refused here is what left the owner's worker at an
	// empty prompt with no task and no rules.
	waitForCondition(t, "the briefing to stay queued after a refusal that wrote nothing", func() bool {
		phase, ok := s.phase()
		return ok && phase == assistant.PhaseQueued
	})
	if enters := s.keys.enterCalls(); enters != 0 {
		t.Fatalf("Enter calls = %d, want none: nothing was written, so nothing may be submitted", enters)
	}
	// AND THE LINE SAYS WHICH STATE AND WHY: the owner's log could not tell a
	// stale target from a cancelled message, so the next occurrence has to be
	// one line rather than a re-send by hand.
	if !logtest.WaitFor(ctx, watchdogBound, func(r logtest.Record) bool {
		return r.Message == deliveryRefusedMessage &&
			recordAttr(r, "paste_state") == "refused" &&
			recordAttr(r, "refusal_cause") == "stale_target"
	}) {
		t.Fatalf("no %q line naming paste_state=refused and refusal_cause=stale_target (log was:\n%s)",
			deliveryRefusedMessage, logtest.TextSlog(testLog))
	}

	// THE PANE SETTLES — the redraw that made the target stale is over — and
	// the next settled reading delivers the message that was never lost.
	clock.fire(t)
	waitForCondition(t, "the briefing to arrive on the next settled reading", func() bool {
		phase, ok := s.phase()
		return ok && phase == assistant.PhaseSubmitted
	})
	if enters := s.keys.enterCalls(); enters != 1 {
		t.Fatalf("Enter calls = %d, want exactly one for one briefing", enters)
	}
	// TWO paste CALLS, one of which was refused and wrote nothing: the text
	// reached the pane exactly once.
	if pasted := s.pastedTexts(); len(pasted) != 2 || pasted[0] != s.briefing.Text() || pasted[1] != s.briefing.Text() {
		t.Fatalf("pastes = %q, want the refused attempt and then the briefing's one message", pasted)
	}
}

// refusedOnceThenHappyKeys answers the FIRST paste with the helper's own
// refusal of a target that went stale, and every later one with the ordinary
// happy path — the pane that answered a dialog and settled, at the keys seam
// where that sequence is a script rather than a race.
func refusedOnceThenHappyKeys(reader *fakeMsgReader) *fakeMsgKeys {
	k := &fakeMsgKeys{
		pasteResult: assistant.KeysResult{State: "refused", Refusal: &proto.IntentRefusal{Cause: "stale_target"}},
		enterResult: assistant.KeysResult{State: "executed"},
	}
	pastes := 0
	k.onSend = func(req assistant.KeysRequest) {
		switch {
		case req.Text != nil:
			pastes++
			// Every call AFTER the first is answered as an ordinary paste. The
			// first call's own answer was already read before this hook ran, so
			// it stays the refusal it was constructed with — and nothing
			// echoes for it, because a refused write reached no box.
			k.mu.Lock()
			k.pasteResult = assistant.KeysResult{State: "executed", BytesWritten: len(*req.Text)}
			k.mu.Unlock()
			if pastes == 1 {
				return
			}
			reader.setBox(pastedEcho(*req.Text))
		case req.Key != nil:
			reader.setBox("")
		}
	}
	return k
}

// Criterion (acceptance 3): with a menu up, NOTHING is typed and the briefing
// stays queued rather than being refused or written into the menu; the existing
// "when=free" retry is the whole mechanism, and it is the same one every other
// queued message uses.
//
// The menu is the corpus's own frame and the shipped rule's own classification
// (pane_messages_test.go's menuUp), so pasteReady refuses for the reason a real
// pane refuses: a Claude menu displaces the input box.
func TestABriefingBehindAMenuTypesNothingAndThenDeliversOnce(t *testing.T) {
	s := newBriefingStand(t, "menu", nil)
	menuUp(t, s.reader)

	if err := s.pm.EnqueueBriefing(context.Background(), s.coordinatorSession, s.participant, s.briefing); err != nil {
		t.Fatalf("EnqueueBriefing: %v", err)
	}
	s.exactlyOneBriefing(t)
	waitForCondition(t, "the briefing to be recorded as queued", func() bool {
		phase, ok := s.phase()
		return ok && phase == assistant.PhaseQueued
	})
	if calls := s.keys.callCount(); calls != 0 {
		t.Fatalf("keys calls = %d, want 0 while the menu is still up", calls)
	}

	// The menu clears — a person answering it, or the coordinator's own
	// session.keys — and the queue delivers the briefing: one text, one Enter.
	s.reader.setFrame(boxFrame(""), agentdriver.StateFreeText, sessionruntime.TargetInput)
	s.pm.PaneChanged(s.sessionID)
	waitForCondition(t, "the briefing to reach submitted once the menu cleared", func() bool {
		phase, ok := s.phase()
		return ok && phase == assistant.PhaseSubmitted
	})
	pasted := s.pastedTexts()
	if len(pasted) != 1 || pasted[0] != s.briefing.Text() {
		t.Fatalf("pastes = %q, want the briefing's one message", pasted)
	}
	if enters := s.keys.enterCalls(); enters != 1 {
		t.Fatalf("Enter calls = %d, want exactly one", enters)
	}
}

// A pane can become free while pasteReady is reading it. The observation
// callback may run before readiness returns; the queue must already be listening
// so that this change is not lost between the read and the wait.
func TestABriefingDoesNotLosePaneChangeDuringReadinessProbe(t *testing.T) {
	s := newBriefingStand(t, "change-during-probe", nil)
	menuUp(t, s.reader)
	// Disable the safety retry: delivery must proceed from the change that
	// happened during its readiness read, not from a later periodic retry.
	never := make(chan time.Time)
	s.pm.after = func(time.Duration) <-chan time.Time { return never }
	s.reader.mu.Lock()
	s.reader.onRead = func(n int) {
		if n != 1 {
			return
		}
		s.reader.setFrame(boxFrame(""), agentdriver.StateFreeText, sessionruntime.TargetInput)
		s.pm.PaneChanged(s.sessionID)
	}
	s.reader.mu.Unlock()
	if err := s.pm.EnqueueBriefing(context.Background(), s.coordinatorSession, s.participant, s.briefing); err != nil {
		t.Fatalf("EnqueueBriefing: %v", err)
	}
	waitForCondition(t, "the briefing to submit after a pane change during its readiness probe", func() bool {
		phase, ok := s.phase()
		return ok && phase == assistant.PhaseSubmitted
	})
	if got := s.pastedTexts(); len(got) != 1 || got[0] != s.briefing.Text() {
		t.Fatalf("pastes = %q, want one briefing", got)
	}
}

// A briefing is rules AND a task. Half of one is refused at the QUEUE — the
// caller never reaches the delivery — so "a worker was handed work without ever
// being told how to report on it" is not a state a caller can put this queue
// into. With one message the refusal is the value's own
// (workers.Briefing.Validate), because there is no longer a later moment at
// which a missing rules half could be noticed.
func TestABriefingWithoutItsRulesIsRefusedRatherThanQueued(t *testing.T) {
	s := newBriefingStand(t, "half", nil)

	err := s.pm.EnqueueBriefing(context.Background(), s.coordinatorSession, s.participant,
		workers.Briefing{Task: "do the thing"})
	if err == nil {
		t.Fatal("a briefing with no rules was queued")
	}
	if pending := s.nocxPending(); len(pending) != 0 {
		t.Fatalf("a refused briefing left %+v queued", pending)
	}
	if calls := s.keys.callCount(); calls != 0 {
		t.Fatalf("keys calls = %d, want 0", calls)
	}
}

// The idempotency the task's own queue already had (design §8.4) is kept for
// the briefing: a second briefing for the same participant incarnation queues
// nothing new, and a second briefing whose TEXT differs is refused rather than
// silently replacing what the worker was told.
func TestASecondBriefingForTheSameWorkerIsNotQueuedTwice(t *testing.T) {
	s := newBriefingStand(t, "idempotent", nil)
	ctx := context.Background()

	if err := s.pm.EnqueueBriefing(ctx, s.coordinatorSession, s.participant, s.briefing); err != nil {
		t.Fatalf("first EnqueueBriefing: %v", err)
	}
	s.exactlyOneBriefing(t)
	if err := s.pm.EnqueueBriefing(ctx, s.coordinatorSession, s.participant, s.briefing); err != nil {
		t.Fatalf("second EnqueueBriefing: %v", err)
	}
	waitForCondition(t, "the briefing to reach submitted", func() bool {
		phase, ok := s.phase()
		return ok && phase == assistant.PhaseSubmitted
	})
	if pasted := s.pastedTexts(); len(pasted) != 1 {
		t.Fatalf("pastes = %q, want the briefing delivered exactly once", pasted)
	}
	if pending := s.nocxPending(); len(pending) != 1 {
		t.Fatalf("the queue holds %+v, want one briefing", pending)
	}

	changed := s.briefing
	changed.Task = s.briefing.Task + ", and report how many you found"
	err := s.pm.EnqueueBriefing(ctx, s.coordinatorSession, s.participant, changed)
	if err == nil {
		t.Fatal("a briefing whose text differs from the one already delivered was accepted")
	}
}

// A briefing queued for a participant that is GONE before its pane frees up is
// refused rather than delivered, and nothing is written to its pane — the
// participant was closed (Kill, or the ordinary exit path
// workerSupervisor.report reduces), which bumps its delegation generation and
// makes StillHolds false for the chain the briefing resolved.
//
// This was the owed task's own test before nocx-luqz9.5 (there is no debt map
// any more: the queue re-checks StillHolds before every delivery attempt, the
// same mechanism design §8.6 gives every other message), and it is kept
// verbatim through the briefing because the rule it pins is unchanged — a
// participant that is gone is written nothing, and is not retried forever.
func TestABriefingForAParticipantThatIsGoneIsRefusedRatherThanDelivered(t *testing.T) {
	s := newBriefingStand(t, "gone", nil)
	// A menu is up, exactly as at the moment a spawn left this worker its
	// briefing: nothing can be written yet, so it waits.
	menuUp(t, s.reader)

	if err := s.pm.EnqueueBriefing(context.Background(), s.coordinatorSession, s.participant, s.briefing); err != nil {
		t.Fatalf("EnqueueBriefing: %v", err)
	}
	s.exactlyOneBriefing(t)
	waitForCondition(t, "the briefing to be recorded as queued", func() bool {
		phase, ok := s.phase()
		return ok && phase == assistant.PhaseQueued
	})

	if _, err := s.hub.registrar.Close(context.Background(), s.coordinatorSession, workers.ParticipantID(s.sessionID)); err != nil {
		t.Fatalf("close (simulate Kill/exit): %v", err)
	}

	// The menu now clears — a person could still press Enter on a pane nocx
	// no longer holds authority over — and the queue must refuse rather than
	// deliver, and must not spin forever either.
	s.reader.setFrame(boxFrame(""), agentdriver.StateFreeText, sessionruntime.TargetInput)
	// The authority safety recheck (bounded to one second) observes the close;
	// no pane-state change is required for this refusal.
	waitForCondition(t, "the briefing to end refused rather than deliver to a gone participant", func() bool {
		phase, ok := s.phase()
		return ok && phase == assistant.PhaseRefused
	})
	if calls := s.keys.callCount(); calls != 0 {
		t.Fatalf("keys calls = %d, want 0: nothing may be written to a participant that is gone", calls)
	}
}
