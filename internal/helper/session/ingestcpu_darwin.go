//go:build darwin

package session

import "golang.org/x/sys/unix"

// currentThreadID answers 0: x/sys/unix exposes no thread identity on darwin,
// and no per-thread CPU counter is read here either (threadCPUMillis below),
// so the watchdog names neither rather than inventing one. The thread it
// would name is only ever used to attribute a CPU number.
func currentThreadID() int { return 0 }

// processCPUMillis is this whole process's CPU time, user plus system —
// coarse context, and the one CPU number this platform gives the watchdog at
// all.
func processCPUMillis() (int64, bool) {
	var ru unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &ru); err != nil {
		return 0, false
	}
	return ru.Utime.Sec*1000 + int64(ru.Utime.Usec)/1000 + ru.Stime.Sec*1000 + int64(ru.Stime.Usec)/1000, true
}

// threadCPUMillis reports that one thread's CPU time is not readable here.
// The watchdog then reports the process's own number, labelled as coarse, and
// says nothing it did not measure.
func threadCPUMillis(int) (int64, bool) { return 0, false }
