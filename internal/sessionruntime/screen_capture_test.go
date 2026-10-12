package sessionruntime

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestReadScreenCaptureReturnsOneAbsoluteBackendOwnedRange(t *testing.T) {
	s, _, _ := realRuntime(t)
	var output strings.Builder
	for row := range 30 {
		fmt.Fprintf(&output, "row-%02d\r\n", row)
	}
	if err := s.Ingest([]byte(output.String())); err != nil {
		t.Fatalf("ingest screen rows: %v", err)
	}

	capture, err := s.ReadScreenCapture(nil)
	if err != nil {
		t.Fatalf("read backend capture: %v", err)
	}
	if capture.Start != 0 || capture.End != 31 || capture.Floor != 0 || capture.Total != 31 {
		t.Fatalf("capture range = floor %d, [%d,%d), total %d; want floor 0, [0,31), total 31",
			capture.Floor, capture.Start, capture.End, capture.Total)
	}
	if got := len(capture.Rows); got != 31 {
		t.Fatalf("capture returned %d rows, want 31", got)
	}
	if got := capture.Rows[0].Cells[0].Grapheme; got != "r" {
		t.Fatalf("first absolute row starts with %q, want r", got)
	}
	if capture.Identity.BufferInstance == 0 || capture.Generation == 0 || capture.Revision == 0 {
		t.Fatalf("capture lacks backend identity/revision: %+v", capture)
	}
	if capture.Identity.AltScreen {
		t.Fatal("buffer identity is alternate, want primary/normal")
	}
}

func TestReadScreenCaptureRefusesRowsAboveJSONSafeIntegerCeiling(t *testing.T) {
	s, _, _ := realRuntime(t)
	s.screenDepartedRows = MaxScreenRowOffset
	if _, err := s.ReadScreenCapture(nil); err == nil || !strings.Contains(err.Error(), "JSON safe-integer ceiling") {
		t.Fatalf("capture at the safe-integer ceiling returned err=%v, want refusal before row offsets become inexact", err)
	}
}

func TestReadScreenCaptureChangesIdentityAcrossAlternateBufferSessions(t *testing.T) {
	s, _, _ := realRuntime(t)
	if err := s.Ingest([]byte("primary")); err != nil {
		t.Fatalf("write primary buffer: %v", err)
	}
	primary, captureErr := s.ReadScreenCapture(nil)
	if captureErr != nil {
		t.Fatalf("capture primary buffer: %v", captureErr)
	}
	if err := s.Ingest([]byte("\x1b[?1049hALT")); err != nil {
		t.Fatalf("enter alternate buffer: %v", err)
	}
	alternate, alternateErr := s.ReadScreenCapture(nil)
	if alternateErr != nil {
		t.Fatalf("capture alternate buffer: %v", alternateErr)
	}
	if !alternate.Identity.AltScreen || alternate.Identity.BufferInstance == primary.Identity.BufferInstance {
		t.Fatalf("alternate identity = %+v, primary = %+v", alternate.Identity, primary.Identity)
	}
	if alternate.Floor != 0 || alternate.Start != 0 || alternate.Total != alternate.End || len(alternate.Rows) != alternate.Identity.Rows {
		t.Fatalf("alternate capture includes primary history: floor %d, [%d,%d), rows %d", alternate.Floor, alternate.Start, alternate.End, alternate.Total)
	}
	if err := s.Ingest([]byte("\x1b[?1049l")); err != nil {
		t.Fatalf("leave alternate buffer: %v", err)
	}
	restored, err := s.ReadScreenCapture(nil)
	if err != nil {
		t.Fatalf("capture restored primary buffer: %v", err)
	}
	if restored.Identity.AltScreen || restored.Identity.BufferInstance == primary.Identity.BufferInstance {
		t.Fatalf("restored primary identity = %+v, original = %+v; buffer identity must not be reused", restored.Identity, primary.Identity)
	}
}

func TestReadScreenCaptureRefusesARangeOutsideRetainedRows(t *testing.T) {
	s, _, _ := realRuntime(t)
	if err := s.Ingest([]byte("hello")); err != nil {
		t.Fatalf("ingest screen: %v", err)
	}
	_, err := s.ReadScreenCapture(&ScreenRange{Start: 500, End: 501})
	if err == nil {
		t.Fatal("an absolute range beyond the captured screen was silently accepted")
	}
}

func TestEncodeFrameRejectsAnUnsafeAbsoluteRowRange(t *testing.T) {
	s, _, _ := realRuntime(t)
	snapshot := s.Snapshot()
	snapshot.BaseRow = MaxScreenRowOffset
	if _, err := EncodeFrame(snapshot); err == nil {
		t.Fatal("frame encoder accepted a base row whose screen range exceeds JavaScript's safe integers")
	}
}

