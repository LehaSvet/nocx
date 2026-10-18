package app

// The wake, at the composition level (nocx-luqz9.3; ADR-0070 decision 4,
// design §5).
//
// What is under test here is the thing a person depends on: a coordinator that
// has gone idle is TYPED AT by the backend when its mailbox has something new,
// and it is typed at THROUGH THE SHIPPED GATES — the real grid fed from byte
// zero of a real capture, the real rule, a real calibration verdict, and the one
// Typist the agent.type method reaches. Nothing here fakes a screen, because the
// screen is the thing that decides whether a keystroke is safe, and a test that
// faked it would be asserting that a fake permits typing.
//
// The readings of the coordinator's own pane arrive the way production's do:
// through the observation bridge, off the sweep the watcher already runs. That
// is the hop the unit tests cannot cover, and it is the one that decides whether
// a coordinator is ever typed at in the product.

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/agentcalib"
	"github.com/shady2k/nocx/internal/agentcapture"
	"github.com/shady2k/nocx/internal/agentcapture/replaylocal"
	"github.com/shady2k/nocx/internal/agentdriver"
	"github.com/shady2k/nocx/internal/agenttyping"
	"github.com/shady2k/nocx/internal/lifecycle"
	"github.com/shady2k/nocx/internal/log"
	"github.com/shady2k/nocx/internal/notify"
	"github.com/shady2k/nocx/internal/paneobserve"
	"github.com/shady2k/nocx/internal/paneview"
	"github.com/shady2k/nocx/internal/paneview/paneviewtest"
	"github.com/shady2k/nocx/internal/session"
	"github.com/shady2k/nocx/internal/transport"
	"github.com/shady2k/nocx/internal/waittest"
	"github.com/shady2k/nocx/internal/workers"
)

// ── the stand ─────────────────────────────────────────────────────────────

// recordingRaiser is the far end of the escalation. It is the notify seam and
// not the whole pipeline: what an escalation may REACH is internal/notify's
// and is asserted there, against a routing table this test does not own.
//
// A RAISE THAT FAILS IS REPORTED THROUGH Outcome.Err AND NOTHING ELSE, and
// that is not this double's convention — it is the seam's own. The pipeline the
// adapter stands behind is asynchronous past its debounce window
// (notify.Ingress.Raise returns an empty Outcome by design), so Resolved and
// Results are empty on every notice that was ACCEPTED, including one no channel
// will ever take. The only failure a caller here can act on is admission being
// refused, and that is what `refuse` stands for.
type recordingRaiser struct {
	events []notify.Event
	// refuse is the admission refusal this raise reports. Nil is the ordinary
	// accepted case, which is what every other test in this file wants.
	refuse error
}

func (r *recordingRaiser) Raise(_ context.Context, ev notify.Event) notify.Outcome {
	r.events = append(r.events, ev)
	return notify.Outcome{Event: ev, Err: r.refuse}
}

// wakeStand is a worker stand plus a REAL coordinator pane: a session with a
// pty, a grid enrolled from byte zero, an observation, and a calibration
// verdict that permits typing.
type wakeStand struct {
	*workerStand
	coordinator session.ID
	coordPTY    *recordingPTY
	grid        *paneviewtest.Views
	rules       *agentdriver.Registry
	raiser      *recordingRaiser
	chunks      []agentcapture.Chunk
	workerID    workers.ID
	// observe is the production bridge — the SAME one the composition root
	// binds to the sweep — so what these tests drive is the mapping and the
	// record's settle rule together rather than a call the product never makes.
	observe *WorkerObservation
	// workerSessions is every session a worker has already been given, so a
	// second registration waits for a session that did not exist yet.
	workerSessions map[session.ID]bool
}

const wakeAgent = "claude"

// standMailbox is the record's own store, read by the wake.
//
// It exists because of an ordering the composition root does not have: the wake
// is built before the record, and the record owns the store, so `nil` is
// correct at construction and the store arrives a few lines later. It is a
// deferral and not a second view of the mailbox — every call goes straight
// through to the one store, so there is one order, one cursor and one set of
// rows.
type standMailbox struct{ store workers.Store }

func (m *standMailbox) Since(ctx context.Context, mailbox workers.ReaderID, after int64, limit int) ([]workers.Message, error) {
	return m.store.Since(ctx, mailbox, after, limit)
}

