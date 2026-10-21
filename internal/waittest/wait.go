// Package waittest provides polling helpers for asynchronous tests.
package waittest

import (
	"testing"
	"time"
)

const (
	// DefaultTimeout is the ceiling used by WaitFor. Tests with an unusually
	// slow external fixture can use WaitForTimeout with an explicit ceiling.
	DefaultTimeout = 5 * time.Second
	pollInterval   = 10 * time.Millisecond
	// deadlineMargin is how long before the test's own deadline
	// WaitForHealthy reports instead of letting the package timeout kill the
	// run: the failure's detail needs those seconds to reach the log.
	deadlineMargin = 5 * time.Second
)

// WaitFor polls cond until it returns true. It fails the test if the condition
// is not observed within DefaultTimeout; what is included in that failure so
// the missing state is actionable.
func WaitFor(t testing.TB, what string, cond func() bool) {
	t.Helper()
	waitFor(t, what, DefaultTimeout, cond)
}

// WaitForTimeout is WaitFor with an explicit timeout for genuinely slow test
// fixtures. Polling remains internal so callers wait on state, not a duration.
func WaitForTimeout(t testing.TB, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	waitFor(t, what, timeout, cond)
}

// WaitForHealthy polls cond until it returns true, and fails the test the
// moment healthy returns false. It is the wait AGENTS.md's own rule asks for —
// wait on an observable state change, never on a duration — with a breaker:
// the wait ends when the condition arrives, when the waiter is SEEN broken
// (a pane whose session left the registry, a delivery whose record reached a
// terminal refusal — whatever the caller's healthy names), or at the test's
// own deadline, which is the package's bound and not a ceiling standing in
// for the observable. A wall clock that decides a healthy-but-slow waiter's
// fate is the load-flake class this package keeps meeting (nocx-1ebrs.34,
// nocx-k3d2f): on a loaded runner the same wait fired on green pipelines
// because the ceiling, not the condition, decided. healthy must answer a
// STATE and never a staleness: a heartbeat read needs a wall clock, and a
// wall clock is what this wait is forgoing.
func WaitForHealthy(t testing.TB, what string, detail func() string, healthy func() bool, cond func() bool) {
	t.Helper()
	for {
		if cond() {
			return
		}
		if !healthy() {
			t.Fatalf("gave up on %s: the waiter was seen broken; %s", what, detail())
			return
		}
		// The test's own deadline (go test -timeout) is the last backstop.
		// testing.TB does not carry it, so this reads it off whatever does —
		// a plain *testing.T — and skips it where none exists.
		if td, ok := t.(interface{ Deadline() (time.Time, bool) }); ok {
			if d, has := td.Deadline(); has && time.Until(d) < deadlineMargin {
				t.Fatalf("timed out waiting for %s at the test's own deadline; %s", what, detail())
				return
			}
		}
		time.Sleep(pollInterval)
	}
}

// WaitForDetail is WaitFor with state attached to the failure. detail is
// called only when the wait times out, so the failure can say what the system
// was actually doing and not merely what it was waiting for — the accepted
// events, the output so far, the stages that did arrive. A timeout that names
// only its own description cannot be told apart from a starved machine, which
// is the judgement this suite has to make on every red container run.
func WaitForDetail(t testing.TB, what string, detail func() string, cond func() bool) {
	t.Helper()
	waitForDetail(t, what, DefaultTimeout, detail, cond)
}

// WaitForTimeoutDetail is WaitForDetail with an explicit ceiling.
func WaitForTimeoutDetail(t testing.TB, what string, timeout time.Duration, detail func() string, cond func() bool) {
	t.Helper()
	waitForDetail(t, what, timeout, detail, cond)
}

func waitFor(t testing.TB, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	waitForDetail(t, what, timeout, nil, cond)
}

func waitForDetail(t testing.TB, what string, timeout time.Duration, detail func() string, cond func() bool) {
	// Without this the timeout is reported against this file rather than the
	// caller's line, which would defeat the whole point of naming `what`.
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if timeout <= 0 || !time.Now().Before(deadline) {
			if detail != nil {
				t.Fatalf("timed out waiting for %s; %s", what, detail())
			}
			t.Fatalf("timed out waiting for %s", what)
			return
		}
		delay := pollInterval
		if remaining := time.Until(deadline); remaining < delay {
			delay = remaining
		}
		if delay > 0 {
			time.Sleep(delay)
		}
	}
}
