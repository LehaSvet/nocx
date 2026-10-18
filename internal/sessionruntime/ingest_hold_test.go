package sessionruntime

import (
	"testing"
	"time"

	"github.com/shady2k/nocx/internal/emulator"
	"github.com/shady2k/nocx/internal/emulator/ghostty"
)

// blockingEmulator stops one feed inside the runtime's critical section, so
// the IngestHold record can be read while the lock really is held by this
// call — and then released, so the record can be read after it is not.
// Everything else is the real adapter's, embedded.
type blockingEmulator struct {
	emulator.Terminal
	entered chan struct{}
	release chan struct{}
}

func (b *blockingEmulator) Ingest(p []byte) ([]byte, error) {
	b.entered <- struct{}{}
	<-b.release
	return b.Terminal.Ingest(p)
}

// IngestHold is open exactly while an Ingest call is inside Session.mu, and it
// reports the feed that call was handed — so a report can say "this call
// holds the lock" instead of "this call has not returned" (nocx-xn63t.6.47).
func TestIngestHoldIsOpenOnlyWhileAnIngestIsInsideTheLock(t *testing.T) {
	inner, stop, err := newBlockingSession()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	s := inner.session

	if _, held := s.IngestHold(); held {
		t.Fatal("an idle session reported an open Ingest hold")
	}

	done := make(chan error, 1)
	go func() { done <- s.Ingest([]byte("hi")) }()

	select {
	case <-inner.em.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the feed never reached the emulator")
	}
	hold, held := s.IngestHold()
	if !held {
		t.Fatal("a call inside the critical section reported no hold")
	}
	if hold.FeedBytes != 2 {
		t.Fatalf("hold FeedBytes = %d, want 2", hold.FeedBytes)
	}
	if hold.Entered.IsZero() {
		t.Fatal("the hold carries no entry time")
	}

	close(inner.em.release)
	if err := <-done; err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, held := s.IngestHold(); held {
		t.Fatal("the hold was still open after the call returned")
	}

	// Not sticky: a second call opens and closes its own record.
	inner.em.entered = make(chan struct{}, 1)
	inner.em.release = make(chan struct{})
	close(inner.em.release)
	inner.em.entered <- struct{}{}
	go func() { _ = s.Ingest([]byte("again")) }()
	<-inner.em.entered
	if _, held := s.IngestHold(); held {
		t.Fatal("the second call's hold outlived its return")
	}
}

// blockingSession is the runtime over the blocking emulator: the real
// terminal side, the scripted screen side.
type blockingSession struct {
	session *Session
	em      *blockingEmulator
}

func newBlockingSession() (*blockingSession, func(), error) {
	g := Geometry{Cols: 120, Rows: 40, CellWidthPx: 7, CellHeightPx: 16}
	term := newHarnessTerminal(g)
	screen, err := ghostty.New(g)
	if err != nil {
		return nil, nil, err
	}
	em := &blockingEmulator{Terminal: screen, entered: make(chan struct{}, 1), release: make(chan struct{})}
	s, err := New(Config{
		Incarnation:  Incarnation{Session: "ingest-hold", Generation: 1},
		Geometry:     g,
		Terminal:     term,
		Emulator:     em,
		Completeness: CompletenessComplete,
		Replies:      directReplySink{term: term},
	})
	if err != nil {
		return nil, nil, err
	}
	return &blockingSession{session: s, em: em}, screen.Close, nil
}