func newWakeStand(t *testing.T) *wakeStand {
	t.Helper()
	ctx := context.Background()
	logger := log.NewSlogAdapter(nil)

	grid := paneviewtest.NewViews(logger)
	rules, err := agentdriver.NewRegistry(agentdriver.Claude())
	if err != nil {
		t.Fatalf("driver registry: %v", err)
	}
	watch := paneobserve.New(logger, grid.Store, rules, paneobserve.Config{})
	raiser := &recordingRaiser{}

	// The stand is built with the RECORD's own wiring already in it, because
	// the record is what drives the wake off the readings the sweep produces:
	// wiring it afterwards would let a test assert a mechanism the product does
	// not have. The window is ZERO here and it is a configuration rather than a
	// mistake — it means "the second reading of a state is a settled one",
	// which is what a test at this level asks for, because it cannot move the
	// record's clock. internal/workers' own tests move that clock and state the
	// real window.
	// The box the wake counts is the record's own store, which does not exist
	// until the stand is built — the wake is constructed first because the
	// record's constructor takes it. The deferral lives HERE rather than as a
	// setter on production code, which hands the store over directly and has no
	// need of one.
	box := &standMailbox{}
	var typist *agenttyping.Typist
	waker := &workerWaker{typist: nil, log: logger}
	wake := workers.NewWake(waker,
		&workerEscalation{raise: raiser}, box)
	stand := newWorkerStand(t,
		workers.WithWake(wake),
		workers.WithSettleWindow(0),
	)

	typist = newPaneTypist(logger, grid.Store, rules, verifiedClaude(t), watch, stand.reg)
	waker.typist = typist
	// Now the store exists: the SAME box the coordinator fetches from, which
	// is what makes the number the wake types the number a reader will find.
	box.store = stand.workerStore

	// The coordinator's own pane, opened through the SAME one session-open
	// path the product uses (nocx-dkawo.6) — no client attached, which is the
	// whole situation a worker exists for.
	opened, err := stand.tp.OpenSession(ctx, transport.OpenSpec{
		PaneID: "pane-coordinator", Cols: participantCols, Rows: participantRows,
	})
	if err != nil {
		t.Fatalf("open the coordinator's session: %v", err)
	}
	coordinator := opened.Session.ID()
	coordPTY := stand.ptys.last()

	// Its grid, opened from byte zero, and its observation — the two halves of
	// one act, exactly as the pane enroller opens them.
	if enrolErr := grid.Watch(string(coordinator), participantCols, participantRows); enrolErr != nil {
		t.Fatalf("enrol the coordinator's pane: %v", enrolErr)
	}
	t.Cleanup(func() { grid.Withdraw(string(coordinator)) })
	watch.Watch(string(coordinator), wakeAgent)

	workerID := workers.ID("worker-wake")
	stand.ensureWorker(t, workerID, string(coordinator))

	//nolint:gosec // The path is two joined literals naming a corpus in the tree.
	header, chunks, err := agentcapture.Read(
		filepath.Join("..", "agentdriver", "testdata", "captures", "claude-idle.jsonl"))
	if err != nil {
		t.Fatalf("read the idle capture: %v", err)
	}
	if header.Cols != participantCols || header.Rows != participantRows {
		t.Fatalf("the capture is %dx%d and the pane is %dx%d; the frame would be wrapped differently",
			header.Cols, header.Rows, participantCols, participantRows)
	}

	s := &wakeStand{
		workerStand: stand, coordinator: coordinator, coordPTY: coordPTY,
		grid: grid, rules: rules, raiser: raiser,
		chunks: chunks, workerID: workerID,
		workerSessions: map[session.ID]bool{},
	}
	// THE BRIDGE, built here as the composition root builds it — with the record
	// behind it. Its `coordinate` seam is what reaches the wake, and it is the
	// same one app.New binds, so a coordinator typed at in the product is typed
	// at by this code path.
	s.observe = &WorkerObservation{
		enrolments: stand.enrol,
		observe: func(ctx context.Context, id workers.ParticipantID, l workers.Liveness, st workers.ObservedState) error {
			return stand.record.Observe(ctx, id, l, st)
		},
		coordinate: func(sessionID string, st workers.ObservedState) {
			stand.record.ObserveCoordinator(context.Background(), sessionID, st)
		},
		liveness: stand.enrol.livenessOf,
		log:      logger,
	}
	return s
}

// driveTo feeds the coordinator's real grid forward through the capture until
// the shipped rule classifies it as want.
//
// It waits on the CLASSIFICATION and never on a duration: the grid is fed
// through a pipe into an emulator on its own goroutine, so a test that slept
// would be asserting that this machine is fast enough.
func (w *wakeStand) driveTo(t *testing.T, atMs int64, want agentdriver.State) {
	t.Helper()
	through := agentcapture.ChunksThrough(w.chunks, atMs, 0)
	for _, c := range w.chunks[:through] {
		w.grid.Feed(string(w.coordinator), c.Data)
	}
	waittest.WaitFor(t, "the coordinator's pane to reach "+string(want), func() bool {
		f, err := w.grid.Frame(string(w.coordinator))
		return err == nil && w.rules.Classify(wakeAgent, f) == want
	})
}

// reads is one classification of the coordinator's own pane, delivered the way
// the sweep delivers it — through the bridge, so the mapping and the settle
// rule are both exercised.
//
// It is called TWICE per settled state and once per reading in between, because
// the record's window is zero here: the first call is a state it has not seen
// and the second is that state having held, which is what "settled" means.
func (w *wakeStand) reads(t *testing.T, state agentdriver.State) {
	t.Helper()
	// The real frame is read first, so the grid and the driver agree that this
	// is what the pane shows: a classification the pane does not support would
	// be a fact this test invented.
	f, err := w.grid.Frame(string(w.coordinator))
	if err != nil {
		t.Fatalf("read the coordinator's own frame: %v", err)
	}
	if got := w.rules.Classify(wakeAgent, f); got != state {
		t.Fatalf("the coordinator's pane is %q and this call says %q", got, state)
	}
	o := paneobserve.Observation{PaneID: string(w.coordinator), Agent: wakeAgent, State: state}
	w.observe.ObserveSession(o)
	w.observe.ObserveSession(o)
}

// register starts one worker in the wake stand's worker and supplies the
// enrolment its launcher would have sent.
//
// It waits for a session it has not seen before, so a second and a third
// worker are not satisfied by the first one's session — a helper that matched
// "any session that is not the coordinator" would let a fan-out test pass
// while only one worker ever started.
func (w *wakeStand) register(t *testing.T, task string) workers.Participant {
	t.Helper()
	type outcome struct {
		p   workers.Participant
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		p, err := w.record.Register(context.Background(), workers.RegisterRequest{
			Group: w.workerID, CoordinatorSession: string(w.coordinator),
			Role: workers.RoleWorker, Task: task, Command: wakeAgent,
		})
		done <- outcome{p.Participant, err}
	}()
	var sid session.ID
	waittest.WaitFor(t, "a new worker session to exist", func() bool {
		for _, s := range w.reg.List() {
			if s.ID() == w.coordinator || w.workerSessions[s.ID()] {
				continue
			}
			sid = s.ID()
			return true
		}
		return false
	})
	w.workerSessions[sid] = true
	lane := lifecycle.LaneID("lane-worker-" + string(sid))
	w.lanes.register(lane, string(sid))
	w.enrol.enrolled(sid, string(lane))
	got := <-done
	if got.err != nil {
		t.Fatalf("register: %v", got.err)
	}
	return got.p
}

// settledIdle drives one worker's pane to settled idle, which is what puts a
// message in the coordinator's mailbox.
func (w *wakeStand) settledIdle(t *testing.T, p workers.Participant) {
	t.Helper()
	o := paneobserve.Observation{
		PaneID: p.Liveness.SessionID, Agent: wakeAgent, State: agentdriver.StateFreeText,
	}
	w.observe.ObserveSession(o)
	w.observe.ObserveSession(o)
}

