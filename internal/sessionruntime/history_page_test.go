package sessionruntime

// The live history page (nocx-zg3k3.10.3): one page of the scrollback the
// emulator holds, read under ONE acquisition of the runtime lock so the
// page's rows, its floor and the head it is measured against describe one
// instant of the buffer.
//
// The cursor a caller pages with is the ABSOLUTE history row number this
// package has kept since the first row left the screen — screenDepartedRows
// space, corrected by the rows a growing pane pulled back onto the screen
// (owed): a row's number never changes because output arrived, the pane was
// resized, or the alternate screen took over. What moves the numbers is only
// what really removes rows: the library's retention pruning (floor rises) and
// an erase-saved-lines (history empties; floor jumps to the head). Every page
// answer states the floor and the interval it delivered, so a cursor that
// fell below the floor is answered with the truth — the empty interval at the
// cursor — instead of a short list a caller would read as a short history.
//
// These tests run over the REAL emulator (libghostty-vt behind its port):
// the retention question, the alternate screen and ED3 are the library's
// behaviour, and a fake would answer for it.

import (
	"fmt"
	"testing"
)

func historyPageUint(v uint64) *uint64 { return &v }

func historyPageTexts(p HistoryPageView) []string {
	out := make([]string, 0, len(p.Rows))
	for _, r := range p.Rows {
		out = append(out, streamRowText(r))
	}
	return out
}

func historyPageWant(t *testing.T, got []string, from, to int) {
	t.Helper()
	if len(got) != to-from {
		t.Fatalf("page holds %d rows, want %d (L%06d..L%06d)", len(got), to-from, from, to-1)
	}
	for i, txt := range got {
		if want := fmt.Sprintf("L%06d", from+i); txt != want {
			t.Fatalf("row %d reads %q, want %q", from+i, txt, want)
		}
	}
}

// A client pages backwards through history and the active screen in one
// absolute coordinate, receiving every row exactly once with no gap at a
// page seam. The final page reaches the retention floor; the screen suffix
// is not omitted or renumbered.
func TestHistoryPageWalksBackwardDeliveringEveryRowOnce(t *testing.T) {
	s := obsSession(t, harnessGeometry(80, 24))
	obsFeed(t, s, 0, 100) // 77 have departed; the active range follows them

	var got []string
	first, last := HistoryPageView{}, HistoryPageView{}
	before := (*uint64)(nil)
	pages := 0
	for {
		p, err := s.ReadHistoryPage(before, 30)
		if err != nil {
			t.Fatalf("read the page: %v", err)
		}
		if pages == 0 {
			first = p
		}
		last = p
		got = append(got, historyPageTexts(p)...)
		if !p.More {
			break
		}
		b := p.Start
		before = &b
		pages++
		if pages > 10 {
			t.Fatal("paging never reached the floor: more stayed true past the buffer")
		}
	}
	// Each page is oldest-first, the walk newest-page-first: [71,101),
	// [41,71), [11,41), [0,11). The final active row is blank but still has
	// its absolute position and must be delivered exactly once.
	for i, txt := range got {
		var want int
		switch {
		case i < 30:
			want = 71 + i
		case i < 60:
			want = 41 + (i - 30)
		case i < 90:
			want = 11 + (i - 60)
		default:
			want = i - 90
		}
		if i == 29 {
			if txt != "" {
				t.Fatalf("active bottom row reads %q, want blank", txt)
			}
			continue
		}
		if name := fmt.Sprintf("L%06d", want); txt != name {
			t.Fatalf("walked row %d reads %q, want %q", i, txt, name)
		}
	}
	if len(got) != 101 {
		t.Fatalf("the walk collected %d rows, want the 77 departed rows and 24 active rows", len(got))
	}
	if first.Start != 71 || first.End != 101 || first.BaseRow != 77 {
		t.Fatalf("the head page is [%d,%d) at base %d, want [71,101) with active base 77", first.Start, first.End, first.BaseRow)
	}
	if last.Start != 0 || last.End != 11 {
		t.Fatalf("the walk's last page is [%d,%d), want [0,11) with the floor reached", last.Start, last.End)
	}
	if last.Floor != 0 {
		t.Fatalf("the walk ends at floor %d, want 0: nothing was pruned", last.Floor)
	}
}

