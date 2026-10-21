package session

// What the pty RECEIVES for a Cyrillic keystroke, on the pane's own encoding
// stack (nocx-1ebrs.38): a hostSession over a REAL pty, an interactive text
// intent through the same path Service answers the pane with, and the bytes
// the program actually read off its stdin. A TUI echoes what it receives, so
// this file is the truth the doubling question is decided against: one intent
// is one copy of the bytes, two intents are two — the runtime cannot invent a
// copy and cannot lose one. A doubling has to arrive as a changed intent
// count on this path, which is why these two cases are written down together.
//
// The payload is one Cyrillic letter — two UTF-8 bytes — because the field
// report is Cyrillic text and because the multi-byte shape is its ordinary
// case, not an edge.

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shady2k/nocx/internal/helper/proto"
	"github.com/shady2k/nocx/internal/log/logtest"
	"github.com/shady2k/nocx/internal/pty"
	"github.com/shady2k/nocx/internal/sessionruntime"
)

// startReaderProgram runs a real shell on a real pty whose whole job is to
// read want bytes from its stdin and leave their hex dump in a file the test
// reads. stty turns the line discipline off first, so the bytes the pane sent
// are the bytes dd reads — no echo, no canonical buffering, no NL.
func startReaderProgram(t *testing.T, want int) (*pty.LocalPty, string) {
	t.Helper()
	dump := filepath.Join(t.TempDir(), "received.hex")
	script := "stty -icanon -echo min 1 time 0\ndd bs=1 count=" + strconv.Itoa(want) +
		" 2>/dev/null | od -An -tx1 > '" + dump + "'\n"
	_, lg := logtest.New(t)
	lp, err := pty.NewLocal(lg, pty.Config{
		Command: "/bin/sh",
		Args:    []string{"-c", script},
		Cols:    80,
		Rows:    24,
	})
	if err != nil {
		t.Fatalf("start a real shell on a real pty: %v", err)
	}
	t.Cleanup(func() { _ = lp.Close() })
	return lp, dump
}

// receivedAfterExit waits for the program to exit and returns what it
// dumped: the bytes it read off the pty, lower-cased, without od's spacing.
func receivedAfterExit(t *testing.T, lp *pty.LocalPty, dump string) string {
	t.Helper()
	select {
	case <-lp.Done():
	case <-time.After(30 * time.Second):
		t.Fatal("the reader program never finished reading its stdin")
	}
	raw, err := os.ReadFile(dump) //nolint:gosec // a path this test just created under t.TempDir()
	if err != nil {
		t.Fatalf("read the dump: %v", err)
	}
	return strings.ToLower(strings.Join(strings.Fields(string(raw)), " "))
}

// sendText sends one interactive text intent the way the pane's
// session.intent does, and fails when it is not executed.
func sendText(t *testing.T, hs *hostSession, epoch sessionruntime.ControlEpoch, text string) {
	t.Helper()
	got, err := hs.intent(context.Background(), proto.IntentParams{
		Interactive: true,
		AccessEpoch: uint64(epoch),
		CommitBy:    math.MaxInt64,
		Kind:        "text",
		Payload:     []byte(text),
	})
	if err != nil {
		t.Fatalf("intent: %v", err)
	}
	if got.State != "executed" {
		t.Fatalf("text intent = state=%s refusal=%+v, want executed", got.State, got.Refusal)
	}
}

// TestWhatThePtyReceivesForOneCyrillicTextIntent is the measurement the
// doubling was decided with: ONE text intent puts the letter's two UTF-8
// bytes on the pty exactly once.
func TestWhatThePtyReceivesForOneCyrillicTextIntent(t *testing.T) {
	lp, dump := startReaderProgram(t, 2)
	hs, ctrl := newIntentTestSession(t, lp)

	sendText(t, hs, ctrl.Epoch, "\u0444")

	if got, want := receivedAfterExit(t, lp, dump), "d1 84"; got != want {
		t.Fatalf("the pty received %q, want %q (one letter, once)", got, want)
	}
}

// TestWhatThePtyReceivesForTwoIdenticalTextIntents is the other half of the
// truth: the double the TUI showed is two intents, not one — the pty mirrors
// the intent count exactly, so the wire is where a doubling is born or
// disproved.
func TestWhatThePtyReceivesForTwoIdenticalTextIntents(t *testing.T) {
	lp, dump := startReaderProgram(t, 4)
	hs, ctrl := newIntentTestSession(t, lp)

	sendText(t, hs, ctrl.Epoch, "\u0444")
	sendText(t, hs, ctrl.Epoch, "\u0444")

	if got, want := receivedAfterExit(t, lp, dump), "d1 84 d1 84"; got != want {
		t.Fatalf("the pty received %q, want %q (the letter twice, from two intents)", got, want)
	}
}
