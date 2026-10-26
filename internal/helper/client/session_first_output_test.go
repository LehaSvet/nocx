package client_test

import (
	"bytes"
	"sync/atomic"
	"testing"

	"github.com/shady2k/nocx/internal/helper/host"
	"github.com/shady2k/nocx/internal/helper/proto"
)

func TestFirstOutputObservesOnlyTheFirstNonemptyPtyRead(t *testing.T) {
	payload := []byte{'a', 0, 0x1b, '[', 'm', 0xff, '\n'}
	lifecyclePayload := []byte("lifecycle only")
	var sendFrames func() error
	attached, svc := attachOverAWithheldAck(t, func(h *host.Host, session, subscriber [16]byte) error {
		// Keep delivery under the test's control so the observer is present
		// when bytes arrive, but no PTY reader has started yet.
		sendFrames = func() error {
			for _, stream := range []string{"lifecycle", ""} {
				if err := h.SendNotification(proto.Notification{
					Service: proto.ServiceSession, Event: proto.EventSessionReset,
					Params: proto.SessionReset{
						Subscriber: "0123456789abcdef0123456789abcdef",
						Session: proto.HostSessionID{
							Generation: "g", Session: proto.SessionHex(session),
						},
						Resume: proto.Resume{
							Reset: true, From: 0,
							Gap: &proto.Gap{Start: 0, End: 1, Reason: proto.GapReasonWindow},
						},
						Stream: stream,
					},
				}); err != nil {
					return err
				}
			}
			if err := h.SendSessionData(proto.SessionFrame{
				Session: session, Subscriber: subscriber,
			}); err != nil {
				return err
			}
			if err := h.SendSessionData(proto.SessionFrame{
				Session: session, Subscriber: subscriber, Payload: payload,
			}); err != nil {
				return err
			}
			// This last frame is also a wire fence: reading it proves all PTY
			// frames were delivered, without reading any of their bytes yet.
			return h.SendLifecycleData(proto.SessionFrame{
				Session: session, Subscriber: subscriber, Payload: lifecyclePayload,
			})
		}
		return nil
	})

	var calls atomic.Int32
	attached.OnFirstOutput(func() {
		calls.Add(1)
		// Registration takes the attachment mutex too. This must complete
		// before Read returns, and registration cannot rearm the observation.
		attached.OnFirstOutput(func() { calls.Add(1) })
	})
	if err := sendFrames(); err != nil {
		t.Fatalf("send frames: %v", err)
	}

	assertNoOutput := func(stage string) {
		t.Helper()
		if got := calls.Load(); got != 0 {
			t.Fatalf("first output fired %d times after %s, before a nonempty PTY read", got, stage)
		}
	}
	assertEmptyRead := func(read func([]byte) (int, error), buf []byte, stage string) {
		t.Helper()
		if n, err := read(buf); n != 0 || err != nil {
			t.Fatalf("%s read = %d, %v, want 0, nil", stage, n, err)
		}
		assertNoOutput(stage)
	}
	assertNoOutput("delivery")
	assertEmptyRead(attached.Read, nil, "zero-length buffer")

	lifecycle := attached.Lifecycle()
	t.Cleanup(func() { _ = lifecycle.Close() })
	buf := make([]byte, 64)
	assertEmptyRead(lifecycle.Read, buf, "lifecycle reset")
	assertBytesArriveWhileTheAckIsUnanswered(t, svc, lifecycle.Read, lifecyclePayload, lifecycleCursor)
	assertNoOutput("nonempty lifecycle read")
	assertEmptyRead(attached.Read, buf, "PTY reset")
	assertEmptyRead(attached.Read, buf, "empty PTY payload")

	// Split one payload across reads. The first read observes output before
	// returning, without holding either the bytes or the callback behind ack.
	readFirst := func(p []byte) (int, error) { return attached.Read(p[:2]) }
	assertBytesArriveWhileTheAckIsUnanswered(t, svc, readFirst, payload[:2], ptyCursor)
	if got := calls.Load(); got != 1 {
		t.Fatalf("first nonempty PTY read returned after %d observations, want 1", got)
	}
	n, err := attached.Read(buf)
	if err != nil || !bytes.Equal(buf[:n], payload[2:]) {
		t.Fatalf("remaining PTY read = %q, %v, want unchanged %q", buf[:n], err, payload[2:])
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("later PTY read fired the observer again: got %d calls, want 1", got)
	}
}
