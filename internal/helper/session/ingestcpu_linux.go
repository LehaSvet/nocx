//go:build linux

package session

import (
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// currentThreadID answers the OS thread this call is on. It is read on the
// ingest's own goroutine at the start of the call, because only that
// goroutine is on that thread.
func currentThreadID() int { return unix.Gettid() }

// processCPUMillis is this whole process's CPU time, user plus system. It is
// coarse — every goroutine and every session this helper serves is inside
// it — and the watchdog reports it as context beside the thread's own number,
// never as the holder's CPU.
func processCPUMillis() (int64, bool) {
	var ru unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &ru); err != nil {
		return 0, false
	}
	return ru.Utime.Sec*1000 + ru.Utime.Usec/1000 + ru.Stime.Sec*1000 + ru.Stime.Usec/1000, true
}

// threadCPUMillis is one OS thread's CPU time, user plus system, from
// /proc/self/task/<tid>/stat. USER_HZ is 100 on the Linux this ships to, so
// the resolution is 10 ms — enough to tell a thread that worked for seconds
// from one that did not, and not enough for anything finer, which the hold
// this exists for (a minute and more) never needs.
//
// A cgo call keeps its goroutine on its M for the call's duration, so a
// capture loop that is working shows in the thread it started on; a goroutine
// may still be migrated between two cgo calls, which is why the number is
// named after the thread and not after the goroutine.
func threadCPUMillis(tid int) (int64, bool) {
	if tid <= 0 {
		return 0, false
	}
	raw, err := os.ReadFile("/proc/self/task/" + strconv.Itoa(tid) + "/stat")
	if err != nil {
		return 0, false
	}
	// The comm field is a bare string that may contain spaces and
	// parentheses, so the fields are counted from the LAST ')': the next
	// field is 3 (state), utime is 14 and stime is 15.
	i := strings.LastIndexByte(string(raw), ')')
	if i < 0 {
		return 0, false
	}
	fields := strings.Fields(string(raw[i+1:]))
	if len(fields) < 13 {
		return 0, false
	}
	utime, err := strconv.ParseInt(fields[11], 10, 64)
	if err != nil {
		return 0, false
	}
	stime, err := strconv.ParseInt(fields[12], 10, 64)
	if err != nil {
		return 0, false
	}
	return (utime + stime) * 10, true
}