func TestFrameHistoryAndScreenCaptureShareAbsoluteRows(t *testing.T) {
	s, _, _ := realRuntime(t)
	var output strings.Builder
	for row := range 30 {
		fmt.Fprintf(&output, "row-%02d\r\n", row)
	}
	if err := s.Ingest([]byte(output.String())); err != nil {
		t.Fatalf("ingest screen rows: %v", err)
	}
	capture, err := s.ReadScreenCapture(nil)
	if err != nil {
		t.Fatalf("read screen capture: %v", err)
	}
	history, err := s.ReadHistoryPage(nil, MaxHistoryPageRows)
	if err != nil {
		t.Fatalf("read history page: %v", err)
	}
	snapshot := s.Snapshot()
	raw, err := EncodeFrame(snapshot)
	if err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	var frame struct {
		State struct {
			BaseRow        uint64 `json:"baseRow"`
			BufferKind     string `json:"bufferKind"`
			BufferInstance string `json:"bufferInstance"`
		} `json:"state"`
	}
	if decodeErr := json.Unmarshal(raw, &frame); decodeErr != nil {
		t.Fatalf("decode frame: %v", decodeErr)
	}
	if capture.Identity.Rows <= 0 {
		t.Fatalf("screen capture has invalid geometry rows: %d", capture.Identity.Rows)
	}
	rows := uint64(capture.Identity.Rows) // #nosec G115 -- runtime geometry is a non-negative int
	wantBase := capture.Total - rows
	if frame.State.BaseRow != wantBase || history.BaseRow != wantBase {
		t.Fatalf("absolute bases: frame=%d history=%d capture=%d, want %d", frame.State.BaseRow, history.BaseRow, wantBase, wantBase)
	}
	if history.Start != capture.Start || history.End != capture.End || len(history.Rows) != len(capture.Rows) {
		t.Fatalf("history interval [%d,%d) rows=%d; capture [%d,%d) rows=%d", history.Start, history.End, len(history.Rows), capture.Start, capture.End, len(capture.Rows))
	}
	if frame.State.BufferKind != frameBufferKind(capture.Identity.AltScreen) || history.BufferKind != frame.State.BufferKind || history.BufferInstance != fmt.Sprint(capture.Identity.BufferInstance) || frame.State.BufferInstance != fmt.Sprint(capture.Identity.BufferInstance) {
		t.Fatalf("buffer identities: frame=%+v history=%s/%s capture=%+v", frame.State, history.BufferKind, history.BufferInstance, capture.Identity)
	}
	for i := range capture.Rows {
		if got, want := streamRowText(history.Rows[i]), streamRowText(capture.Rows[i]); got != want {
			rowOffset := uint64(i) // #nosec G115 -- i is a non-negative slice index
			t.Fatalf("absolute row %d differs: history=%q capture=%q", capture.Start+rowOffset, got, want)
		}
	}
	if ingestErr := s.Ingest([]byte("\x1b[?1049hALT")); ingestErr != nil {
		t.Fatalf("enter alternate screen: %v", ingestErr)
	}
	altCapture, err := s.ReadScreenCapture(nil)
	if err != nil {
		t.Fatalf("read alternate capture: %v", err)
	}
	altHistory, err := s.ReadHistoryPage(nil, MaxHistoryPageRows)
	if err != nil {
		t.Fatalf("read alternate history page: %v", err)
	}
	altRaw, err := EncodeFrame(s.Snapshot())
	if err != nil {
		t.Fatalf("encode alternate frame: %v", err)
	}
	if err := json.Unmarshal(altRaw, &frame); err != nil {
		t.Fatalf("decode alternate frame: %v", err)
	}
	if frame.State.BaseRow != 0 || altHistory.BaseRow != 0 || altCapture.Start != 0 || altHistory.Start != altCapture.Start || altHistory.End != altCapture.End {
		t.Fatalf("alternate ranges disagree: frame base=%d history=[%d,%d) base=%d capture=[%d,%d)", frame.State.BaseRow, altHistory.Start, altHistory.End, altHistory.BaseRow, altCapture.Start, altCapture.End)
	}
	if frame.State.BufferKind != "alternate" || altHistory.BufferKind != "alternate" || altHistory.BufferInstance != fmt.Sprint(altCapture.Identity.BufferInstance) || altCapture.Identity.BufferInstance == capture.Identity.BufferInstance {
		t.Fatalf("alternate identities disagree: frame=%+v history=%s/%s capture=%+v", frame.State, altHistory.BufferKind, altHistory.BufferInstance, altCapture.Identity)
	}
}
