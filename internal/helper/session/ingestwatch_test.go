package session

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/shady2k/nocx/internal/log"
)

func testLogger(buf *bytes.Buffer) log.Logger {
	return log.NewSlogAdapter(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
}

// A call past the threshold is reported WITH its own numbers and the stacks,
// and a call inside it is not (nocx-xn63t.6.47). The clock is the test's, so
// the assertion is about the watchdog's rule and never about a duration this
// machine happened to take (AGENTS.md: a test may not depend on timing).
func TestTheIngestWatchdogNamesALongCallWithItsNumbersAndStacks(t *testing.T) {
	var buf bytes.Buffer
	lg := testLogger(&buf)
	now := time.Unix(1_700_000_000, 0)
	w := &ingestWatch{now: func() time.Time { return now }}

	w.begin(2100)
	if w.sample(lg, nil) {
		t.Fatalf("a fresh ingest was reported as a long call:\n%s", buf.String())
	}

	now = now.Add(ingestWatchAfter)
	if !w.sample(lg, func() (time.Duration, bool) { return 4 * time.Second, true }) {
		t.Fatalf("an ingest in flight for %s was not reported", ingestWatchAfter)
	}
	out := buf.String()
	for _, want := range []string{
		// The call, not the lock: ownership comes from the runtime's record.
		"the runtime ingest call is still in flight",
		"call_ms=5000",
		"feed_bytes=2100",
		"ingest_hold_ms=4000",
		"sample=1",
		"goroutines=",
		"ingest watchdog stack",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not carry %q:\n%s", want, out)
		}
	}
	// The process's CPU is reported where the platform can read it, and its
	// absence is a statement rather than a zero.
	if _, ok := processCPUMillis(); ok && !strings.Contains(out, "proc_cpu_ms=") {
		t.Errorf("a readable process CPU was not reported:\n%s", out)
	}

	// A call that is NOT inside the critical section carries no hold: the
	// line says what was measured, and nothing about a lock it did not see.
	now = now.Add(ingestWatchEvery)
	buf.Reset()
	if !w.sample(lg, func() (time.Duration, bool) { return 0, false }) {
		t.Fatal("the second observation was not taken")
	}
	if strings.Contains(buf.String(), "ingest_hold_ms") {
		t.Errorf("a call outside the critical section reported a hold:\n%s", buf.String())
	}

	// The budget is bounded: the third observation is the last one, so a
	// pathological hour cannot become a pathological log.
	now = now.Add(ingestWatchEvery)
	if !w.sample(lg, nil) {
		t.Fatal("the third observation was not taken")
	}
	now = now.Add(ingestWatchEvery)
	if w.sample(lg, nil) {
		t.Fatal("the watchdog reported a call past its own sample budget")
	}

	// An ingest that returned is nobody's call, however long it was.
	w.end()
	now = now.Add(time.Hour)
	if w.sample(lg, nil) {
		t.Fatal("a finished ingest was still reported as in flight")
	}
}

// The stacks the watchdog reports are the ones that can bear on a session
// hold: this package's own goroutine (the owner's, in production) is in the
// filter, so a real report names the participants rather than only numbers.
func TestTheIngestWatchdogReportsTheGoroutinesThatCanHoldTheLock(t *testing.T) {
	stacks := sessionStacks()
	if len(stacks) == 0 {
		t.Fatal("no goroutine in this process matched the session filter")
	}
	named := false
	for _, g := range stacks {
		if strings.Contains(g.frames, "internal/helper/session") {
			named = true
		}
		if !strings.Contains(g.header, "goroutine ") {
			t.Errorf("a reported goroutine has no header: %q", g.header)
		}
	}
	if !named {
		t.Fatalf("the filter dropped this package's own goroutines: %+v", stacks)
	}
}

// Where this platform can read one thread's CPU, the report carries the
// number for the thread the call began on, named beside the process's own so
// neither stands in for the other.
func TestTheIngestWatchdogReadsTheThreadItBeganOn(t *testing.T) {
	var buf bytes.Buffer
	lg := testLogger(&buf)
	now := time.Unix(1_700_000_000, 0)
	w := &ingestWatch{now: func() time.Time { return now }}
	w.begin(1)
	defer w.end()
	if _, ok := threadCPUMillis(int(w.tid.Load())); !ok {
		t.Skip("this platform has no per-thread CPU counter; the report omits it")
	}
	now = now.Add(ingestWatchAfter)
	if !w.sample(lg, nil) {
		t.Fatal("the observation was not taken")
	}
	out := buf.String()
	if !strings.Contains(out, "thread_cpu_ms=") || !strings.Contains(out, "thread=") {
		t.Errorf("the thread's own CPU was not reported:\n%s", out)
	}
}
