package shellintegration

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/shady2k/nocx/internal/waittest"
)

// transcript is one process on a real pty, plus everything the pty has
// carried: what a person watching the session sees, before any terminal
// decides what to paint (stripPrivateOSC is what that decision looks like for
// the private OSC markers).
//
// It exists so the two defects of nocx-1ebrs.22/.23 can be stated the way the
// owner stated them — "this is what the terminal showed me" — instead of as
// assertions about a reader. Both tests below need the same three things: the
// bytes, a wait that is anchored on a byte rather than on a sleep, and a
// process whose exit is an event.
type transcript struct {
	t      *testing.T
	cmd    *exec.Cmd
	ptmx   *os.File
	mu     sync.Mutex
	out    []byte
	pumped chan struct{}
	path   string
}

// startTranscript starts cmd on a pty of its own and pumps its output.
func startTranscript(t *testing.T, cmd *exec.Cmd, path string) *transcript {
	t.Helper()
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("start %s on a pty: %v", path, err)
	}
	tr := &transcript{t: t, cmd: cmd, ptmx: ptmx, pumped: make(chan struct{}), path: path}
	go func() {
		defer close(tr.pumped)
		buf := make([]byte, 8192)
		for {
			n, rerr := ptmx.Read(buf)
			if n > 0 {
				tr.mu.Lock()
				tr.out = append(tr.out, buf[:n]...)
				tr.mu.Unlock()
			}
			if rerr != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	return tr
}

// text is everything the pty has carried so far.
func (tr *transcript) text() string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return string(tr.out)
}

// visible is the transcript as a terminal paints it: every private OSC string
// removed, because a terminal consumes those whole and draws none of them.
func (tr *transcript) visible() string { return stripPrivateOSC(tr.text()) }

// waitFor blocks until the transcript satisfies cond. The detail it prints on
// failure is the VISIBLE transcript, so a red test names what the person
// would have seen rather than the wrapper that hid it.
func (tr *transcript) waitFor(what string, cond func(string) bool) {
	tr.t.Helper()
	waittest.WaitForTimeoutDetail(tr.t, what, 30*time.Second,
		func() string { return tr.path + " transcript: " + tr.visible() },
		func() bool { return cond(tr.text()) })
}

// write types into the session.
func (tr *transcript) write(s string) {
	tr.t.Helper()
	if _, err := tr.ptmx.Write([]byte(s)); err != nil {
		tr.t.Fatalf("write %q: %v", s, err)
	}
}

// waitExit waits for the process to end and for the pump to see everything it
// wrote before it died. Both are events: the exit status, and the pty's own
// end of file.
func (tr *transcript) waitExit() error {
	tr.t.Helper()
	err := tr.cmd.Wait()
	<-tr.pumped
	return err
}

// noDebuggerLines asserts the invariant of nocx-1ebrs.23: a shell nocx starts
// prints nothing its own startup would not print. bash prints exactly these
// two lines when it is asked to start the debugger and bashdb is not
// installed, and they reached a person's transcript once already.
func (tr *transcript) noDebuggerLines() {
	tr.t.Helper()
	got := tr.text()
	for _, line := range []string{"bashdb", "cannot start debugger"} {
		if strings.Contains(got, line) {
			tr.t.Errorf("the transcript carries bash's debugger complaint (%q):\n%s", line, tr.visible())
		}
	}
}
