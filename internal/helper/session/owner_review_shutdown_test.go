package session

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/shady2k/nocx/internal/sessionruntime"
)

// Pause the owner inside finishRead, after it has observed EOF but before it
// returns to its event loop. Unlike a sleep this fixes the exact lifetime edge.
type reviewEOFPauseProcess struct {
	*ownerFakeProcess
	atEOF   chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *reviewEOFPauseProcess) WakeReadiness() {
	p.once.Do(func() {
		close(p.atEOF)
		<-p.release
	})
}

func TestReviewShutdownResolvesAlreadyAdmittedIncoming(t *testing.T) {
	proc := &reviewEOFPauseProcess{
		ownerFakeProcess: newOwnerFakeProcess(),
		atEOF:            make(chan struct{}), release: make(chan struct{}),
	}
	owner, _ := newTestOwner(t, proc)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(proc.release) }) }
	t.Cleanup(func() { release(); owner.stop(true, time.Time{}) })
	go owner.run()
	proc.endRead(io.EOF)
	select {
	case <-proc.atEOF:
	case <-time.After(5 * time.Second):
		t.Fatal("owner never observed EOF")
	}
	frameDone, err := owner.submit(ownerItem{kind: itemClientFrame, payload: []byte("accepted")})
	if err != nil {
		t.Fatalf("submit ordinary input: %v", err)
	}
	bumpDone, err := owner.submit(ownerItem{kind: itemAccessBump, above: 9})
	if err != nil {
		t.Fatalf("submit access bump: %v", err)
	}
	owner.closeOnce.Do(func() { close(owner.closingSignal) })
	release()
	select {
	case <-owner.stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("owner did not stop")
	}
	select {
	case res := <-frameDone:
		if !errors.Is(res.Err, errOwnerClosing) || res.State != sessionruntime.IntentStateCancelled {
			t.Errorf("accepted frame = %+v, want cancelled/closing", res)
		}
	default:
		t.Error("owner stopped with accepted ordinary input unresolved")
	}
	select {
	case res := <-bumpDone:
		if res.Err != nil || res.Epoch <= 9 {
			t.Errorf("accepted bump = %+v, want epoch above 9", res)
		}
	default:
		t.Error("owner stopped with accepted access bump unresolved")
	}
	if got := owner.queueUsed.Load(); got != 0 {
		t.Errorf("queueUsed = %d after stop, want 0", got)
	}
	if got := proc.writtenPayloads(); len(got) != 0 {
		t.Errorf("shutdown wrote cancelled input: %q", got)
	}
}
