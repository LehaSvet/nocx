package session

// The ingest watchdog (nocx-xn63t.6.47).
//
// A PTY FEED THAT DOES NOT RETURN HOLDS THE SESSION'S LOCK FOR ITS WHOLE
// CALL. Session.Ingest takes Session.mu on entry and releases it on return,
// so a feed that stalls stops every lifecycle completion and every screen
// subscriber behind it — and the only evidence such an event has left is a
// single SIGQUIT sample, taken a minute later, showing the ingest somewhere
// inside the emulator. That sample cannot say whether the loop was WORKING
// (a pathological input, a cost that grew) or was NOT RUNNING (the process
// was not scheduled, or the call blocked) — and those two need opposite
// fixes.
//
// WHAT THIS REPORTS, AND WHAT EACH NUMBER IS. The evidence is the samples:
// the stacks, and whether the capture's row index in those stacks ADVANCES
// from one sample to the next. Everything else is a fact about the same
// moment, each with its own exact meaning:
//
//   call_ms        how long the Ingest CALL has been in flight, measured by
//                  the caller. A call in flight is NOT the same fact as the
//                  lock being held: one queued behind another holder is in
//                  flight and owns nothing.
//   ingest_hold_ms REPORTED ONLY WHILE THE CALL IS INSIDE THE CRITICAL
//                  SECTION. It comes from Session.IngestHold, which the
//                  runtime writes after acquiring Session.mu and clears
//                  before releasing it (internal/sessionruntime), so its
//                  presence means an Ingest call — the holder of the lock,
//                  not a waiter for it — entered that long ago. Absent means
//                  no Ingest holds the lock at this sample: the call is
//                  queued (its stack shows sync.Mutex.Lock under
//                  Session.Ingest) or between two calls.
//   feed_bytes     the length of the feed the call was handed, so a minute is
//                  never mistaken for a large feed.
//   thread_cpu_ms  CPU consumed by the OS THREAD the call began on, over the
//                  same window (Linux: /proc/self/task/<tid>/stat, read at
//                  10 ms granularity). This is the closest thing to the
//                  holder's own CPU that is readable here: a cgo call keeps
//                  its goroutine on its M, so a capture loop that is working
//                  shows on that thread. It is still the THREAD's number —
//                  a goroutine may be migrated between two cgo calls, and a
//                  thread may run other work — so it is context for the
//                  stacks, never a verdict on its own. Darwin has no
//                  per-thread counter this package reads, and the field is
//                  absent there.
//   proc_cpu_ms    the whole PROCESS's CPU over the same window. COARSE, AND
//                  COARSE ONLY: every other goroutine and every other
//                  session this helper serves is inside it. It says "the
//                  process was not idle"; it cannot say the holder worked,
//                  and it is never the only number a reader is given.
//   sample         which observation this is (bounded, below).
//
// THE AMBIGUITY THIS LEAVES, stated rather than papered over: nothing here
// separates "one cgo call blocked inside the library" from "the process
// stopped being scheduled" when the stacks are identical and the numbers are
// zero. Two samples apart are what narrow it — an advancing row index rules
// out both, and a frozen index with a rising thread CPU names a loop that is
// working on one call, while a frozen index and a flat thread CPU leaves the
// blocked-or-not-running pair standing.
//
// It costs two atomic stores and one Gettid per ingest and one timer per
// session, and it never fires unless an ingest outlives ingestWatchAfter.
// The thresholds are deliberately generous: an ordinary feed at 120x40
// measures under 12 ms even with the live scrollback pinned at its
// 10,000-line budget (measured 2026-10-18, 520 feeds of 100 rows: p99 9.7 ms,
// max 11.7 ms).

import (
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/shady2k/nocx/internal/log"
)

const (
	// ingestWatchAfter is how long one ingest may run before the first
	// observation. Two orders of magnitude above the ordinary worst feed,
	// and far below the minute-long holds this exists to explain.
	ingestWatchAfter = 5 * time.Second
	// ingestWatchEvery is the gap between observations after the first.
	ingestWatchEvery = 5 * time.Second
	// ingestWatchDumps bounds one hold's observations: three samples cover
	// the first fifteen seconds, which is enough to see progress and CPU
	// without turning a pathological hour into a pathological log.
	ingestWatchDumps = 3
	// ingestWatchStackBytes bounds one all-goroutine dump. A helper serving
	// one pane is a few dozen goroutines; the cap is for the one that is not.
	ingestWatchStackBytes = 1 << 20
	// ingestWatchFrameBytes bounds the frames reported per goroutine: the
	// part that names what the goroutine is doing, without the megabytes a
	// full traceback of a deep call tree can carry.
	ingestWatchFrameBytes = 2 << 10
)

// ingestWatch observes the one ingest this owner has in flight.
type ingestWatch struct {
	started atomic.Int64 // UnixNano the ingest began; 0 when none is in flight
	bytes   atomic.Int64 // the feed's own length
	tid     atomic.Int64 // OS thread the ingest began on; 0 when unknown
	tcpu    atomic.Int64 // that thread's CPU milliseconds at begin, or -1
	pcpu    atomic.Int64 // process CPU milliseconds at begin, or -1
	dumped  atomic.Int32 // observations taken for this hold

	// now is the clock a test replaces; nil means time.Now.
	now func() time.Time
}

