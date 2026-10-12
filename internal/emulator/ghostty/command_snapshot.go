package ghostty

import (
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"github.com/shady2k/nocx/internal/emulator"
)

// OSC 636 is the shell-owned, session-local command table. Ghostty's pinned
// unknown-sequence hook does not expose OSC payloads, so this recognizer is
// advanced in scanMarkers, beside the other exact observations the emulator
// owns. Every byte still reaches Ghostty exactly once; this code recognizes
// only the shell's bounded OSC 636 contract and does not parse VT generally.
const (
	commandSnapshotFixed        = "\x1b]636;"
	commandSnapshotMaxPayload   = 65536 + 128
	commandSnapshotMaxNames     = 4096
	commandSnapshotMaxNameBytes = 65536
)

type commandSnapshotMessage struct {
	hello bool
	nonce string
	names []string
}

// scanCommandSnapshotByte advances the OSC 636 scanner by one stream byte.
// A complete BEL-terminated payload updates the terminal's private nonce or
// leaves a validated snapshot pending for ingestLocked to publish.
func (t *terminal) scanCommandSnapshotByte(b byte) bool {
	if t.commandSnapshotCollecting {
		if b == 0x07 { // The shell writer's only terminator.
			complete := !t.commandSnapshotOverflow
			payload := t.commandSnapshotPayload
			t.commandSnapshotCollecting = false
			t.commandSnapshotPayload = nil
			t.commandSnapshotOverflow = false
			if complete {
				t.acceptCommandSnapshotPayload(payload)
			}
			return true
		}
		if !t.commandSnapshotOverflow {
			if len(t.commandSnapshotPayload) >= commandSnapshotMaxPayload {
				t.commandSnapshotOverflow = true
				t.commandSnapshotPayload = nil
			} else {
				t.commandSnapshotPayload = append(t.commandSnapshotPayload, b)
			}
		}
		return false
	}

	if b == commandSnapshotFixed[t.commandSnapshotIdx] {
		t.commandSnapshotIdx++
		if t.commandSnapshotIdx == len(commandSnapshotFixed) {
			t.commandSnapshotIdx = 0
			t.commandSnapshotCollecting = true
			t.commandSnapshotPayload = t.commandSnapshotPayload[:0]
			t.commandSnapshotOverflow = false
		}
		return false
	}
	if b == commandSnapshotFixed[0] {
		t.commandSnapshotIdx = 1
	} else {
		t.commandSnapshotIdx = 0
	}
	return false
}

func (t *terminal) acceptCommandSnapshotPayload(payload []byte) {
	message, ok := parseCommandSnapshotPayload(payload)
	if !ok {
		return
	}
	if message.hello {
		// The first valid hello binds this terminal incarnation. A later hello
		// cannot re-anchor the nonce after shell output becomes user-controlled.
		if t.commandSnapshotNonce == "" {
			t.commandSnapshotNonce = message.nonce
		}
		return
	}
	if message.nonce != t.commandSnapshotNonce {
		return
	}
	t.commandSnapshotPending = append([]string(nil), message.names...)
}

func parseCommandSnapshotPayload(payload []byte) (commandSnapshotMessage, bool) {
	if len(payload) == 0 || len(payload) > commandSnapshotMaxPayload {
		return commandSnapshotMessage{}, false
	}
	kind, rest, ok := strings.Cut(string(payload), ";")
	if !ok {
		return commandSnapshotMessage{}, false
	}
	switch kind {
	case "H":
		if strings.Contains(rest, ";") || !validCommandSnapshotNonce(rest) {
			return commandSnapshotMessage{}, false
		}
		return commandSnapshotMessage{hello: true, nonce: rest}, true
	case "S":
		nonce, encodedNames, ok := strings.Cut(rest, ";")
		if !ok || !validCommandSnapshotNonce(nonce) {
			return commandSnapshotMessage{}, false
		}
		names, ok := decodeCommandSnapshotNames(encodedNames)
		if !ok {
			return commandSnapshotMessage{}, false
		}
		return commandSnapshotMessage{nonce: nonce, names: names}, true
	default:
		return commandSnapshotMessage{}, false
	}
}

func validCommandSnapshotNonce(nonce string) bool {
	if len(nonce) == 0 || len(nonce) > 128 {
		return false
	}
	for i := 0; i < len(nonce); i++ {
		c := nonce[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func decodeCommandSnapshotNames(encoded string) ([]string, bool) {
	parts := strings.Split(encoded, ";")
	names := make([]string, 0, len(parts))
	total := 0
	for _, part := range parts {
		if part == "" { // The shell writer ends the list with a separator.
			continue
		}
		name, ok := decodeCommandSnapshotName([]byte(part))
		if !ok || len(name) == 0 || !utf8.Valid(name) {
			return nil, false
		}
		total += len(name)
		if total > commandSnapshotMaxNameBytes {
			return nil, false
		}
		names = append(names, string(name))
		if len(names) > commandSnapshotMaxNames {
			return nil, false
		}
	}
	if len(names) == 0 {
		return nil, false
	}
	return names, true
}

func decodeCommandSnapshotName(encoded []byte) ([]byte, bool) {
	decoded := make([]byte, 0, len(encoded))
	for i := 0; i < len(encoded); i++ {
		if encoded[i] != '\\' {
			decoded = append(decoded, encoded[i])
			continue
		}
		if i+1 >= len(encoded) {
			return nil, false
		}
		switch encoded[i+1] {
		case '\\':
			decoded = append(decoded, '\\')
			i++
		case 'x':
			if i+3 >= len(encoded) {
				return nil, false
			}
			var b [1]byte
			if _, err := hex.Decode(b[:], encoded[i+2:i+4]); err != nil {
				return nil, false
			}
			decoded = append(decoded, b[0])
			i += 3
		default:
			return nil, false
		}
	}
	return decoded, true
}

func (t *terminal) publishPendingCommandSnapshot() {
	if t.commandSnapshotPending == nil {
		return
	}
	t.effects = append(t.effects, emulator.Effect{
		Kind:         emulator.EffectCommandSnapshot,
		CommandNames: append([]string(nil), t.commandSnapshotPending...),
	})
	t.commandSnapshotPending = nil
}
