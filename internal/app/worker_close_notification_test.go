package app

// A close the coordinator asked for raises no session-ended notification
// (nocx-xn63t.6.38).
//
// WHAT THE OWNER SAW (2026-10-17, dev stand): the coordinator closed its
// finished worker with workers.close. The worker's tab closed cleanly, and a
// warning toast appeared anyway — "Local session ended" with the body "exit
// status -1", which is not an exit status a person can read.
//
// THE RAISE IS SOURCE CORRECTNESS, NOT FILTERING. Every event that reaches
// the feed is a member of it and policy decides only its channels, so an end
// nobody needs to be told about must not become an event in the first place.
// The transport already decides that for the two ends IT asks for — the
// user's own close, and a pane that has left the layout — by recording the
// ask before the registry close, which is what monitorExit reads to stay
// quiet (nocx-xn63t.6.33's sibling, ws_notify_session_ended_test.go). A close
// the composition root performs reaches the REGISTRY directly
// (internal/app's workerCloser ends a participant by ending its session), so
// it was the third ask and the only one nobody had recorded.
//
// THE TWO TESTS BELOW ARE THE INTERVAL'S TWO ENDS, which is why they are a
// pair: a suppression keyed by "somebody asked" is only correct while the end
// nobody asked for is unchanged — and that is the case the bell exists for.

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/shady2k/nocx/internal/notify"
	"github.com/shady2k/nocx/internal/session"
	"github.com/shady2k/nocx/internal/waittest"
)

// recordedRaises is the notify pipeline's narrow seam
// (transport.NotifyRaiser), recording what monitorExit raised. It records
// rather than delivers because what these tests assert is MEMBERSHIP — was an
// event raised at all — and the channels a member then takes belong to the
// router's own suite.
type recordedRaises struct {
	mu     sync.Mutex
	events []notify.Event
}

func (r *recordedRaises) Raise(_ context.Context, ev notify.Event) notify.Outcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return notify.Outcome{}
}

// sessionEnded is what was raised about this session, and nothing else: a
// stand test that counted every event would fail the day an unrelated source
// joins the feed.
func (r *recordedRaises) sessionEnded(sid string) []notify.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []notify.Event
	for _, ev := range r.events {
		if ev.Kind == notify.KindSessionEnded && ev.SessionID == sid {
			out = append(out, ev)
		}
	}
	return out
}

// --- the renderer's side of the same connection -------------------------

// wireFrame is one frame off one renderer connection, in the two shapes that
// matter here: a response (an id) and a notification (a method).
type wireFrame struct {
	id     *int
	method string
	params json.RawMessage
	raw    []byte
}

// wireFrames is the SINGLE reader on one renderer connection. One reader and
// no read deadline, for the reason workerNotifications' own doc gives: a
// gorilla websocket that has failed a read stays failed, so a helper that
// polled the socket with a timeout would turn "the frame has not arrived yet"
// into a crash.
type wireFrames struct {
	ch chan wireFrame
}