// verifiedClaude drives a REAL calibration to completion against the shipped
// claude rule, using the corpus's own frames for the three states a person is
// asked to produce. There is no other way to obtain a verdict that permits
// typing — agentcalib.Verdict's permission is unexported — so a test that
// wanted to shortcut this would be faking the gate it is here to exercise.
func verifiedClaude(t *testing.T) agenttyping.Authority {
	t.Helper()
	frames := &stepFrames{frames: []paneview.Frame{
		captureFrame(t, "claude-idle", 11000),       // Begin: the geometry
		captureFrame(t, "claude-idle", 11000),       // idle     → free_text
		captureFrame(t, "claude-working", 17000),    // working  → working
		captureFrame(t, "claude-permission", 49000), // asks-you → permission_choice
	}}
	store, err := agentcalib.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("calibration store: %v", err)
	}
	rules, err := agentdriver.NewRegistry(agentdriver.Claude())
	if err != nil {
		t.Fatalf("driver registry: %v", err)
	}
	calib := agentcalib.New(log.NewSlogAdapter(nil), frames, store, rules, replaylocal.Replayer{})
	if _, err := calib.Begin(context.Background(), "calibration-pane", wakeAgent); err != nil {
		t.Fatalf("begin calibration: %v", err)
	}
	for i, step := range agentcalib.Steps() {
		answer := agentcalib.AnswerCapture
		if !step.Required {
			answer = agentcalib.AnswerSkip
		}
		if _, err := calib.Answer(context.Background(), "calibration-pane", i, answer); err != nil {
			t.Fatalf("answer step %d (%s): %v", i, step.Label, err)
		}
	}
	if v := calib.Verify(context.Background(), wakeAgent); !v.MayType() {
		t.Fatalf("the shipped rule did not verify against the corpus it was written from: %+v", v)
	}
	return calib
}

// stepFrames hands out one frame per read, which is what a calibration walk
// is: a person drives their agent into a state and nocx labels what it sees.
type stepFrames struct {
	frames []paneview.Frame
	at     int
}

func (s *stepFrames) Frame(string) (paneview.Frame, error) {
	if s.at >= len(s.frames) {
		return paneview.Frame{}, paneview.ErrNotWatched
	}
	f := s.frames[s.at]
	s.at++
	return f, nil
}

func captureFrame(t *testing.T, name string, atMs int64) paneview.Frame {
	t.Helper()
	//nolint:gosec // The path is joined literals naming a corpus in the tree.
	header, chunks, err := agentcapture.Read(
		filepath.Join("..", "agentdriver", "testdata", "captures", name+".jsonl"))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	moments, err := agentcapture.Frames(context.Background(), replaylocal.Replayer{}, header, chunks, []int64{atMs})
	if err != nil {
		t.Fatalf("replay %s to %dms: %v", name, atMs, err)
	}
	return moments[0].Frame
}

// ── the criterion, through the shipped seams ──────────────────────────────

// THE ACCEPTANCE CRITERION at the composition level: a coordinator that has
// been sitting idle is woken by the backend when a worker's pane settles, and
// the line reaches its own pty through the shipped typing gate without a person
// touching anything.
//
// The four assertions are ordered the way the mechanism is: nothing is typed
// while the mailbox is empty; the settled state puts a message in it; the wake
// reaches the coordinator's pty and carries no worker content; and the
// coordinator's OWN read is what clears the batch.
func TestAnIdleCoordinatorIsWokenWhenItsWorkerSettlesIdle(t *testing.T) {
	w := newWakeStand(t)
	ctx := context.Background()
	w.driveTo(t, 11000, agentdriver.StateFreeText)

	p := w.register(t, "read AGENTS.md and report")

	// The round trip that has to happen before anything is typed: the
	// coordinator's own pane is read as idle and that reading is settled.
	w.reads(t, agentdriver.StateFreeText)
	if got := w.coordPTY.read(); got != "" {
		t.Fatalf("nocx typed into a coordinator with an empty mailbox: %q", got)
	}

	// The worker's pane settles idle, which is ONE message in the mailbox.
	w.settledIdle(t, p)

	// The bytes travel the SAME queue a person's keystrokes take, and the queue
	// drains on the session's own write loop — so this waits on the pty having
	// them rather than on a duration.
	//
	// The condition is the WHOLE wake — the text and the submit key that follows
	// it as a separate write. Waiting on the text alone would be satisfied by
	// the text, which is the state in which the coordinator is looking at an
	// unsent line and no turn has started.
	var typed string
	waittest.WaitFor(t, "the wake and its submit key to reach the coordinator's pty", func() bool {
		typed = w.coordPTY.read()
		return strings.Contains(typed, "workers.inbox") && strings.HasSuffix(typed, "\r")
	})
	// THE SENTENCE NAMES WHAT IS WAITING (nocx-xn63t.6.39), and what this worker
	// produced is an OBSERVATION: it settled idle and said nothing.
	const want = "nocx: you have 1 new observation of your workers. Call workers.inbox."
	if !strings.Contains(typed, want) {
		t.Fatalf("the coordinator's pane was typed at with %q, want it to contain %q", typed, want)
	}
	// NO WORKER CONTENT, asserted as the absence of everything the worker ever
	// said: its participant id and its task are the only text it owns, and a
	// line carrying free text from a model into another agent's input region is
	// prompt injection performed with our own hands.
	if strings.Contains(typed, string(p.ID)) || strings.Contains(typed, "AGENTS.md") {
		t.Fatalf("the wake carried a worker's own words into the coordinator's pane: %q", typed)
	}
	// The submit key, sent SEPARATELY so it cannot be swallowed as paste
	// content. Without it the coordinator is looking at an unsent line and no
	// turn starts, which is the difference between typing and waking.
	if !strings.HasSuffix(typed, "\r") {
		t.Fatalf("nothing submitted the wake, so no turn starts: %q", typed)
	}

	// And the coordinator's own read is what ends the batch.
	if got := len(w.record.Undispatched()); got != 0 {
		t.Fatalf("undispatched before the coordinator looked = %d, want 0: a settled idle is not a fact", got)
	}
	box := workers.ReaderID(w.coordinator)
	fetched, err := w.record.Inbox(ctx, box, box, 0)
	if err != nil {
		t.Fatalf("the coordinator's own read: %v", err)
	}
	if len(fetched.Messages) != 1 || fetched.Messages[0].Observed == nil {
		t.Fatalf("the coordinator was handed %+v, want the one observation", fetched.Messages)
	}

	// A read that has happened retypes nothing, at any pause.
	before := len(w.coordPTY.read())
	waittest.WaitFor(t, "the record to settle after the read", func() bool { return true })
	if got := w.coordPTY.read()[before:]; got != "" {
		t.Fatalf("a batch that was read was typed at anyway: %q", got)
	}
}

