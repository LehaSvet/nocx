package agentcapture_test

// The format, and the two properties everything downstream stands on: a
// capture read back is the capture that was written, and a frame PAINTED into
// a capture replays to the frame it was painted from.
//
// The second one is the load-bearing one. A calibration set is a capture file
// and a mark per label; the frame a person produced is not stored beside it,
// it is DERIVED by replaying to the mark. That is one owner of one truth
// rather than two that agree until they do not — and it is only sound if
// painting is lossless for everything a rule can read, which is what these
// tests measure rather than assume.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/agentcapture"
	"github.com/shady2k/nocx/internal/agentcapture/replaylocal"
	"github.com/shady2k/nocx/internal/log"
	"github.com/shady2k/nocx/internal/paneview"
	"github.com/shady2k/nocx/internal/paneview/paneviewtest"
)

// paint builds a frame the way the product builds one — through a real
// panegrid Store fed real bytes — because a frame assembled by hand is a frame
// the product never makes.
func liveFrame(t *testing.T, cols, rows int, bytes string) paneview.Frame {
	t.Helper()
	store := paneviewtest.NewViews(log.NewSlogAdapter(nil))
	const pane = "probe"
	if err := store.Watch(pane, cols, rows); err != nil {
		t.Fatalf("enrol: %v", err)
	}
	t.Cleanup(func() { store.Withdraw(pane) })
	store.Feed(pane, []byte(bytes))
	f, err := store.Frame(pane)
	if err != nil {
		t.Fatalf("frame: %v", err)
	}
	return f
}

// TestPaintReplaysToTheSameFrame is the whole argument for storing marks
// rather than frames. Double-width graphemes, a parked cursor and the
// alternate screen are all in one screen because each is a way the round trip
// could lose exactly what a rule reads: ADR-0041 pins the emulator for its
// COLUMN geometry, and the cursor is the one marker an agent cannot forge.
func TestPaintReplaysToTheSameFrame(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		cols, rows int
		bytes      string
	}{
		{
			name: "alternate screen with chrome and a parked cursor",
			cols: 24, rows: 6,
			bytes: "\x1b[?1049h\x1b[2J\x1b[2;1H────────────────────────" +
				"\x1b[3;1H❯ \x1b[4;1H────────────────────────\x1b[3;3H",
		},
		{
			name: "double-width graphemes hold their columns",
			cols: 12, rows: 3,
			bytes: "\x1b[2J\x1b[1;1H日本語 ok\x1b[2;5Hx\x1b[1;4H",
		},
		{
			// The rendering is part of the frame (nocx-xn63t.6.56): a mark
			// painted from a frame that carries a faint run replays with that
			// run still faint, which is the whole of what a rule reads an
			// agent's own drawing off.
			name: "a faint run keeps its rendering",
			cols: 24, rows: 3,
			bytes: "\x1b[2J\x1b[2;1H❯\u00a0\x1b[2mTry \"fix lint errors\"\x1b[22m typed\x1b[3;1H─",
		},
		{
			name: "an empty screen paints to an empty screen",
			cols: 8, rows: 3,
			bytes: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := liveFrame(t, tc.cols, tc.rows, tc.bytes)
			header := agentcapture.Header{
				Agent: "probe", Argv: []string{"probe"},
				Cols: want.Cols, Rows: want.Rows,
			}
			chunks := []agentcapture.Chunk{{AtMs: 0, Offset: 0, Data: agentcapture.Paint(want)}}
			moments, err := agentcapture.Frames(context.Background(), replaylocal.Replayer{}, header, chunks, []int64{0})
			if err != nil {
				t.Fatalf("frames: %v", err)
			}
			if len(moments) != 1 {
				t.Fatalf("moments = %d, want 1", len(moments))
			}
			if diff := frameDiff(moments[0].Frame, want); diff != "" {
				t.Fatalf("replayed frame differs from the painted one:\n%s", diff)
			}
		})
	}
}

// frameDiff names the first thing that differs, in the vocabulary a person
// reading the failure has: a cursor cell, a geometry, or a row. Printing two
// whole frames as structs prints four thousand cells and says nothing.
func frameDiff(got, want paneview.Frame) string {
	if got.Cols != want.Cols || got.Rows != want.Rows {
		return fmt.Sprintf("geometry %dx%d, want %dx%d", got.Cols, got.Rows, want.Cols, want.Rows)
	}
	if got.AltScreen != want.AltScreen {
		return fmt.Sprintf("altScreen %v, want %v", got.AltScreen, want.AltScreen)
	}
	if got.CursorX != want.CursorX || got.CursorY != want.CursorY {
		return fmt.Sprintf("cursor at %d,%d, want %d,%d", got.CursorX, got.CursorY, want.CursorX, want.CursorY)
	}
	var out strings.Builder
	for y := range want.Lines {
		if sameRow(got.Lines[y], want.Lines[y]) {
			continue
		}
		fmt.Fprintf(&out, "row %d:\n got %q\nwant %q\n", y, got.Text(y), want.Text(y))
		for x := range want.Lines[y] {
			if !sameCell(got.Lines[y][x], want.Lines[y][x]) {
				fmt.Fprintf(&out, "  first differing column %d: got %+v, want %+v\n",
					x, got.Lines[y][x], want.Lines[y][x])
				break
			}
		}
	}
	return out.String()
}