// Output arriving BETWEEN pages must not duplicate or lose a row at the seam:
// the numbers the first page handed out still name the same rows after forty
// more lines have departed.
func TestHistoryPageAcrossArrivingOutputHasNoSeamDuplicate(t *testing.T) {
	s := obsSession(t, harnessGeometry(80, 24))
	obsFeed(t, s, 0, 60) // 37 rows departed; the active screen starts at 37

	head, err := s.ReadHistoryPage(nil, 10)
	if err != nil {
		t.Fatalf("read the head page: %v", err)
	}
	if head.Start != 51 || head.End != 61 || head.BaseRow != 37 {
		t.Fatalf("the head page is [%d,%d) at base %d, want [51,61) at 37", head.Start, head.End, head.BaseRow)
	}
	if len(head.Rows) != 10 {
		t.Fatalf("the head page carries %d rows, want ten active-screen rows", len(head.Rows))
	}

	// The live-history reader skips the active-screen suffix and continues
	// from its base. Those older rows keep their numbers while output arrives.
	firstCursor := head.BaseRow
	first, err := s.ReadHistoryPage(&firstCursor, 10)
	if err != nil {
		t.Fatalf("read the first history page: %v", err)
	}
	if first.Start != 27 || first.End != 37 || first.BaseRow != 37 {
		t.Fatalf("the first history page is [%d,%d) at base %d, want [27,37) at 37", first.Start, first.End, first.BaseRow)
	}
	historyPageWant(t, historyPageTexts(first), 27, 37)

	obsFeed(t, s, 60, 40) // forty more lines depart while the caller reads

	second, err := s.ReadHistoryPage(&first.Start, 10)
	if err != nil {
		t.Fatalf("read the second page: %v", err)
	}
	if second.Start != 17 || second.End != 27 {
		t.Fatalf("the second page is [%d,%d), want [17,27) — arrivals moved the head, not the cursor", second.Start, second.End)
	}
	historyPageWant(t, historyPageTexts(second), 17, 27)
	if second.Floor != 0 {
		t.Fatalf("the second page states floor %d, want 0", second.Floor)
	}
}

// A page after ED3 still carries the active screen, while its floor advances
// to the first active row. A cursor from before the clear that lies below
// this floor is answered with the empty interval at that cursor.
func TestHistoryPageBelowTheFloorIsAnsweredWithTheFloor(t *testing.T) {
	s := obsSession(t, harnessGeometry(80, 24))
	obsFeed(t, s, 0, 50) // history L000000..L000026, head 27

	first, err := s.ReadHistoryPage(nil, 10)
	if err != nil {
		t.Fatalf("read the first page: %v", err)
	}
	if err = s.Ingest([]byte("\x1b[H\x1b[2J\x1b[3J")); err != nil {
		t.Fatalf("clear: %v", err)
	}

	p, err := s.ReadHistoryPage(nil, 30)
	if err != nil {
		t.Fatalf("read a page after the clear: %v", err)
	}
	if len(p.Rows) != 24 {
		t.Fatalf("the page after the clear carries %d rows, want the 24 active-screen rows", len(p.Rows))
	}
	if p.Start != 27 || p.End != 51 || p.BaseRow != 27 {
		t.Fatalf("the page after the clear is [%d,%d) at base %d, want active range [27,51) at 27", p.Start, p.End, p.BaseRow)
	}
	if p.Floor != 27 {
		t.Fatalf("the page after the clear states floor %d, want 27 — where the live tier now begins", p.Floor)
	}
	if p.More {
		t.Fatal("the page after the clear says more, want false")
	}

	// The cursor a client held from BEFORE the clear points below the floor.
	// The answer is the floor again, not an error and not silence about it.
	stale := first.Floor
	q, err := s.ReadHistoryPage(&stale, 30)
	if err != nil {
		t.Fatalf("read with the stale cursor: %v", err)
	}
	if len(q.Rows) != 0 || q.More {
		t.Fatalf("the stale cursor's page carries %d rows with more=%v, want the empty answer", len(q.Rows), q.More)
	}
	if q.Start != stale || q.End != stale {
		t.Fatalf("the stale cursor's page is [%d,%d), want the empty interval at the cursor [%d,%d)", q.Start, q.End, stale, stale)
	}
	if q.Floor != 27 {
		t.Fatalf("the stale cursor's page states floor %d, want 27", q.Floor)
	}
}

