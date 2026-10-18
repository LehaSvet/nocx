//go:build !linux && !darwin

package session

// currentThreadID answers 0 where this platform's thread identity is not
// read here, and the two CPU counters answer that they are unavailable. The
// watchdog then names what it can measure — the hold, the feed and the
// stacks — and omits the numbers rather than reporting zeros that would read
// like "nothing ran".
func currentThreadID() int { return 0 }

func processCPUMillis() (int64, bool) { return 0, false }

func threadCPUMillis(int) (int64, bool) { return 0, false }
