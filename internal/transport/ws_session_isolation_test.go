package transport

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shady2k/nocx/internal/helper/proto"
	"github.com/shady2k/nocx/internal/session"
)

// Only the helper boundary is blocked: socket ingress, ownership checks and
// actual JSON-RPC responses remain the production path.
type blockingPaneIntentSource struct {
	started       chan struct{}
	release       chan struct{}
	once          sync.Once
	secondStarted chan struct{}
}

func (s *blockingPaneIntentSource) Intent(ctx context.Context, _ string, p proto.IntentParams) (proto.IntentResult, error) {
	if string(p.Payload) == "second" && s.secondStarted != nil {
		close(s.secondStarted)
	}
	if string(p.Payload) == "block" {
		s.once.Do(func() { close(s.started) })
		select {
		case <-s.release:
		case <-ctx.Done():
			return proto.IntentResult{}, ctx.Err()
		}
	}
	return proto.IntentResult{State: "executed", BytesWritten: len(p.Payload)}, nil
}

func (*blockingPaneIntentSource) AccessEpoch(context.Context, string) (uint64, error) {
	return 1, nil
}

func sendSessionIntentRaw(t *testing.T, conn *websocket.Conn, sid, payload string, id int) {
	t.Helper()
	raw := mustMarshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": "session.intent",
		"params": map[string]any{"sessionId": sid, "accessEpoch": 1, "kind": "text", "payload": []byte(payload)},
	})
	if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Fatalf("write session.intent: %v", err)
	}
}

// This regression deliberately uses no keyed-scheduler API: on the old global
// completion FIFO, B cannot answer until the test releases A, on either socket.
func TestBlockedSessionIntentDoesNotStallOtherSessionsOverTheWire(t *testing.T) {
	for _, separateSocket := range []bool{false, true} {
		name := "same_socket"
		if separateSocket {
			name = "different_socket"
		}
		t.Run(name, func(t *testing.T) {
			source := &blockingPaneIntentSource{started: make(chan struct{}), release: make(chan struct{})}
			ws, _, _ := newPanesWS(t, WithPaneIntentSource(source))
			// Release before the fixture stops the server, including failure paths.
			t.Cleanup(func() { close(source.release) })
			ownerA := connectWS(t, ws)
			sidA := openSessionOnConn(t, ws, ownerA, 1)
			ownerB := ownerA
			if separateSocket {
				ownerB = connectWS(t, ws)
			}
			sidB := openSessionOnConn(t, ws, ownerB, 2)
			sendSessionIntentRaw(t, ownerA, sidA, "block", 10)
			select {
			case <-source.started:
			case <-time.After(5 * time.Second):
				t.Fatal("A intent never reached helper")
			}
			sendSessionIntentRaw(t, ownerB, sidB, "B", 20)
			sendResizeRaw(t, ownerB, sidB, 100, 30, 21)
			responses := collectResponses(t, ownerB, []int{20, 21}, 5*time.Second)
			for _, id := range []int{20, 21} {
				assertResponseOK(t, responses[id], id)
			}
			var result struct {
				Result sessionIntentResult `json:"result"`
			}
			if err := json.Unmarshal(responses[20], &result); err != nil || result.Result.State != "executed" || result.Result.BytesWritten != 1 {
				t.Fatalf("B intent did not execute: %s (decode error %v)", responses[20], err)
			}
			select {
			case <-source.release:
				t.Fatal("test released A before B responded")
			default:
			}
		})
	}
}