// A COORDINATOR THAT IS NOT WAITING FOR INPUT RECEIVES NOTHING AT ALL, and
// nothing is armed under the refusal.
//
// This is the hazard the whole typing gate exists for: a keystroke into a
// permission menu does not merely fail to deliver, it ANSWERS the menu, which
// can approve a tool call the person never saw.
func TestACoordinatorThatIsNotWaitingForInputIsNotTyped(t *testing.T) {
	w := newWakeStand(t)
	w.driveTo(t, 11000, agentdriver.StateFreeText)
	p := w.register(t, "reports into a busy coordinator")

	// The coordinator is idle and settled first — so it is a pane nocx has
	// already decided it MAY type into — and then it starts working.
	w.reads(t, agentdriver.StateFreeText)
	busy := replayInto(t, w, "claude-working", 17000, agentdriver.StateWorking)
	if !busy {
		t.Fatalf("the corpus did not drive the pane to working")
	}
	w.reads(t, agentdriver.StateWorking)

	before := len(w.coordPTY.read())
	w.settledIdle(t, p)
	if got := w.coordPTY.read()[before:]; got != "" {
		t.Fatalf("nocx typed %q into a pane that was not waiting for input", got)
	}
	// And the mail is still there: a refusal is not a loss, it is a wait.
	box := workers.ReaderID(w.coordinator)
	fetched, err := w.record.Inbox(context.Background(), box, box, 0)
	if err != nil {
		t.Fatalf("read the mailbox: %v", err)
	}
	if len(fetched.Messages) != 1 {
		t.Fatalf("the observation was lost with the refusal: %+v", fetched.Messages)
	}
}

// replayInto feeds a second capture onto the coordinator's live grid and
// reports whether the rule reached want. It is a second capture on the SAME
// grid deliberately: a pane's state changes under a grid that was opened once,
// which is exactly what the interval means.
func replayInto(t *testing.T, w *wakeStand, name string, atMs int64, want agentdriver.State) bool {
	t.Helper()
	//nolint:gosec // The path is joined literals naming a corpus in the tree.
	_, chunks, err := agentcapture.Read(
		filepath.Join("..", "agentdriver", "testdata", "captures", name+".jsonl"))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	through := agentcapture.ChunksThrough(chunks, atMs, 0)
	for _, c := range chunks[:through] {
		w.grid.Feed(string(w.coordinator), c.Data)
	}
	reached := false
	waittest.WaitFor(t, "the coordinator's pane to reach "+string(want), func() bool {
		f, ferr := w.grid.Frame(string(w.coordinator))
		if ferr != nil {
			return false
		}
		reached = w.rules.Classify(wakeAgent, f) == want
		return reached
	})
	return reached
}

// ── the person's own draft (nocx-xn63t.6.44) ─────────────────────────────

// A WAKE MUST NOT BE APPENDED TO WHAT THE PERSON IS TYPING IN THAT PANE.
//
// The pane is free_text either way — that is what the state gate reads, and it
// is why the state gate alone let a wake be concatenated with a draft and
// submitted with it on 2026-10-18. What distinguishes "the coordinator is idle
// and waiting" from "somebody has started a question there" is the BOX.
//
// Both halves are the same recorded session: `claude-2.1.266-turn` holds the
// person's sentence in the box at 47s, submits it, and is back to an idle empty
// box at 84s. Nothing here is painted over anything else and no frame is made
// up: the pane is enrolled empty and fed ONE capture forward from its byte
// zero, in order.
func TestACoordinatorWakeDoesNotTypeOverThePersonsDraft(t *testing.T) {
	w := newWakeStand(t)
	ctx := context.Background()

	// The person is at the keyboard: their question is in the coordinator's
	// box, and the pane is still waiting for input.
	turn := openCapture(t, "claude-2.1.266-turn")
	turn.to(t, w, 47000, agentdriver.StateFreeText, "the person to have typed a draft")
	if got := w.coordBox(t); got == "" {
		t.Fatalf("the coordinator's box reads empty, and this test needs the draft the corpus recorded there")
	}

	p := w.register(t, "reports while the person is typing")

	// The coordinator is idle and settled WITH the draft still in its box.
	w.reads(t, agentdriver.StateFreeText)
	before := len(w.coordPTY.read())

	// Mail lands. The wake decides — and the decision has to be "not now".
	w.settledIdle(t, p)
	// One more settled reading: the sweep retries a refused line on every one,
	// which is the mechanism the wait depends on, so this is a second chance to
	// get it wrong rather than a sleep.
	w.reads(t, agentdriver.StateFreeText)
	if got := w.coordPTY.read()[before:]; got != "" {
		t.Fatalf("nocx typed %q into a box holding the person's draft", got)
	}
	// A refusal is a WAIT and not a loss: the mail is still unread.
	waiting, err := w.workerStore.Since(ctx, workers.ReaderID(w.coordinator), 0, 10)
	if err != nil {
		t.Fatalf("read the mailbox: %v", err)
	}
	if len(waiting) == 0 {
		t.Fatalf("the observation was lost with the refusal, so the coordinator will never hear about it")
	}

	// The person sends their question and the turn ends: the box is empty and
	// the pane is idle again, which is the state the wake may type in.
	turn.to(t, w, 84000, agentdriver.StateFreeText, "the person's turn to end with an empty box")
	if got := w.coordBox(t); got != "" {
		t.Fatalf("the coordinator's box reads %q, want it empty", got)
	}
	w.reads(t, agentdriver.StateFreeText)

	// The line, and the submit key that follows it as a separate write.
	waittest.WaitFor(t, "the wake to reach the box the person emptied", func() bool {
		typed := w.coordPTY.read()
		return strings.Contains(typed, "workers.inbox") && strings.HasSuffix(typed, "\r")
	})
	// EXACTLY ONCE. The whole of what reached that pane is one wake line, which
	// is also what makes the assertion above unambiguous: had the wake typed
	// while the draft was there, the second line would be here.
	typed := w.coordPTY.read()
	if got := strings.Count(typed, "workers.inbox"); got != 1 {
		t.Fatalf("the coordinator was typed at %d times for one batch: %q", got, typed)
	}
	if got := strings.Count(typed, "\r"); got != 1 {
		t.Fatalf("%d submit keys reached the pane for one line: %q", got, typed)
	}
	const want = "nocx: you have 1 new observation of your workers. Call workers.inbox."
	if !strings.Contains(typed, want) {
		t.Fatalf("the coordinator's pane was typed at with %q, want it to contain %q", typed, want)
	}

	// AND A FURTHER SETTLED READING TYPES NOTHING, which is the other half of
	// "exactly once": a box that is now empty again (the submit key started
	// the turn) must not tempt the sweep into a second line for the batch the
	// first one already announced. The line that landed armed the pause, and
	// the pause is two minutes, so this is the mechanism's own answer rather
	// than a race with a clock.
	woken := len(w.coordPTY.read())
	w.reads(t, agentdriver.StateFreeText)
	if got := w.coordPTY.read()[woken:]; got != "" {
		t.Fatalf("a second settled reading typed %q at a coordinator that was already woken", got)
	}
}

