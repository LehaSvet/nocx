package sessionruntime

// The live history page's read (nocx-zg3k3.10.3): one page of retained
// history followed, in the same absolute coordinate, by the active screen.
// The frame, read-screen capture and block output anchors use this coordinate
// too; the active range therefore joins history without a gap or overlap.
//
// # The cursor, and what it must survive
//
// A client pages BACKWARDS while output keeps arriving, so its cursor cannot
// be a position in the moving buffer. It is an ABSOLUTE row number: every
// primary row keeps the number assigned when it first left the top of the
// screen, even when output arrives or a pane grows and pulls rows back. The
// active screen starts at screenDepartedRows less the emulator's owed rows.
// Against that numbering:
//
//   - output appends at the head and moves nothing below it;
//   - a page may include active-screen rows, and baseRow names the first one;
//     a consumer that paints the live grid separately stops scrollback at
//     this boundary;
//   - the alternate screen uses its own buffer identity and starts at row
//     zero. It never exposes primary rows; leaving it restores the primary
//     identity and the primary's unchanged row numbers;
//   - retention pruning and erase-saved-lines (ED3) raise the floor. A cursor
//     below it is answered with an empty interval at that cursor and the
//     floor stated. After ED3 the floor is the head, but the page still
//     carries the active screen and invents no boundary row.
//
// # One instant
//
// The head, the debt, the buffer identity, geometry and rows are taken under
// ONE acquisition of the runtime lock: a page measured against a different
// screen than its rows would be a torn read.
//
// The bound on one page is deliberate: one helper round trip and one carrier
// document, small by construction, with paging — not a bigger page — as how a
// caller reaches deeper history.

import (
	"fmt"
	"strconv"

	"github.com/shady2k/nocx/internal/emulator"
)

// MaxHistoryPageRows bounds one page. Every layer that spells the request
// holds the same number: the wire contract's schema maximum, the transport's
// params validator, and this seam's own refusal — one bound, not three.
const MaxHistoryPageRows = 64

// HistoryPageView is one page answer: the interval of absolute history row
// numbers the rows were read at, the floor below which nothing is retained,
// whether rows remain below, and the rows themselves, oldest first, in the
// emulator's own copy. Start is always the exclusive lower bound of what was
// DELIVERED — an empty page's interval is empty at the cursor it was taken
// at — so a caller's next page is always before = Start, whatever the answer
// held.
type HistoryPageView struct {
	Floor          uint64         `json:"floor"`
	Start          uint64         `json:"start"`
	End            uint64         `json:"end"`
	More           bool           `json:"more"`
	BufferKind     string         `json:"bufferKind"`
	BufferInstance string         `json:"bufferInstance"`
	BaseRow        uint64         `json:"baseRow"`
	Cols           int            `json:"cols"`
	GeometryRows   int            `json:"rows"`
	Rows           []emulator.Row `json:"-"`
}

// ReadHistoryPage reads one page of the session's live history. before is
// the exclusive upper bound in the absolute history row numbering, or nil
// for the head — the newest retained rows. limit is clamped to
// [1, MaxHistoryPageRows] by REFUSAL, never by silence: a caller asking for
// more than a page is naming a shape the wire does not carry.
func (s *Session) ReadHistoryPage(before *uint64, limit int) (HistoryPageView, error) {
	if limit <= 0 || limit > MaxHistoryPageRows {
		return HistoryPageView{}, fmt.Errorf("sessionruntime: history page limit %d out of range 1..%d",
			limit, MaxHistoryPageRows)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	base, err := s.emulator.HistoryRows(0, 0)
	if err != nil {
		return HistoryPageView{}, err
	}
	activeRows, _, identity, _ := s.screenStateLocked()
	activeBase, err := s.activeBaseRowLocked(identity.AltScreen)
	if err != nil {
		return HistoryPageView{}, err
	}
	var floor uint64
	if !identity.AltScreen {
		historyRows := uint64(base.Total) //nolint:gosec // emulator row count
		if activeBase > historyRows {
			floor = activeBase - historyRows
		}
	}
	// In primary mode retained history ends at activeBase. In alternate mode
	// the active grid owns a separate local coordinate beginning at zero.
	activeCount := uint64(len(activeRows)) //nolint:gosec // emulator row count
	if activeBase > MaxScreenRowOffset || activeCount > MaxScreenRowOffset-activeBase {
		return HistoryPageView{}, fmt.Errorf("sessionruntime: absolute screen rows exceed JSON safe-integer ceiling %d", MaxScreenRowOffset)
	}
	total := activeBase + activeCount
	end := total
	if before != nil && *before < end {
		end = *before
	}
	start := end
	if end > floor {
		start = floor
		if room := end - floor; room > uint64(limit) { //nolint:gosec // row count
			start = end - uint64(limit) //nolint:gosec // row count
		}
	}
	rows := make([]emulator.Row, 0, end-start)
	historyEnd := min(end, activeBase)
	if !identity.AltScreen && start < historyEnd {
		page, err := s.emulator.HistoryRows(int(start-floor), int(historyEnd-start)) //nolint:gosec // bounded by limit
		if err != nil {
			return HistoryPageView{}, err
		}
		rows = append(rows, page.Rows...)
	}
	activeStart := max(start, activeBase)
	if end > activeStart {
		from := int(activeStart - activeBase) //nolint:gosec // bounded by screen geometry
		to := int(end - activeBase)           //nolint:gosec // bounded by screen geometry
		rows = append(rows, activeRows[from:to]...)
	}
	return HistoryPageView{
		Floor: floor, Start: start, End: start + uint64(len(rows)),
		BaseRow: activeBase, BufferKind: frameBufferKind(identity.AltScreen),
		BufferInstance: strconv.FormatUint(identity.BufferInstance, 10),
		Cols:           identity.Cols, GeometryRows: identity.Rows,
		More: start > floor, Rows: rows,
	}, nil
}
