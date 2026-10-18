package app

// A connected renderer is TOLD when a participant's tab leaves the window,
// through the composition root (nocx-xn63t.4.6).
//
// WHY THIS TEST IS HERE AT ALL, and not in the stands that already cover the
// close. Every half of this feature is green on its own and each of those
// halves hands ITSELF the seam it needs: internal/app's close path announces
// (worker_close_tab_test.go), internal/transport sends what it is given
// (ws_worker_tabclosed_test.go), the renderer folds the fact into the strip
// (frontend's panes.test.ts). The one place the product WIRES those halves
// together is the composition root, and nothing watched it — so a closer built
// with no announcer passed every test in this repository while the owner's
// stand showed a closed worker's tab still standing, red card and all, twice
// (2026-09-17 and again 2026-10-18, after the close path itself had landed).
//
// So this test drives what New BUILT, and nothing this test constructed: the
// app's own closer, the app's own layout chain, the app's own transport, and a
// real socket a renderer is holding. It is the local half of "watch the real
// order"; the browser half is the frontend's own test and the e2e suite.
//
// WHAT IS THIS TEST'S OWN, stated rather than hidden: the participant. A real
// spawn needs a live agent CLI on PATH and the helper-backed opener that
// internal/app's spawn stands hand-wire, so the two facts a spawn would have
// produced are written here — the tab row (through the SAME tabs.create a
// renderer sends) and the participant-to-tab pairing (the write workers.go's
// workerSpawner performs the moment the row exists). Everything after them is
// the product's, which is the half that was broken.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/shady2k/nocx/internal/content"
	"github.com/shady2k/nocx/internal/storage/storagetest"
	"github.com/shady2k/nocx/internal/workers"
)

// closeFrameBound is how long the test waits for the notification before it
// reports that none came. It is a bound on a BLOCKING SOCKET READ, not a wait
// for a state to become true: the frame is either queued by the time the close
// returns or it is never coming, and a gorilla connection is permanently
// failed by any read error, so the wait cannot be polled in pieces.
const closeFrameBound = 10 * time.Second

func TestClosingAParticipantTellsAConnectedRenderer(t *testing.T) {
	ctx := context.Background()
	storagetest.Isolate(t)
	a, err := newTestApp(t)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if startErr := a.Start(ctx); startErr != nil {
		t.Fatalf("Start: %v", startErr)
	}
	conn := dialAppWS(t, a)
	defer func() { _ = conn.Close() }()

	// The tab the participant's pane was minted in, created over the same
	// socket a renderer creates one through: the close is a statement about a
	// row the chain holds, and a tab seeded behind the chain's back would not
	// be one. The ids are UUIDv7 because that is the shape the renderer mints
	// them in and the chain's own validator refuses anything else.
	tabID := uuid.Must(uuid.NewV7()).String()
	paneID := uuid.Must(uuid.NewV7()).String()
	createTabOverWS(t, conn, 1, content.DefaultWorkspaceID, tabID, paneID)

	// The pairing workers.spawn writes when it mints that tab (workers.go's
	// workerSpawner, "WHERE THE TAB IS, RECORDED THE MOMENT IT EXISTS").
	const participant = workers.ParticipantID("p-tab-close")
	a.workerClose.tabs.record(participant, tabID)

	// A FINISHED participant, which is the ordinary case a coordinator closes
	// and the one the owner's stand hit: the process is gone, so there is no
	// session left in the registry, and the participant's PLACE is the whole
	// of what a close has left to take.
	if _, closeErr := a.workerClose.Close(ctx, workers.Participant{ID: participant}); closeErr != nil {
		t.Fatalf("closing a participant whose session is already gone: %v", closeErr)
	}

	// THE WIRE: the window that is looking at the strip is told, off the
	// socket it is holding, which tab left.
	if readErr := conn.SetReadDeadline(time.Now().Add(closeFrameBound)); readErr != nil {
		t.Fatalf("SetReadDeadline: %v — the wait would be unbounded", readErr)
	}
	var closed string
	for closed == "" {
		_, raw, readErr := conn.ReadMessage()
		if readErr != nil {
			t.Fatalf("closing a participant never told the connected renderer: %v", readErr)
		}
		var frame struct {
			Method string `json:"method"`
			Params struct {
				TabID string `json:"tabId"`
			} `json:"params"`
		}
		if json.Unmarshal(raw, &frame) != nil || frame.Method != "workers.tabClosed" {
			continue
		}
		closed = frame.Params.TabID
	}
	if closed != tabID {
		t.Fatalf("workers.tabClosed named tab %q, want the closed participant's own %q", closed, tabID)
	}

	// AND THE STORE, read back over the same socket: the tab the frame named
	// is out of the window the renderer draws from.
	read := callAppWS(t, conn, "layout.read", map[string]any{}, 2)
	if read.Error != nil {
		t.Fatalf("layout.read: %+v", read.Error)
	}
	var chain struct {
		Tabs []struct {
			ID string `json:"id"`
		} `json:"tabs"`
	}
	if unmarshalErr := json.Unmarshal(read.Result, &chain); unmarshalErr != nil {
		t.Fatalf("decode layout.read: %v", unmarshalErr)
	}
	ids := make([]string, 0, len(chain.Tabs))
	for _, tab := range chain.Tabs {
		ids = append(ids, tab.ID)
		if tab.ID == tabID {
			t.Fatalf("the closed participant's tab is still in the window after workers.close: %v", ids)
		}
	}
	if len(ids) == 0 {
		t.Fatalf("the window holds nothing at all after the close: %v", ids)
	}
}