// TestACoordinatorWakeIsTypedWhileClaudeDrawsItsOwnSuggestion is the shape the
// owner's stand of 2026-10-18 measured on a real session, minus the part that
// only a recording could settle: the coordinator's box is EMPTY and Claude has
// painted its own prompt suggestion into it, and a worker's mail lands.
//
// The drawing is not text anybody typed (nocx-xn63t.6.32), so the wake must go
// through ONCE rather than waiting for the suggestion to clear — the waiting is
// the defect: the mail stays unread for as long as the drawing stands, which is
// as long as the coordinator is idle.
//
// The frame is the corpus's own (claude-idle at 1470ms, inside the window where
// the paint is on screen and before the same capture erases it at 1505ms), read
// off the live grid rather than assembled here.
func TestACoordinatorWakeIsTypedWhileClaudeDrawsItsOwnSuggestion(t *testing.T) {
	w := newWakeStand(t)
	turn := openCapture(t, "claude-idle")
	turn.to(t, w, 1470, agentdriver.StateFreeText, "Claude's own prompt suggestion to be on screen")
	if got := w.coordBox(t); got != "" {
		t.Fatalf("the coordinator's box reads %q, want it empty: this frame is Claude's own drawing, painted in SGR 2", got)
	}

	p := w.register(t, "reports while the suggestion stands")
	w.reads(t, agentdriver.StateFreeText)
	before := len(w.coordPTY.read())
	w.settledIdle(t, p)

	waittest.WaitFor(t, "the wake to reach the box Claude is drawing in", func() bool {
		typed := w.coordPTY.read()[before:]
		return strings.Contains(typed, "workers.inbox") && strings.HasSuffix(typed, "\r")
	})
	typed := w.coordPTY.read()[before:]
	if got := strings.Count(typed, "workers.inbox"); got != 1 {
		t.Fatalf("the coordinator was typed at %d times for one batch while its suggestion stood: %q", got, typed)
	}
	if got := strings.Count(typed, "\r"); got != 1 {
		t.Fatalf("%d submit keys reached the pane for one line: %q", got, typed)
	}
}

// TestACoordinatorWakeIsNotTypedIntoASolidBoxCarryingTheSuggestionsWords is
// the OTHER direction of the same measurement (nocx-xn63t.6.56), on the same
// recording and the same characters: Claude's suggestion row repainted WITHOUT
// SGR 2 is a person who typed that sentence.
//
// Measured 2026-10-18 by the nocx-xn63t.6.52 worker: the recorded faint drawing
// and the same characters painted solid both read `BoxEmpty TRUE`, because the
// rule recognised the suggestion by its WORDS. The cost of that spelling is the
// sentence below — a person's own `Try "…"` — being submitted with nocx's wake
// in front of it, which is exactly what the box gate exists to prevent.
func TestACoordinatorWakeIsNotTypedIntoASolidBoxCarryingTheSuggestionsWords(t *testing.T) {
	w := newWakeStand(t)
	ctx := context.Background()

	turn := openCapture(t, "claude-idle")
	turn.to(t, w, 1470, agentdriver.StateFreeText, "Claude's own prompt suggestion to be on screen")

	const drawn = `Try "fix typecheck errors"`
	paintCoordinatorPromptRow(t, w, drawn, false)
	if got := w.coordBox(t); got != drawn {
		t.Fatalf("the coordinator's box reads %q, want %q — the same characters without SGR 2 are text in the box", got, drawn)
	}

	p := w.register(t, "reports while a person's sentence stands in the box")
	w.reads(t, agentdriver.StateFreeText)
	before := len(w.coordPTY.read())

	w.settledIdle(t, p)
	// A second settled reading: the sweep retries a refused line on every one,
	// which is the mechanism the wait depends on, so this is a second chance to
	// get it wrong rather than a sleep.
	w.reads(t, agentdriver.StateFreeText)
	if got := w.coordPTY.read()[before:]; got != "" {
		t.Fatalf("nocx typed %q into a box holding a sentence a person typed", got)
	}
	waiting, err := w.workerStore.Since(ctx, workers.ReaderID(w.coordinator), 0, 10)
	if err != nil {
		t.Fatalf("read the mailbox: %v", err)
	}
	if len(waiting) == 0 {
		t.Fatalf("the observation was lost with the refusal, so the coordinator will never hear about it")
	}

	// And with the SAME characters painted FAINT again — Claude's own drawing,
	// on the same row of the same recording — the box reads empty and the
	// waiting wake goes through at once: one line and one submit key, with no
	// wait for the drawing to clear.
	paintCoordinatorPromptRow(t, w, drawn, true)
	if got := w.coordBox(t); got != "" {
		t.Fatalf("the coordinator's box reads %q after the faint repaint, want it empty", got)
	}
	w.reads(t, agentdriver.StateFreeText)
	waittest.WaitFor(t, "the wake to reach the box the faint drawing stands in", func() bool {
		typed := w.coordPTY.read()[before:]
		return strings.Contains(typed, "workers.inbox") && strings.HasSuffix(typed, "\r")
	})
	typed := w.coordPTY.read()[before:]
	if got := strings.Count(typed, "workers.inbox"); got != 1 {
		t.Fatalf("the coordinator was typed at %d times for one batch: %q", got, typed)
	}
	if got := strings.Count(typed, "\r"); got != 1 {
		t.Fatalf("%d submit keys reached the pane for one line: %q", got, typed)
	}
}

