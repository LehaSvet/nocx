package session

import (
	"testing"
)

// nocx-1ebrs.22, the window's half. A bootstrap marker is now ONE private OSC
// string rather than a line of text, because on the typed-`ssh` path this
// window is a COPY of the person's own session output: it cannot take a marker
// away from the renderer, so the marker has to be something a terminal never
// paints. That makes the wrapper's BEL the line's end, and a pty read can hand
// over its opening and its terminator in different chunks.
//
// The tap is the typed path's only reader, so a split marker must not become
// two lines — and must not be refused for having no line ending.
// TestTheWindowHandsOverAMarkerWholeWhenItsBELArrivesLater is the reader's
// half of the shape: a marker is ONE OSC string, and a pty read can hand over
// its opening and its terminator in different chunks. The window's tap is the
// typed path's only reader, so a split marker must not become two lines or a
// line-too-long refusal.
func TestTheTapHandsOverAMarkerWholeWhenItsBELArrivesLater(t *testing.T) {
	tap := newOutputTap()
	// The bytes the loader emits for its readiness marker, spelled out: this
	// package sits below shellintegration and must not import it.
	marker := "\x1b]1337;NOCX1 LOADER_READY\x07"
	tap.feed([]byte(marker[:9]))
	if line, ok, err := tap.takeLine(); ok || err != nil {
		t.Fatalf("a half-arrived marker completed a line: %q, ok=%v, err=%v", line, ok, err)
	}
	tap.feed([]byte(marker[9:] + "\n"))
	line, ok, err := tap.takeLine()
	if err != nil || !ok {
		t.Fatalf("the completed marker was not handed over: ok=%v err=%v", ok, err)
	}
	if line != marker {
		t.Fatalf("line = %q, want the marker whole: %q", line, marker)
	}
	if rest, ok, _ := tap.takeLine(); !ok || rest != "" {
		t.Fatalf("after the marker, the tap holds %q (ok=%v), want the empty line that followed it", rest, ok)
	}
}
