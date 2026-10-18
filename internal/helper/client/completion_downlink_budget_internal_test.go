package client

// A COMPLETION THE HELPER NEVER ANSWERS IS REPORTED LOST, NOT HELD FOR THE
// SESSION'S LIFE (nocx-uo8pf).
//
// Measured in CI (PR #372, ci-e2e chromium-1, run 39110077317, job
// 117624213613): the coordinator's kernel accepted command 440's completion at
// 08:27:52.151Z and the helper received it there — its host logged the
// request — but the helper's handler never answered: it was parked on the
// session runtime's lock, held by the PTY ingest of that same command's
// output, and the helper's own stacks put it there 59 seconds later. Every
// attempt of this downlink timed out at completionDeliveryTimeout and was
// retried, exactly as designed, until the harness killed the helper at
// teardown. What the person saw was a block that never froze; what the next
// attempt would have seen is the same thing, for as long as the pane lived.
//
// What is judged here is the carrier's own behaviour, off the wire: an
// accepted completion the helper does not answer is DELIVERED or REPORTED
// LOST, and it is reported lost inside the budget rather than when the
// session ends (nocx-2v80t.3.29's own answer for a boundary the helper will
// never hear of). The paired half is that a helper which answers late — but
// inside the budget — still lands, so the budget buys nothing by giving up
// early.

import (
	"context"
	"errors"
	"testing"
	"time"
)

// awaitLost waits for the downlink to report one accepted fact lost.
func awaitLost(t *testing.T, lost <-chan struct{}) {
	t.Helper()
	select {
	case <-lost:
	case <-time.After(5 * time.Second):
		t.Fatal("an accepted completion the helper never answered was neither delivered nor reported lost: " +
			"the block its boundary ends stays open for the session's whole life")
	}
}

// lostWatcher records that onLost was told, on the downlink's own goroutine.
func lostWatcher() (func(context.Context, string, [32]byte), <-chan struct{}) {
	ch := make(chan struct{}, 8)
	return func(context.Context, string, [32]byte) {
		ch <- struct{}{}
	}, ch
}

func TestAnUnansweredCompletionIsReportedLostInsideTheDeliveryBudget(t *testing.T) {
	spy := newSpy()
	unanswered := errors.New("helper: no answer inside one attempt")
	spy.fail = func(int, [32]byte) error { return unanswered }
	d, sl := newTestDownlink(t, spy)
	d.budget = 50 * time.Millisecond
	onLost, lost := lostWatcher()
	d.onLost = onLost
	d.Bind(HostSessionID{Generation: "gen-1", Session: "sess-1"})

	var fence [32]byte
	fence[0] = 0x12
	accept(t, d, fence, nil)

	awaitLost(t, lost)

	records := lostRecords(sl)
	if len(records) != 1 {
		t.Fatalf("%d lost record(s), want exactly 1: %#v", len(records), records)
	}
	cause := errOf(t, records[0])
	if !errors.Is(cause, errDeliveryBudget) {
		t.Fatalf("the lost completion's cause = %v, want it to name the delivery budget", cause)
	}
	if !errors.Is(cause, unanswered) {
		t.Fatalf("the lost completion's cause = %v, want the last attempt's own failure in the chain", cause)
	}
	if attempts := spy.attemptCount(); attempts < 2 {
		t.Fatalf("the completion was attempted %d time(s): a failure that might pass is still retried", attempts)
	}
}

func TestACompletionAnsweredLateButInsideTheBudgetStillLands(t *testing.T) {
	spy := newSpy()
	late := 3
	spy.fail = func(attempt int, _ [32]byte) error {
		if attempt < late {
			return errors.New("helper: not yet")
		}
		return nil
	}
	d, sl := newTestDownlink(t, spy)
	d.budget = 5 * time.Second
	d.onLost = func(context.Context, string, [32]byte) {
		t.Error("a completion answered inside the budget was reported lost")
	}
	d.Bind(HostSessionID{Generation: "gen-1", Session: "sess-1"})

	var fence [32]byte
	fence[0] = 0x34
	accept(t, d, fence, nil)

	awaitLanded(t, spy, 1)
	if delivered := spy.delivered(); len(delivered) != 1 || delivered[0] != fence {
		t.Fatalf("delivered = %x, want exactly %x", delivered, fence)
	}
	if records := lostRecords(sl); len(records) != 0 {
		t.Fatalf("%d lost record(s) for a completion that landed on attempt %d: %#v", len(records), late, records)
	}
}