// paintCoordinatorPromptRow repaints the coordinator's own prompt row with the
// SAME characters it already carries, drawn faint (SGR 2) or solid, and waits
// for the shipped rule to see a free_text pane again.
func paintCoordinatorPromptRow(t *testing.T, w *wakeStand, drawn string, faint bool) {
	t.Helper()
	f, err := w.grid.Frame(string(w.coordinator))
	if err != nil {
		t.Fatalf("read the coordinator's own frame: %v", err)
	}
	row := -1
	for y := range f.Lines {
		if strings.Contains(f.Text(y), drawn) {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("no row of the coordinator's frame carries %q", drawn)
	}
	paint := "\x1b[" + strconv.Itoa(row+1) + ";1H\u276f\u00a0"
	if faint {
		paint += "\x1b[2m"
	}
	paint += drawn + "\x1b[0m"
	w.grid.Feed(string(w.coordinator), []byte(paint))
	waittest.WaitFor(t, "the coordinator's pane to read free_text after the repaint", func() bool {
		fr, ferr := w.grid.Frame(string(w.coordinator))
		return ferr == nil && w.rules.Classify(wakeAgent, fr) == agentdriver.StateFreeText
	})
}

// coordBox is what the shipped rule reads as typed in the coordinator's own
// input box, off the live grid — never a string this test assembled.
func (w *wakeStand) coordBox(t *testing.T) string {
	t.Helper()
	f, err := w.grid.Frame(string(w.coordinator))
	if err != nil {
		t.Fatalf("read the coordinator's own frame: %v", err)
	}
	text, ok := w.rules.Observe(wakeAgent, f).InputText()
	if !ok {
		t.Fatalf("the shipped rule cannot read the coordinator's input box on this frame")
	}
	return text
}

// feed is ONE corpus replayed forward onto the coordinator's live grid: the
// bytes before mark k are fed once, and the next call continues from there.
// A capture cannot be rewound, only replayed from byte zero, so a test that
// needs two moments of one session must keep the position rather than feed the
// whole stream twice over the top of an emulator that already has it.
type feed struct {
	name   string
	chunks []agentcapture.Chunk
	at     int
}

func openCapture(t *testing.T, name string) *feed {
	t.Helper()
	//nolint:gosec // The path is joined literals naming a corpus in the tree.
	_, chunks, err := agentcapture.Read(
		filepath.Join("..", "agentdriver", "testdata", "captures", name+".jsonl"))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return &feed{name: name, chunks: chunks}
}

// to feeds the corpus forward to atMs and waits for the shipped rule to
// classify the coordinator's pane as want. It waits on the CLASSIFICATION and
// never on a duration.
func (f *feed) to(t *testing.T, w *wakeStand, atMs int64, want agentdriver.State, what string) {
	t.Helper()
	next := agentcapture.ChunksThrough(f.chunks, atMs, f.at)
	for _, c := range f.chunks[f.at:next] {
		w.grid.Feed(string(w.coordinator), c.Data)
	}
	f.at = next
	waittest.WaitFor(t, "the coordinator's pane to reach "+string(want)+" in "+f.name+" ("+what+")", func() bool {
		fr, err := w.grid.Frame(string(w.coordinator))
		return err == nil && w.rules.Classify(wakeAgent, fr) == want
	})
}

// ── the human, two ways ───────────────────────────────────────────────────

// The far end of the wake: what the person is actually told, and that the two
// situations get DIFFERENT sentences.
//
// The escalation is the composition root's adapter — it stamps an event and
// hands it to ingress — so this asserts the stamping. Where an attested event
// may REACH is internal/notify's, enforced default-deny against a routing
// table this test does not own and must not restate.
func TestTheEscalationTellsThePersonWhichSituationThisIs(t *testing.T) {
	raiser := &recordingRaiser{}
	esc := &workerEscalation{raise: raiser}

	t.Run("the coordinator never read its mail", func(t *testing.T) {
		raiser.events = nil
		if err := esc.Escalate(context.Background(), workers.Notice{
			Mailbox: "sess-coordinator", Kind: workers.NoticeUnread, Messages: 3, Observations: 1, Lines: 3,
		}); err != nil {
			t.Fatalf("escalate: %v", err)
		}
		if len(raiser.events) != 1 {
			t.Fatalf("events = %d, want 1", len(raiser.events))
		}
		ev := raiser.events[0]
		if ev.Kind != notify.KindCoordinatorStalled || ev.Trust != notify.TrustAttested {
			t.Fatalf("event = %+v, want an attested coordinator.stalled", ev)
		}
		// The coordinator's pane and not a worker's: the situation is about
		// workers and the decision is the coordinator's, so a notification that
		// opened a worker's pane would show a screen that is not the one which
		// stopped reading.
		if ev.SessionID != "sess-coordinator" {
			t.Fatalf("the notification names session %q, want the coordinator's", ev.SessionID)
		}
		// BOTH COUNTS, because the person is being sent to read and the two
		// kinds are different things to find (nocx-xn63t.6.39): a sentence
		// carrying one number would send them looking for messages among
		// observations.
		if !strings.Contains(ev.Body, "3 unread message") || !strings.Contains(ev.Body, "1 unread state change") ||
			!strings.Contains(ev.Body, "3 line") {
			t.Fatalf("the person is not told what is waiting, in which kinds, or how often nocx tried: %q", ev.Body)
		}
	})

	t.Run("the coordinator is blocked with live workers", func(t *testing.T) {
		raiser.events = nil
		if err := esc.Escalate(context.Background(), workers.Notice{
			Mailbox: "sess-coordinator", Kind: workers.NoticeBlocked, LiveWorkers: 2,
		}); err != nil {
			t.Fatalf("escalate: %v", err)
		}
		body := raiser.events[0].Body
		// A DIFFERENT sentence, because it asks the person for a different
		// thing: nothing is being coordinated because the coordinator is stuck
		// on a screen of its own, and a notice about unread mail would send
		// somebody looking for mail that is not the problem.
		if !strings.Contains(body, "blocked") || !strings.Contains(body, "2 worker") {
			t.Fatalf("the blocked notice does not say what is wrong: %q", body)
		}
		if strings.Contains(body, "unread") {
			t.Fatalf("the blocked notice is worded as an unread one: %q", body)
		}
	})

	t.Run("a kind with no words is refused rather than misworded", func(t *testing.T) {
		raiser.events = nil
		err := esc.Escalate(context.Background(), workers.Notice{
			Mailbox: "sess-coordinator", Kind: workers.NoticeKind("invented"),
		})
		if err == nil {
			t.Fatalf("a notice kind with no vocabulary was raised anyway")
		}
		if len(raiser.events) != 0 {
			t.Fatalf("a notice with the wrong words reached the pipeline: %+v", raiser.events)
		}
	})
}

// FAILURE PATH (acceptance criterion 8): a notice nobody could receive is
// reported as a failure, not as a person told.
//
// This is the state a backend with no renderer attached is in, and it is the
// one path whose whole purpose is to be the last resort: reporting it as
// delivered would leave a worker's mail unread for ever with the log claiming
// somebody had been told.
func TestANoticeThePipelineRefusedIsReportedAsAFailure(t *testing.T) {
	raiser := &recordingRaiser{refuse: errors.New("the router's queue is full")}
	esc := &workerEscalation{raise: raiser}

	err := esc.Escalate(context.Background(), workers.Notice{
		Mailbox: "sess-coordinator", Kind: workers.NoticeUnread, Messages: 0, Observations: 1, Lines: 3,
	})
	if err == nil {
		t.Fatalf("a notice the pipeline refused was reported as delivered")
	}
	if !errors.Is(err, raiser.refuse) {
		t.Fatalf("err = %v, want the pipeline's own refusal", err)
	}

	// And with no pipeline at all it is a failure too, rather than a nil that
	// reads as success.
	unwired := &workerEscalation{}
	if err := unwired.Escalate(context.Background(), workers.Notice{
		Mailbox: "sess-coordinator", Kind: workers.NoticeUnread, Messages: 1, Observations: 0,
	}); err == nil {
		t.Fatalf("an unwired escalation reported success")
	}
}

// A DEAD PANE: the coordinator's session is gone while nocx still holds a
// screen for it.
//
// This is the only refusal where the screen says yes: the frame is still
// free_text, so the gate that stops the other cases lets this one through, and
// what refuses it is the pane's own input queue. Nothing is reported as woken,
// and the mail is still there for whoever reads it next.
func TestACoordinatorWhoseSessionIsGoneIsNotReportedAsWoken(t *testing.T) {
	w := newWakeStand(t)
	w.driveTo(t, 11000, agentdriver.StateFreeText)
	p := w.register(t, "reports into a coordinator that is gone")

	w.reads(t, agentdriver.StateFreeText)

	if err := w.reg.Close(w.coordinator); err != nil {
		t.Fatalf("close the coordinator's session: %v", err)
	}
	waittest.WaitFor(t, "the coordinator's session to leave the registry", func() bool {
		_, err := w.reg.Get(w.coordinator)
		return err != nil
	})

	before := len(w.coordPTY.read())
	w.settledIdle(t, p)
	if got := w.coordPTY.read()[before:]; got != "" {
		t.Fatalf("nocx typed %q into a session that no longer exists", got)
	}
}

// ── the outcome translation ───────────────────────────────────────────────

// fakeTypist answers with one outcome. It is here for the ONE case the real
// gates cannot be driven into from a capture — a paste the pane took and a
// submit key it did not — and that case is the whole reason this translation
// is not a two-line switch nobody read.
type fakeTypist struct{ res agenttyping.Result }

func (f fakeTypist) Submit(ctx context.Context, _, _ string) agenttyping.Result { return f.res }

// ONLY a submission is a delivery.
//
// Text that reached the input region without its submit key starts no turn:
// the coordinator is looking at an unsent line, which is indistinguishable
// from having been told nothing. Calling that delivered is exactly the
// "reported as sent" the record refuses, and it is the one outcome that looks
// like a success from inside the typist.
func TestOnlyASubmittedWakeCountsAsDelivered(t *testing.T) {
	for _, tc := range []struct {
		name      string
		res       agenttyping.Result
		delivered bool
		says      string
	}{
		{
			name:      "submitted",
			res:       agenttyping.Result{Outcome: agenttyping.OutcomeSubmitted},
			delivered: true,
		},
		{
			name: "typed but never submitted",
			res: agenttyping.Result{
				Outcome: agenttyping.OutcomeTyped,
				Reason:  "that pane is not accepting input at the moment, so nothing was written",
			},
			says: "not accepting input",
		},
		{
			name: "refused",
			res: agenttyping.Result{
				Outcome: agenttyping.OutcomeRefused,
				Reason:  "nocx is not watching that pane, so there is no rule to ask about it",
			},
			says: "not watching that pane",
		},
		{
			name: "refused with no sentence",
			res:  agenttyping.Result{Outcome: agenttyping.OutcomeRefused, State: "unknown"},
			says: "unknown",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &workerWaker{typist: fakeTypist{res: tc.res}, log: log.NewSlogAdapter(nil)}
			got := w.Wake(context.Background(), "sess-coordinator", "wake up")
			if got.Delivered != tc.delivered {
				t.Fatalf("delivered = %v, want %v (%+v)", got.Delivered, tc.delivered, got)
			}
			if tc.delivered {
				return
			}
			if got.Reason == "" {
				t.Fatalf("a refusal carries no reason, so the record cannot tell it from a delivery")
			}
			if !strings.Contains(got.Reason, tc.says) {
				t.Fatalf("reason = %q, want it to say %q", got.Reason, tc.says)
			}
		})
	}
}

