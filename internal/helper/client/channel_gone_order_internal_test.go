package client

import (
	"io"
	"testing"
)

// TestAStreamWhoseTransportWentIsGoneBeforeItIsDone pins the order finish
// publishes its two facts in. A reader blocked on the stream wakes on done and
// reports the end (Dial answers ErrLost from exactly that), so a caller that
// then asks the lane's Done must already find it closed: the transport going
// is a fact about the end, not something that arrives after it.
func TestAStreamWhoseTransportWentIsGoneBeforeItIsDone(t *testing.T) {
	for i := range 5000 {
		s := &ChannelStream{data: newStream(), done: make(chan struct{}), gone: make(chan struct{})}
		observed := make(chan bool)
		go func() {
			<-s.done
			select {
			case <-s.gone:
				observed <- true
			default:
				observed <- false
			}
		}()
		s.finish(io.EOF, endOfStream{})
		if !<-observed {
			t.Fatalf("iteration %d: done closed while gone was still open, so a reader that saw the end could ask Done and be told the transport is still there", i)
		}
	}
}