// sameRow compares two rows under the same rule, so a row that differs only in
// how its blanks are spelled is not reported as a difference.
func sameRow(a, b []paneview.Cell) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameCell(a[i], b[i]) {
			return false
		}
	}
	return true
}

// sameCell compares two cells as the frame's own reader does.
//
// A blank column is blank whether it spells its emptiness as "" or as " ": Paint
// writes a space for every cell it draws and trims only the trailing run, so a
// replayed screen carries spaces where the painted one carried nothing, and
// Frame.Text renders both as one space. Asserting on the spelling would make
// this test fail on a difference no consumer can observe.
func sameCell(a, b paneview.Cell) bool {
	if a.Width != b.Width {
		return false
	}
	if a.Width == 0 {
		return true
	}
	// The rendering is compared for the same reason the text is: a frame that
	// lost the faint run on the way through a paint is a frame a rule reads
	// differently (nocx-xn63t.6.56).
	if a.Faint != b.Faint {
		return false
	}
	at, bt := a.Text, b.Text
	if at == " " {
		at = ""
	}
	if bt == " " {
		bt = ""
	}
	return at == bt
}

func TestSplitUTF8SurvivesCaptureAndReplay(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "capture.jsonl")
	header := agentcapture.Header{Agent: "omp", Argv: []string{"omp"}, Cols: 20, Rows: 4}
	want := liveFrame(t, 20, 4, "[H界")
	chunks := []agentcapture.Chunk{
		{AtMs: 1, Offset: 0, Data: []byte{0x1b, '[', 'H', 0xe7}},
		{AtMs: 2, Offset: 4, Data: []byte{0x95, 0x8c}},
	}
	if err := agentcapture.Write(path, header, chunks); err != nil {
		t.Fatalf("write: %v", err)
	}
	gotHeader, gotChunks, err := agentcapture.Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(gotChunks[0].Data, chunks[0].Data) || !bytes.Equal(gotChunks[1].Data, chunks[1].Data) {
		t.Fatalf("capture bytes changed: got %q / %q", gotChunks[0].Data, gotChunks[1].Data)
	}
	moments, err := agentcapture.Frames(context.Background(), replaylocal.Replayer{}, gotHeader, gotChunks, []int64{2})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if diff := frameDiff(moments[0].Frame, want); diff != "" {
		t.Fatalf("replayed frame differs: %s", diff)
	}
}

func TestCaptureJSONLRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "capture.jsonl")
	header := agentcapture.Header{
		Agent:   "bash",
		Argv:    []string{"bash", "-i"},
		Cols:    80,
		Rows:    24,
		Started: "2026-08-25T12:00:00Z",
		Script:  []string{"+0ms \"\\r\""},
	}
	chunks := []agentcapture.Chunk{
		{AtMs: 3, Offset: 0, Data: []byte("hello\r\n")},
		{AtMs: 8, Offset: 7, Data: []byte("\x1b[2J")},
	}
	if err := agentcapture.Write(path, header, chunks); err != nil {
		t.Fatalf("writeCapture: %v", err)
	}
	gotHeader, gotChunks, err := agentcapture.Read(path)
	if err != nil {
		t.Fatalf("readCapture: %v", err)
	}
	gotHeaderJSON, err := json.Marshal(gotHeader)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	expectedHeaderJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal expected header: %v", err)
	}
	if !bytes.Equal(gotHeaderJSON, expectedHeaderJSON) {
		t.Fatalf("header mismatch: got %s, want %s", gotHeaderJSON, expectedHeaderJSON)
	}
	gotChunksJSON, err := json.Marshal(gotChunks)
	if err != nil {
		t.Fatalf("marshal chunks: %v", err)
	}
	expectedChunksJSON, err := json.Marshal(chunks)
	if err != nil {
		t.Fatalf("marshal expected chunks: %v", err)
	}
	if !bytes.Equal(gotChunksJSON, expectedChunksJSON) {
		t.Fatalf("chunks mismatch: got %s, want %s", gotChunksJSON, expectedChunksJSON)
	}
}

func TestChunksThroughMarkIncludesExactBoundary(t *testing.T) {
	t.Parallel()
	chunks := []agentcapture.Chunk{
		{AtMs: 10, Offset: 0, Data: []byte("a")},
		{AtMs: 20, Offset: 1, Data: []byte("b")},
		{AtMs: 20, Offset: 2, Data: []byte("c")},
		{AtMs: 21, Offset: 3, Data: []byte("d")},
	}
	if consumed := agentcapture.ChunksThrough(chunks, 9, 0); consumed != 0 {
		t.Fatalf("agentcapture.ChunksThrough(..., 9) = %d, want 0", consumed)
	}
	consumed := agentcapture.ChunksThrough(chunks, 20, 0)
	if consumed != 3 {
		t.Fatalf("agentcapture.ChunksThrough(..., 20) = %d, want 3", consumed)
	}
	if consumed = agentcapture.ChunksThrough(chunks, 21, consumed); consumed != 4 {
		t.Fatalf("agentcapture.ChunksThrough(..., 21, 3) = %d, want 4", consumed)
	}
}