// A worker whose record holds no coordinator session has nowhere to type, and
// that is a named refusal rather than a write addressed to the empty string —
// which the typist would answer for whichever pane happens to be keyed by it.
func TestAGroupWithNoCoordinatorSessionIsARefusalAndNotAWrite(t *testing.T) {
	w := &workerWaker{
		typist: fakeTypist{res: agenttyping.Result{Outcome: agenttyping.OutcomeSubmitted}},
		log:    log.NewSlogAdapter(nil),
	}
	got := w.Wake(context.Background(), "", "wake up")
	if got.Delivered {
		t.Fatalf("a wake with no coordinator to address was reported as delivered: %+v", got)
	}
	if !strings.Contains(got.Reason, "no coordinator session") {
		t.Fatalf("reason = %q", got.Reason)
	}
}

// NOTHING IN THE PRODUCT RAISES THE HUMAN FROM A PER-FACT DEADLINE.
//
// The mechanism this replaced put every fact under its own deadline, so a
// worker's declaration or exit could reach a person on its own — and two
// mechanisms that can both call the human will. This asserts the deletion at
// the composition level: a coordinator that never reads its workers' mail is
// still merely idle, and no number of facts about its workers moves it.
func TestFactsAloneReachNobody(t *testing.T) {
	w := newWakeStand(t)
	ctx := context.Background()
	w.driveTo(t, 11000, agentdriver.StateFreeText)
	p := w.register(t, "read AGENTS.md and report")

	// The coordinator is idle and settled, so the wake is armed and willing.
	w.reads(t, agentdriver.StateFreeText)

	// The fact that needs judgement now: an end nocx cannot call ordinary —
	// the shell was lost rather than exited — which is what the old mechanism
	// escalated out of band.
	if _, err := w.record.Exited(ctx, p.ID, p.Liveness,
		workers.Exit{Cause: string(session.ExitInterrupted)}); err != nil {
		t.Fatalf("exit: %v", err)
	}
	if got := len(w.record.Undispatched()); got != 1 {
		t.Fatalf("undispatched = %d, want the fact recorded", got)
	}
	if got := len(w.raiser.events); got != 0 {
		t.Fatalf("a fact alone reached the human: %+v", got)
	}
	// WHAT IS TYPED, IF ANYTHING, IS THE WAKE'S OWN POINTER LINE — the count of
	// the coordinator's unread mail and nothing else. That is the mechanism
	// that replaced the deadline, and the distinction this test exists for is
	// that no worker's words and no notice about a person are in it.
	if got := w.coordPTY.read(); got != "" && !strings.Contains(got, "workers.inbox") {
		t.Fatalf("a fact produced a line that is not the wake's pointer: %q", got)
	}
}

