package transport

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/shady2k/nocx/internal/content"
)

// The recording's answer to "what does this session's output no longer hold"
// is pushed when it changes (nocx-zg3k3.14.24).
//
// WHY THIS IS A WIRE TEST AND NOT A UNIT ONE. The defect it closes was not a
// wrong number — the ledger's arithmetic was right at every instant it was
// asked. It was that the client was told the answer ONCE, at reclaim, while
// the recorder went on evicting behind it, so the card and a fresh
// session.recoveryStatus disagreed about the same session (3.1 MB against
// 3.6 MB on PR #380's CI). Only a real socket, a real recorder and a real
// retention bound can show that the client now hears each revision.

// tapNotification returns the params of the next notification carrying method.
// The tap owns the socket (one concurrent reader), so this reads the channel it
// fills rather than the connection.
func tapNotification(t *testing.T, tap *socketTap, method string) json.RawMessage {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case msg, ok := <-tap.msgs:
			if !ok {
				t.Fatalf("socket closed before %s%s", method, socketClosedWhy)
			}
			var env struct {
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.Unmarshal(msg, &env); err != nil {
				continue
			}
			if env.Method == method {
				return env.Params
			}
		case <-deadline:
			t.Fatalf("no %s notification within 15s", method)
		}
	}
}

// TestTheRecordingRevisionsReachTheAttachedClient: an attached pane is told
// the recording's revised answer while it stays attached, and what it is told
// is what session.recoveryStatus answers — one fact, two doors.
func TestTheRecordingRevisionsReachTheAttachedClient(t *testing.T) {
	const capBytes = 512
	db := openRecoveryStatusRecordingStore(t, capBytes)
	term := newFeedablePTY()
	ws, stop := newRecordingWSServer(t, term, WithSessionOutputRecorder(db.SessionOutput()))
	defer stop()
	conn := connectWS(t, ws)
	tap := newSocketTap(conn)
	sid := openInPaneTapped(t, ws, conn, tap, reclaimPane, 1).SessionID

	// The program prints past the retention bound. The recording evicts its
	// oldest bytes to stay inside it — the very fact the card's number is
	// made of, and one no reader of the socket has been told before.
	term.emit(t, string(recordingStream(4096)))

	revision := tapNotification(t, tap, "session.recoveryChanged")
	validateJSON(t, loadSchema(t, "session.recoveryChanged.schema.json"), revision,
		"session.recoveryChanged params off the real socket")

	var got struct {
		SessionID string        `json:"sessionId"`
		Produced  uint64        `json:"produced"`
		Gaps      []content.Gap `json:"gaps"`
	}
	if err := json.Unmarshal(revision, &got); err != nil {
		t.Fatalf("decode revision: %v", err)
	}
	if got.SessionID != sid {
		t.Fatalf("revision names session %q, want %q", got.SessionID, sid)
	}

	live, _ := liveSessions(t, conn, tap, 2)
	entry := entryFor(t, live, sid)
	result, rpcErr := readRecoveryStatus(t, conn, tap, 3, map[string]any{
		"sessionId": sid, "instanceId": entry.InstanceID, "sessionEpoch": entry.SessionEpoch,
	})
	if rpcErr != nil {
		t.Fatalf("session.recoveryStatus: %+v", rpcErr)
	}
	var ledger struct {
		Produced uint64        `json:"produced"`
		Gaps     []content.Gap `json:"gaps"`
	}
	if err := json.Unmarshal(result, &ledger); err != nil {
		t.Fatalf("decode ledger: %v", err)
	}

	// The revision is the ledger's own answer, not a second opinion about it.
	// This is the whole acceptance criterion in one comparison: the number the
	// card is built from and the number the ledger states are one fact.
	if got.Produced != ledger.Produced {
		t.Fatalf("revision produced = %d, ledger produced = %d", got.Produced, ledger.Produced)
	}
	if len(got.Gaps) == 0 || len(ledger.Gaps) == 0 {
		t.Fatalf("no hole to report: revision %+v, ledger %+v", got.Gaps, ledger.Gaps)
	}
	revised, _ := json.Marshal(got.Gaps)
	stated, _ := json.Marshal(ledger.Gaps)
	if string(revised) != string(stated) {
		t.Fatalf("revision gaps = %s, ledger gaps = %s", revised, stated)
	}
}

// TestSessionRecoveryChangedDTOConformsToContract holds the marshalled DTO to
// the notification's own schema. The body is sessionRecoveryStatusResult —
// declared once, on purpose — so this also proves the two contracts describe
// the same shape rather than two shapes that happen to agree today.
func TestSessionRecoveryChangedDTOConformsToContract(t *testing.T) {
	schema := loadSchema(t, "session.recoveryChanged.schema.json")
	raw, err := json.Marshal(sessionRecoveryRevision{
		SessionID: "0123456789abcdef0123456789abcdef",
		Produced:  4096,
		Gaps: []content.Gap{
			{Start: 0, End: 1024, Reason: content.GapReasonCap},
			{Start: 2048, End: 3072, Reason: content.GapReasonHostWindow},
		},
	})
	if err != nil {
		t.Fatalf("marshal revision: %v", err)
	}
	validateJSON(t, schema, raw, "session.recoveryChanged DTO")

	empty, err := json.Marshal(sessionRecoveryRevision{
		SessionID: "0123456789abcdef0123456789abcdef",
		Produced:  0,
		Gaps:      []content.Gap{},
	})
	if err != nil {
		t.Fatalf("marshal empty revision: %v", err)
	}
	validateJSON(t, schema, empty, "session.recoveryChanged DTO with no hole")
}
