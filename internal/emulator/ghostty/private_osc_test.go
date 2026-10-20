package ghostty

import (
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/emulator"
)

// nocx-1ebrs.22, the emulator's half: the private OSC a bootstrap marker
// travels in paints NOTHING and moves NOTHING.
//
// The marker used to be a line of text, which is why it appeared in a
// person's transcript on the typed-`ssh` path — the far side's stdout is the
// pty they are watching, and the bootstrap window is a copy of it that cannot
// take a byte away. Wrapping the marker in OSC 1337 only fixes that if a
// terminal consumes the string whole: no cell, no cursor movement, no
// scrollback row. This is that assertion, made where the decision is
// implemented rather than assumed by the writer.
func TestAPrivateOSCMarkerPaintsNoCellAndMovesNoCursor(t *testing.T) {
	term := newTerminal(t, 40, 5)
	ingest(t, term, "before")
	before := firstRow(t, term)

	// Exactly the bytes the loader emits for its readiness marker.
	if replies := ingest(t, term, "\x1b]1337;NOCX1 LOADER_READY\x07"); len(replies) != 0 {
		t.Fatalf("the marker produced a reply to the program: %q", replies)
	}
	if after := firstRow(t, term); after != before {
		t.Fatalf("the marker was painted:\nbefore: %q\nafter:  %q", before, after)
	}

	// And the text that follows it lands where it would have without the
	// marker: same row, same column. A consumed string that still moved the
	// cursor would corrupt every prompt drawn after it.
	ingest(t, term, "-after")
	if got, want := firstRow(t, term), "before-after"; got != want {
		t.Fatalf("output after the marker = %q, want %q — the marker moved the cursor", got, want)
	}
}

// firstRow is the top row as text, with the padding a screen carries removed.
func firstRow(t *testing.T, term emulator.Terminal) string {
	t.Helper()
	return strings.TrimRight(screenText(t, term, 1), " \n")
}