// After `clear` (ED3) in a session with no shell integration the emulator's
// history is simply what it holds, and output that scrolls off afterwards
// departs into a history whose numbering continues — no boundary is invented,
// and no row is renumbered by the clear.
func TestHistoryPageContinuesNumberingAfterClear(t *testing.T) {
	s := obsSession(t, harnessGeometry(80, 24))
	obsFeed(t, s, 0, 50)
	if err := s.Ingest([]byte("\x1b[H\x1b[2J\x1b[3J")); err != nil {
		t.Fatalf("clear: %v", err)
	}

	obsFeed(t, s, 50, 40) // seventeen of these lines depart into the fresh history

	p, err := s.ReadHistoryPage(nil, 5)
	if err != nil {
		t.Fatalf("read the head page: %v", err)
	}
	if p.Start != 63 || p.End != 68 || p.BaseRow != 44 {
		t.Fatalf("the head page is [%d,%d) at base %d, want [63,68) at 44", p.Start, p.End, p.BaseRow)
	}
	texts := historyPageTexts(p)
	if len(texts) != 5 || texts[0] != "L000086" || texts[3] != "L000089" || texts[4] != "" {
		t.Fatalf("the head page rows are %v, want four latest active rows and the trailing blank row", texts)
	}
	if p.Floor != 27 {
		t.Fatalf("the head page states floor %d, want 27", p.Floor)
	}
	if !p.More {
		t.Fatal("the head page says no more, want true: rows remain below")
	}
	cursor := p.BaseRow
	history, err := s.ReadHistoryPage(&cursor, 10)
	if err != nil {
		t.Fatalf("read history after the active screen: %v", err)
	}
	if history.Start != 34 || history.End != 44 {
		t.Fatalf("history page is [%d,%d), want [34,44)", history.Start, history.End)
	}
	historyPageWant(t, historyPageTexts(history), 57, 67)
}

// While the alternate screen holds the pane a page answers its own active
// rows and identity, never the primary's rows. Restoring the primary resumes
// its history under the same absolute row numbers.
func TestHistoryPageOnTheAlternateScreenReturnsNoPrimaryRows(t *testing.T) {
	s := obsSession(t, harnessGeometry(80, 24))
	obsFeed(t, s, 0, 50) // primary history L000000..L000026

	if err := s.Ingest([]byte("\x1b[?1049h")); err != nil {
		t.Fatalf("enter the alternate screen: %v", err)
	}
	p, err := s.ReadHistoryPage(nil, 30)
	if err != nil {
		t.Fatalf("read a page on the alternate screen: %v", err)
	}
	if len(p.Rows) != 24 || p.More || p.Start != 0 || p.End != 24 || p.BaseRow != 0 || p.BufferKind != "alternate" {
		t.Fatalf("alternate page range=[%d,%d) base=%d kind=%s rows=%d more=%v, want [0,24) local active rows", p.Start, p.End, p.BaseRow, p.BufferKind, len(p.Rows), p.More)
	}
	alternateInstance := p.BufferInstance

	if err = s.Ingest([]byte("\x1b[?1049l")); err != nil {
		t.Fatalf("restore the primary screen: %v", err)
	}
	q, err := s.ReadHistoryPage(nil, 10)
	if err != nil {
		t.Fatalf("read a page after the restore: %v", err)
	}
	if q.Start != 41 || q.End != 51 || q.BaseRow != 27 || q.BufferKind != "normal" || q.BufferInstance == alternateInstance {
		t.Fatalf("restored page range=[%d,%d) base=%d identity=%s/%s, want primary [41,51) at base 27 with a new identity", q.Start, q.End, q.BaseRow, q.BufferKind, q.BufferInstance)
	}
	cursor := q.BaseRow
	history, err := s.ReadHistoryPage(&cursor, 10)
	if err != nil {
		t.Fatalf("read primary history after the restore: %v", err)
	}
	if history.Start != 17 || history.End != 27 {
		t.Fatalf("the restored history page is [%d,%d), want [17,27) — the excursion moved nothing", history.Start, history.End)
	}
	historyPageWant(t, historyPageTexts(history), 17, 27)
}