func TestSessionOperationsKeepFIFOAcrossOwnershipTransfer(t *testing.T) {
	for _, terminalMethod := range []string{"close", "detach"} {
		t.Run(terminalMethod, func(t *testing.T) {
			source := &blockingPaneIntentSource{
				started: make(chan struct{}), release: make(chan struct{}), secondStarted: make(chan struct{}),
			}
			ws, _, _ := newPanesWS(t, WithPaneIntentSource(source))
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(source.release) }) }
			t.Cleanup(release)
			oldOwner := connectWS(t, ws)
			sidA := openSessionOnConn(t, ws, oldOwner, 1)
			newOwner := connectWS(t, ws)
			sidB := openSessionOnConn(t, ws, newOwner, 2)
			sess, err := ws.registry.Get(session.ID(sidA))
			if err != nil {
				t.Fatalf("look up A: %v", err)
			}
			identity := sess.Identity()
			sendSessionIntentRaw(t, oldOwner, sidA, "block", 10)
			select {
			case <-source.started:
			case <-time.After(5 * time.Second):
				t.Fatal("first A intent never reached helper")
			}
			// Attach is an ownership transfer, not an ordered input operation.
			// A request already executing keeps its place across that transfer.
			raw := jsonrpcCallWithID(t, newOwner, "attach", map[string]any{
				"sessionId": sidA, "instanceId": identity.InstanceID, "sessionEpoch": identity.Epoch, "offset": 0,
			}, 3)
			assertResponseOK(t, raw, 3)
			sendSessionIntentRaw(t, newOwner, sidA, "second", 11)
			sendResizeRaw(t, newOwner, sidA, 100, 30, 12)
			if err := newOwner.WriteMessage(websocket.TextMessage, mustMarshal(map[string]any{
				"jsonrpc": "2.0", "id": 13, "method": terminalMethod, "params": map[string]any{"sessionId": sidA},
			})); err != nil {
				t.Fatalf("write %s: %v", terminalMethod, err)
			}
			sendSessionIntentRaw(t, newOwner, sidA, "after-terminal", 14)
			sendSessionIntentRaw(t, newOwner, sidB, "B", 20)
			assertResponseOK(t, collectResponses(t, newOwner, []int{20}, 5*time.Second)[20], 20)
			select {
			case <-source.secondStarted:
				t.Fatal("new owner's A intent overtook old owner's blocked intent")
			default:
			}
			release()
			assertResponseOK(t, collectResponses(t, oldOwner, []int{10}, 5*time.Second)[10], 10)
			responses := collectResponses(t, newOwner, []int{11, 12, 13, 14}, 5*time.Second)
			assertResponseOK(t, responses[11], 11)
			assertResponseOK(t, responses[13], 13)
			assertResponseError(t, responses[14], 14, -32602)
			// Resize's receipt is application, not enqueue: the subsequent
			// terminal operation may cancel its asynchronous application.
			var resize struct {
				Error *RPCError `json:"error"`
			}
			if err := json.Unmarshal(responses[12], &resize); err != nil {
				t.Fatalf("decode resize: %v", err)
			}
			if resize.Error != nil && resize.Error.Code != resizeNotAppliedCode {
				t.Fatalf("resize failed outside its cancellation contract: %s", responses[12])
			}
		})
	}
}

func TestSessionOrderingSaturationRemainsNonblockingAndValidatesBeforeAdmission(t *testing.T) {
	source := &blockingPaneIntentSource{started: make(chan struct{}), release: make(chan struct{})}
	ws, _, _ := newPanesWS(t, WithPaneIntentSource(source))
	t.Cleanup(func() { close(source.release) })
	conn := connectWS(t, ws)
	sidA := openSessionOnConn(t, ws, conn, 1)
	sidB := openSessionOnConn(t, ws, conn, 2)
	sendSessionIntentRaw(t, conn, sidA, "block", 10)
	select {
	case <-source.started:
	case <-time.After(5 * time.Second):
		t.Fatal("A intent never reached helper")
	}
	// One running task plus 31 queued tasks consumes the entire server-wide
	// bound. Different keys do not multiply the admitted-work allowance.
	for id := 100; id < 131; id++ {
		sendSessionIntentRaw(t, conn, sidA, "queued", id)
	}
	sendSessionIntentRaw(t, conn, sidB, "overflow", 200)
	sendResizeRaw(t, conn, "", 100, 30, 201)
	responses := collectResponses(t, conn, []int{200, 201}, 5*time.Second)
	assertResponseError(t, responses[200], 200, -32004)
	assertResponseError(t, responses[201], 201, -32602)
}
