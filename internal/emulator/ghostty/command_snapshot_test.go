package ghostty

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/shady2k/nocx/internal/emulator"
)

func TestCommandSnapshotPayloadParser(t *testing.T) {
	msg, ok := parseCommandSnapshotPayload([]byte(`S;0123456789abcdef0123456789abcdef;alias;semi\x3bcolon;back\\slash;café;`))
	if !ok {
		t.Fatal("well-formed shell snapshot was rejected")
	}
	want := []string{"alias", "semi;colon", "back\\slash", "café"}
	if !reflect.DeepEqual(msg.names, want) {
		t.Fatalf("decoded names = %#v, want %#v", msg.names, want)
	}
}

func TestCommandSnapshotScannerAcceptsOnePayload(t *testing.T) {
	const nonce = "0123456789abcdef0123456789abcdef"
	term := &terminal{}
	for _, b := range []byte("\x1b]636;H;" + nonce + "\a") {
		term.scanCommandSnapshotByte(b)
	}
	for _, b := range []byte("\x1b]636;S;" + nonce + ";alias;function\a") {
		term.scanCommandSnapshotByte(b)
	}
	if !reflect.DeepEqual(term.commandSnapshotPending, []string{"alias", "function"}) {
		t.Fatalf("scanner pending names = %#v, nonce=%q", term.commandSnapshotPending, term.commandSnapshotNonce)
	}
}

func TestCommandSnapshotRequiresTheSessionHelloAndDecodesShellEscapes(t *testing.T) {
	term := newTerminal(t, 20, 4)
	const nonce = "0123456789abcdef0123456789abcdef"
	// A snapshot before the hello is untrusted and must not establish state.
	ingest(t, term, "\x1b]636;S;"+nonce+";forged\a")
	if got := term.Effects(); len(got) != 0 {
		t.Fatalf("snapshot before hello produced effects %#v", got)
	}

	// The writer's protocol uses one hello per shell and may split any marker
	// across PTY reads. The backend, not the renderer, owns that split state.
	ingest(t, term, "\x1b]636;H;"+nonce+"\a")
	ingest(t, term, "\x1b]636;S;"+nonce+";alias;semi\\x3bcolon;back\\\\slash;café;\a")
	got := term.Effects()
	want := []emulator.Effect{{
		Kind:         emulator.EffectCommandSnapshot,
		CommandNames: []string{"alias", "semi;colon", "back\\slash", "café"},
	}}
	if !reflect.DeepEqual(got, want) {
		state, ok := term.(*terminal)
		if !ok {
			t.Fatalf("terminal type = %T, want *terminal", term)
		}
		t.Fatalf("snapshot effects = %#v, want %#v; nonce=%q collecting=%v payload=%q pending=%#v", got, want, state.commandSnapshotNonce, state.commandSnapshotCollecting, state.commandSnapshotPayload, state.commandSnapshotPending)
	}
}

func TestCommandSnapshotRejectsForgedHelloAndMalformedPayloads(t *testing.T) {
	term := newTerminal(t, 20, 4)
	const nonce = "0123456789abcdef0123456789abcdef"
	ingest(t, term, "\x1b]636;H;"+nonce+"\a")
	// A later hello cannot re-anchor the shell's nonce. A malformed encoded
	// name list must leave the current snapshot untouched and emit nothing.
	ingest(t, term, "\x1b]636;H;aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\a")
	ingest(t, term, "\x1b]636;S;aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa;forged\a")
	ingest(t, term, "\x1b]636;S;"+nonce+";bad\\x0zescape\a")
	if got := term.Effects(); len(got) != 0 {
		t.Fatalf("forged or malformed snapshot produced effects %#v", got)
	}
}

func TestCommandSnapshotScannerSurvivesEveryChunkBoundary(t *testing.T) {
	const nonce = "0123456789abcdef0123456789abcdef"
	stream := "\x1b]636;H;" + nonce + "\a" + "\x1b]636;S;" + nonce + ";alias;function\a"
	for split := 0; split <= len(stream); split++ {
		t.Run(strconv.Itoa(split), func(t *testing.T) {
			term := newTerminal(t, 20, 4)
			ingest(t, term, stream[:split])
			ingest(t, term, stream[split:])
			got := term.Effects()
			if len(got) != 1 || got[0].Kind != emulator.EffectCommandSnapshot || !reflect.DeepEqual(got[0].CommandNames, []string{"alias", "function"}) {
				t.Fatalf("split %d produced %#v", split, got)
			}
		})
	}
}