func (w *ingestWatch) clock() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

// begin records the ingest starting, on the ingest's own goroutine: the
// thread it is on is read HERE, because only this goroutine is on it. Every
// baseline is stored before the start marker, so a sampler that sees a start
// never sees a zero baseline.
func (w *ingestWatch) begin(n int) {
	tid := currentThreadID()
	tcpu, tok := threadCPUMillis(tid)
	if !tok {
		tcpu = -1 // absent, not zero: see cpuSince's own comment
	}
	pcpu, pok := processCPUMillis()
	if !pok {
		pcpu = -1
	}
	w.tid.Store(int64(tid))
	w.tcpu.Store(tcpu)
	w.pcpu.Store(pcpu)
	w.bytes.Store(int64(n))
	w.dumped.Store(0)
	w.started.Store(w.clock().UnixNano())
}

// end records the ingest returning, whatever it returned.
func (w *ingestWatch) end() {
	w.started.Store(0)
}

// held answers how long the ingest in flight has been running, and whether
// there is one at all.
func (w *ingestWatch) held() (time.Duration, bool) {
	started := w.started.Load()
	if started == 0 {
		return 0, false
	}
	return w.clock().Sub(time.Unix(0, started)), true
}

// sample takes one observation if the call in flight has run past its next
// threshold and the budget is not spent, and reports whether it logged.
// ingestHold answers the runtime's own record of the critical section
// (Session.IngestHold), which is the only thing here that says the call is
// the holder rather than a waiter.
func (w *ingestWatch) sample(lg log.Logger, ingestHold func() (time.Duration, bool)) bool {
	held, ok := w.held()
	if !ok || lg == nil {
		return false
	}
	taken := w.dumped.Load()
	if taken >= ingestWatchDumps {
		return false
	}
	if held < ingestWatchAfter+time.Duration(taken)*ingestWatchEvery {
		return false
	}
	if !w.dumped.CompareAndSwap(taken, taken+1) {
		return false
	}
	attrs := []any{
		"call_ms", held.Milliseconds(),
		"feed_bytes", w.bytes.Load(),
		"goroutines", runtime.NumGoroutine(),
		"sample", taken + 1,
	}
	if ingestHold != nil {
		if inside, ok := ingestHold(); ok {
			attrs = append(attrs, "ingest_hold_ms", inside.Milliseconds())
		}
	}
	if tid := w.tid.Load(); tid > 0 {
		if cpu, ok := threadCPUMillis(int(tid)); ok {
			if delta, known := cpuSince(w.tcpu.Load(), cpu); known {
				attrs = append(attrs, "thread_cpu_ms", delta, "thread", tid)
			}
		}
	}
	if cpu, ok := processCPUMillis(); ok {
		if delta, known := cpuSince(w.pcpu.Load(), cpu); known {
			attrs = append(attrs, "proc_cpu_ms", delta)
		}
	}
	lg.Warn("session owner: the runtime ingest call is still in flight", attrs...)
	for _, g := range sessionStacks() {
		lg.Warn("session owner: ingest watchdog stack",
			"goroutine", g.header, "frames", g.frames)
	}
	return true
}

// goroutineTrace is one goroutine of a dump: its header line (which carries
// the state and how long it has waited in it) and its bounded frames.
type goroutineTrace struct {
	header string
	frames string
}

// sessionStacks reads every goroutine's stack and returns the ones that can
// bear on a held session lock: the runtime, the emulator, the PTY, this
// package, the host's own read loop (parked in Read is "nothing arrived",
// which is a different fact from "dispatched and waiting"), and anything
// waiting on a mutex (a waiter names the hold from the other side).
func sessionStacks() []goroutineTrace {
	buf := make([]byte, ingestWatchStackBytes)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) || len(buf) >= 8*ingestWatchStackBytes {
			// Either the whole dump fits, or a dump this large is not going
			// to be read whole: what is in hand is what is reported.
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	var out []goroutineTrace
	for _, block := range strings.Split(string(buf), "\n\n") {
		if strings.TrimSpace(block) == "" || !interestingStack(block) {
			continue
		}
		lines := strings.Split(block, "\n")
		header := lines[0]
		frames := block
		if len(frames) > ingestWatchFrameBytes {
			frames = frames[:ingestWatchFrameBytes] + "… (truncated)"
		}
		out = append(out, goroutineTrace{header: header, frames: frames})
	}
	return out
}

// interestingStack answers whether one goroutine's block names a participant
// in a session hold. It is a filter over TEXT, deliberately: a dump is all
// there is to read, and matching symbols by name is what a reader would do.
func interestingStack(block string) bool {
	for _, marker := range []string{
		"internal/sessionruntime",
		"internal/emulator",
		"internal/pty",
		"internal/helper/session",
		"internal/helper/host",
		"sync.Mutex.Lock",
	} {
		if strings.Contains(block, marker) {
			return true
		}
	}
	return false
}

// cpuSince is the difference between a baseline and now, and whether that is
// measurable at all. A negative baseline means the begin could not read the
// counter: the number is then omitted rather than reported as a zero that
// reads like "nothing ran".
func cpuSince(baseline, now int64) (int64, bool) {
	if baseline < 0 {
		return 0, false
	}
	return now - baseline, true
}