// A coordinator whose pane nocx never watched is the honest refusal at the
// other end: there is no rule to ask about that pane, so nothing is typed into
// it, and the mail is still there.
func TestAGroupWhoseCoordinatorPaneIsNotWatchedIsNotTyped(t *testing.T) {
	w := newWakeStand(t)
	w.driveTo(t, 11000, agentdriver.StateFreeText)
	p := w.register(t, "reports into an unwatched coordinator")

	// A settled idle reading first, so the record knows it has a coordinator —
	// and then the person closed the agent in that pane, which ends the
	// observation.
	w.reads(t, agentdriver.StateFreeText)
	w.grid.Withdraw(string(w.coordinator))

	before := len(w.coordPTY.read())
	w.settledIdle(t, p)
	if got := w.coordPTY.read()[before:]; got != "" {
		t.Fatalf("nocx typed %q into a pane it has no live screen for", got)
	}
	box := workers.ReaderID(w.coordinator)
	fetched, err := w.record.Inbox(context.Background(), box, box, 0)
	if err != nil {
		t.Fatalf("read the mailbox: %v", err)
	}
	if len(fetched.Messages) != 1 {
		t.Fatalf("the observation was lost with the refusal: %+v", fetched.Messages)
	}
}

// ── ending, at the composition level ──────────────────────────────────────

// A coordinator ends every worker it names, through the real record, the real
// closer and the real layout chain — and the record says WHY each one ended.
func TestACloseEndsEveryWorkerTheCoordinatorNames(t *testing.T) {
	ctx := context.Background()
	w := newWakeStand(t)
	w.driveTo(t, 11000, agentdriver.StateFreeText)

	participants := []workers.Participant{
		w.register(t, "read AGENTS.md"),
		w.register(t, "read the architecture"),
		w.register(t, "read the vision"),
	}

	for _, p := range participants {
		if _, err := w.record.Close(ctx, string(w.coordinator), p.ID); err != nil {
			t.Fatalf("close %s: %v", p.ID, err)
		}
		waittest.WaitFor(t, "the closed worker's end to reach the record", func() bool {
			stored, err := w.workerStore.Participant(ctx, p.ID)
			return err == nil && stored.State == workers.StateClosed
		})
	}
}

// A coordinator cannot close somebody else's worker, and the refusal comes
// from the DELEGATION rather than from anything the caller said.
func TestACoordinatorCannotCloseAWorkerItDoesNotHold(t *testing.T) {
	ctx := context.Background()
	w := newWakeStand(t)
	w.driveTo(t, 11000, agentdriver.StateFreeText)
	p := w.register(t, "belongs to this coordinator")

	if _, err := w.record.Close(ctx, "sess-somebody-else", p.ID); !errors.Is(err, workers.ErrNotHeld) {
		t.Fatalf("close by a stranger = %v, want ErrNotHeld", err)
	}
	if _, err := w.reg.Get(session.ID(p.Liveness.SessionID)); err != nil {
		t.Fatalf("a refused close ended the worker's session anyway: %v", err)
	}
}
