package bootstrapstream

import (
	"bytes"
	"testing"
)

// The pane bytes that precede a loader's token, exactly as a zsh line editor
// emits them: cursor-show and bracketed-paste resets, a partial-line echo
// terminated by CARRIAGE RETURN (never a newline), then the token itself.
// The reader's positional rule cannot see the token there - the buffer does
// not begin with an OSC and holds no newline - and on CI (run
// 39531371611, both ci-linux variants) it starved the typed-ssh bootstrap
// until its receiver deadline gave up and aborted a far side that had been
// about to announce itself (nocx-1ebrs.39).
func TestNextLineStartSkipsRendererBytesBeforeAnEmbeddedToken(t *testing.T) {
	junk := "\x1b[K\x1b[?1h\x1b=\x1b[?2004h\x1b[Ks\rssh runner@127.0.0.1:44433"
	// #nosec G101 — a protocol token from the fixture, not a credential.
	token := "\x1b]1337;NOCX1 LOADER_READY\x07"
	buf := []byte(junk + token)

	if _, _, ok := LineEnd(buf); ok {
		t.Fatal("the pre-fix reader must not see the token behind renderer bytes (the starvation this fix removes)")
	}
	start, ok := NextLineStart(buf)
	if !ok {
		t.Fatalf("the next line start not found in %q", printableForTest(buf))
	}
	if !bytes.HasPrefix(buf[start:], []byte(token)) {
		t.Fatalf("next line start = %d, want the token's own opening: %q", start, printableForTest(buf[start:]))
	}
	trimmed := buf[start:]
	content, _, ok := LineEnd(trimmed)
	if !ok {
		t.Fatalf("the token still does not parse after the trim: %q", printableForTest(trimmed))
	}
	if !bytes.Equal(trimmed[:content], []byte(token)) {
		t.Fatalf("line = %q, want the token whole", printableForTest(trimmed[:content]))
	}
}

// No OSC opening in the buffer at all: nothing to trim, and a future byte
// (a newline, or an OSC opening) is what can still complete a line.
func TestNextLineStartFindsNothingWithoutAnOscOpening(t *testing.T) {
	buf := []byte("\x1b[Ks\rssh runner@127.0.0.1:44433 more renderer bytes")
	start, ok := NextLineStart(buf)
	if ok {
		t.Fatalf("next line start = %d in a buffer with no OSC opening, want none", start)
	}
}

// An unterminated OSC at the head is a PREFIX MATCH: its BEL may still
// arrive, so nothing is trimmed and the start is the buffer's own head.
func TestNextLineStartKeepsAnUnterminatedOscAtTheHead(t *testing.T) {
	buf := []byte("\x1b]1337;NOCX1 LOA")
	start, ok := NextLineStart(buf)
	if !ok || start != 0 {
		t.Fatalf("next line start = %d (ok=%v), want 0/true for an unterminated OSC at the head", start, ok)
	}
}

func printableForTest(b []byte) string {
	var sb bytes.Buffer
	for _, r := range b {
		switch {
		case r == 0x1b:
			sb.WriteString("<ESC>")
		case r == 0x07:
			sb.WriteString("<BEL>")
		case r < 0x20:
			sb.WriteString("<")
			const hex = "0123456789abcdef"
			sb.WriteByte(hex[r>>4])
			sb.WriteByte(hex[r&0xf])
			sb.WriteString(">")
		default:
			sb.WriteByte(r)
		}
	}
	return sb.String()
}