// A pane that GREW pulled history rows back onto the screen. History pages
// still cover the active range, but baseRow marks where the UI must stop
// painting scrollback; asking at that base returns contiguous older history.
func TestHistoryPageHeadExcludesRowsPulledBackOntoTheScreen(t *testing.T) {
	s := obsSession(t, harnessGeometry(80, 24))
	obsFeed(t, s, 0, 60) // history L000000..L000036, head 37

	if _, err := s.CommitGeometry(harnessGeometry(80, 48)); err != nil {
		t.Fatalf("grow the pane: %v", err)
	}
	// The growth refilled the screen from the history: L000013..L000036 are
	// on the screen again, each owed the departure it will not report again,
	// and the history holds L000000..L000012. One line departs: it pays one
	// row of that debt, so the history grows by a re-departure the head must
	// not count twice — and a ten-row page still asks the emulator only for
	// positions it retains.
	obsFeed(t, s, 60, 1)

	p, err := s.ReadHistoryPage(nil, 10)
	if err != nil {
		t.Fatalf("read the head page: %v", err)
	}
	if p.Start != 52 || p.End != 62 || p.BaseRow != 14 {
		t.Fatalf("the active head page is [%d,%d) at base %d, want [52,62) at 14", p.Start, p.End, p.BaseRow)
	}
	if len(p.Rows) != 10 || !p.More {
		t.Fatalf("the active head page carries %d rows with more=%v, want ten rows and older history", len(p.Rows), p.More)
	}
	if p.Floor != 0 {
		t.Fatalf("the head page states floor %d, want 0", p.Floor)
	}
	cursor := p.BaseRow
	q, err := s.ReadHistoryPage(&cursor, 10)
	if err != nil {
		t.Fatalf("read history below the active screen: %v", err)
	}
	if q.Start != 4 || q.End != 14 || !q.More {
		t.Fatalf("history page is [%d,%d) with more=%v, want [4,14) with more=true", q.Start, q.End, q.More)
	}
	historyPageWant(t, historyPageTexts(q), 4, 14)
	below, err := s.ReadHistoryPage(&q.Start, 10)
	if err != nil {
		t.Fatalf("read the page below: %v", err)
	}
	if below.Start != 0 || below.End != 4 || below.More {
		t.Fatalf("the page below is [%d,%d) with more=%v, want [0,4) with more=false", below.Start, below.End, below.More)
	}
	historyPageWant(t, historyPageTexts(below), 0, 4)
}

// A missing or out-of-range input is an error, never a silent default.
func TestHistoryPageRefusesAPathologicalLimit(t *testing.T) {
	s := obsSession(t, harnessGeometry(80, 24))
	obsFeed(t, s, 0, 30)

	if _, err := s.ReadHistoryPage(nil, 0); err == nil {
		t.Fatal("a zero limit was accepted, want an error")
	}
	if _, err := s.ReadHistoryPage(nil, -1); err == nil {
		t.Fatal("a negative limit was accepted, want an error")
	}
	if _, err := s.ReadHistoryPage(nil, MaxHistoryPageRows+1); err == nil {
		t.Fatal("a limit above the page bound was accepted, want an error")
	}
	_ = historyPageUint
}
