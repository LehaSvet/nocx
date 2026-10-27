//go:build linux

package shellintegration

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// Every opened master must be paired with its own slave, including while GC
// shrinks the opening goroutine's stack. A lost TIOCGPTN result otherwise names
// /dev/pts/0; Start then tries to acquire someone else's controlling terminal.
// Regression for creack/pty#215, based on its upstream #216 reproducer.
// This is probabilistic GC stress with a fixed 32768-open budget, not a
// deterministic schedule. It belongs in CI as a cheap dependency-regression
// signal; a green run is not proof the race is absent.
func TestPTYOpenKeepsItsSlaveUnderStackShrink(t *testing.T) {
	held, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = slave.Close(); _ = held.Close() }()

	var stop atomic.Bool
	var gc sync.WaitGroup
	gc.Add(1)
	go func() {
		defer gc.Done()
		for !stop.Load() {
			runtime.GC()
		}
	}()
	var workers sync.WaitGroup
	var opened atomic.Int64
	var mismatches atomic.Int64
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 4096 {
				ptyGrowStack(2000)
				master, tty, openErr := pty.Open()
				if openErr != nil {
					t.Errorf("pty.Open: %v", openErr)
					stop.Store(true)
					return
				}
				opened.Add(1)
				index, indexErr := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
				var stat unix.Stat_t
				statErr := unix.Fstat(int(tty.Fd()), &stat)
				if indexErr != nil || statErr != nil {
					t.Errorf("read pair identity: index=%v stat=%v", indexErr, statErr)
					stop.Store(true)
				} else if want := fmt.Sprintf("/dev/pts/%d", index); tty.Name() != want || int64(unix.Minor(stat.Rdev)) != int64(index) {
					mismatches.Add(1)
					t.Errorf("master index=%d tty name=%s slave minor=%d\n%s", index, tty.Name(), unix.Minor(stat.Rdev), ptyFailureState())
				}
				_ = tty.Close()
				_ = master.Close()
			}
		}()
	}
	workers.Wait()
	stop.Store(true)
	gc.Wait()
	t.Logf("checked %d PTY pairs under GC stack-shrink pressure; mismatches=%d", opened.Load(), mismatches.Load())
}

//go:noinline
func ptyGrowStack(depth int) byte {
	var pad [8]byte
	pad[depth&7] = byte(depth)
	if depth > 0 {
		pad[0] += ptyGrowStack(depth - 1)
	}
	return pad[depth&7]
}

func ptyFailureState() string {
	var out strings.Builder
	for _, path := range []string{"/proc/self/status", "/proc/self/limits", "/proc/self/cgroup", "/sys/fs/cgroup/pids.current", "/sys/fs/cgroup/pids.max", "/sys/fs/cgroup/pids.events"} {
		content, err := os.ReadFile(path) // #nosec G304 — fixed proc/cgroup allowlist above, not caller input.
		fmt.Fprintf(&out, "%s: %s (read error: %v)\n", path, content, err)
	}
	return out.String()
}