func readWireFrames(conn *websocket.Conn) *wireFrames {
	f := &wireFrames{ch: make(chan wireFrame, 256)}
	go func() {
		defer close(f.ch)
		for {
			typ, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if typ != websocket.TextMessage {
				continue
			}
			var env struct {
				ID     *int            `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if json.Unmarshal(msg, &env) != nil {
				continue
			}
			f.ch <- wireFrame{id: env.ID, method: env.Method, params: env.Params, raw: msg}
		}
	}()
	return f
}

// await waits for the first frame the predicate accepts, as an observable
// state and never as a duration: the frame either arrived or the wait reports
// what never came.
func (f *wireFrames) await(t *testing.T, what string, want func(wireFrame) bool) wireFrame {
	t.Helper()
	var got wireFrame
	waittest.WaitFor(t, what, func() bool {
		select {
		case fr, ok := <-f.ch:
			if !ok {
				return false
			}
			if want(fr) {
				got = fr
				return true
			}
			return false
		default:
			return false
		}
	})
	return got
}

// exitFrame is the session's own end notification — the LAST thing monitorExit
// does, after the raise decision above it. Reading it is therefore how a test
// that asserts "nothing was raised" orders that read past the decision
// instead of racing it.
func (f *wireFrames) exitFrame(t *testing.T, sid string) wireFrame {
	t.Helper()
	fr := f.await(t, "the exit notification for the closed worker", func(fr wireFrame) bool {
		if fr.method != "exit" {
			return false
		}
		var p struct {
			SessionID string `json:"sessionId"`
		}
		return json.Unmarshal(fr.params, &p) == nil && p.SessionID == sid
	})
	return fr
}

// attachToSession attaches this connection to the participant's session over
// the wire, the way a renderer's reconnect does — because the exit
// notification is addressed to the session's SUBSCRIBER, and a connection with
// no claim on the session would never see the frame that orders these tests.
func attachToSession(t *testing.T, conn *websocket.Conn, frames *wireFrames, sid string) {
	t.Helper()
	req, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "attach",
		"params":  map[string]any{"sessionId": sid, "offset": 0},
	})
	if err != nil {
		t.Fatalf("marshal attach: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, req); err != nil {
		t.Fatalf("write attach: %v", err)
	}
	resp := frames.await(t, "the attach ack", func(fr wireFrame) bool {
		return fr.id != nil && *fr.id == 1
	})
	if containsRefusal(string(resp.raw)) {
		t.Fatalf("attach was refused: %s", resp.raw)
	}
}

// THE CRITERION. A worker closed through workers.close files nothing: the tab
// closes, the renderer still learns the session is gone, and the centre stays
// quiet — because the person reading it is the one who asked.
func TestClosingAWorkerRaisesNoSessionEndedWarning(t *testing.T) {
	stand := newWorkerStand(t)
	stand.serve(t)

	p := stand.registerWithEnrolment(t, "read AGENTS.md and report")
	sid := p.Liveness.SessionID
	ctx := context.Background()

	conn := workerTestDial(t, stand.tp)
	frames := readWireFrames(conn)
	attachToSession(t, conn, frames, sid)

	// THE WORKER IS STILL RUNNING when the close arrives, which is the case
	// the owner hit: the coordinator judged the worker finished, but the
	// agent's own TUI process was live in its pane, so the close ended a live
	// session — and "-1" is what a killed process reports, not what a program
	// that had finished on its own reports.
	if _, err := stand.record.Close(ctx, "sess-coordinator", p.ID); err != nil {
		t.Fatalf("workers.close: %v", err)
	}

	// The exit notification arrives, and it still says what it always said: a
	// forced teardown leaves no shell report, so the cause is a loss. The
	// renderer's half of the close is unaffected by this bead.
	ex := frames.exitFrame(t, sid)
	var exit struct {
		SessionID string `json:"sessionId"`
		Cause     string `json:"cause"`
	}
	if err := json.Unmarshal(ex.params, &exit); err != nil {
		t.Fatalf("decode the exit notification: %v\nraw: %s", err, ex.params)
	}
	if exit.Cause != string(session.ExitInterrupted) {
		t.Errorf("cause = %q, want %q — a teardown has no shell report", exit.Cause, session.ExitInterrupted)
	}

	// Read AFTER the exit frame, which monitorExit sends after its raise
	// decision: this is therefore the decision's own outcome and not a race
	// with it.
	if evs := stand.raises.sessionEnded(sid); len(evs) != 0 {
		t.Fatalf("workers.close raised %d session-ended event(s) for a close the coordinator asked for, want 0: %+v",
			len(evs), evs)
	}
}

// The OTHER END of the interval, and the case the warning exists for: a
// worker whose process dies on its own, with no close anywhere in the story.
// If this one goes quiet, the bell has been silenced rather than corrected.
func TestAWorkerWhoseProcessDiesOnItsOwnStillRaisesTheWarning(t *testing.T) {
	stand := newWorkerStand(t)

	p := stand.registerWithEnrolment(t, "read AGENTS.md and report")
	sid := p.Liveness.SessionID

	// The participant's program ends by itself — no close, no RPC, nothing
	// asked for. The pane's own pty is what a finished program looks like
	// from the layer above (the stand opens exactly one session, so this is
	// the participant's).
	pty := stand.ptys.last()
	if pty == nil {
		t.Fatal("no pty was recorded for the participant's session")
	}
	if err := pty.Close(); err != nil {
		t.Fatalf("the participant's program ends: %v", err)
	}

	waittest.WaitFor(t, "the session.ended raise for the worker that died on its own", func() bool {
		return len(stand.raises.sessionEnded(sid)) == 1
	})
	ev := stand.raises.sessionEnded(sid)[0]
	if ev.Level != notify.LevelWarning {
		t.Errorf("Level = %q, want %q — nobody was told a worker died", ev.Level, notify.LevelWarning)
	}
	if ev.Title != "Local session was interrupted" {
		t.Errorf("Title = %q, want the local loss wording", ev.Title)
	}
}
