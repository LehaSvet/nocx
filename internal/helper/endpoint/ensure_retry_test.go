package endpoint_test

// A START IS NOT LOST TO A HELPER THAT LEAVES (nocx-xn63t.6.41).
//
// CI run 39057679352, ci-e2e (webkit, 1): one-input-per-pane.spec.ts timed out
// waiting for the prompt editor, and the pane read
//
//	Nocx installed this machine's helper and it did not start: local helper
//	f0eb82a262630a5c: endpoint: no helper is serving this generation:
//	/work/.e2e/home/.nocx/run/22-f0eb82a262630a5c.sock: dial unix …:
//	connect: no such file or directory.
//
// That sentence is ONE path of endpoint.Ensure returning a bare ErrNoEndpoint:
// the child it started ended with SUCCESS — a helper that finds another of its
// generation serving yields to it and exits 0 — and the dial that followed
// found no socket, because the peer it yielded to was a daemon retiring under
// D2 and had unlinked the endpoint as it went. The start was not lost because
// the binary failed. It was lost because the endpoint the start handed itself
// to went away, and Ensure believed the first dial.
//
// The two tests below pin both halves: a yield makes the start WAIT on the
// socket again (and a real helper takes the endpoint), and a start that keeps
// yielding ends in a refusal that names the yielding instead of a bare "nothing
// is serving" — never a hang.
//
// WHAT IS MODELLED RATHER THAN RE-DERIVED: the child's decision to yield. A
// real helper reaches it by probing the endpoint (alreadyServing), which is a
// fact about the child's own process that Ensure cannot observe — Ensure sees
// only "exited 0". So the wrapper below states it (its first run exits 0) and
// the test stages the state Ensure then sees: a real helper of this generation
// HAD been serving, and its endpoint is gone. Every wait is on an observable
// fact — the endpoint answering, the endpoint refusing, a process ending — and
// nothing waits on a duration.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shady2k/nocx/internal/helper/client"
	"github.com/shady2k/nocx/internal/helper/endpoint"
	"github.com/shady2k/nocx/internal/helper/proto"
)

// peerHelper is one real `nocx-helper serve` the test started, with the one
// way to end it: its own shutdown, so the endpoint is unlinked by the daemon
// that owns it rather than by the test.
type peerHelper struct {
	cmd  *exec.Cmd
	stop func()
}

// startPeer runs the real helper against a home of its own and waits until the
// endpoint ANSWERS — not until a duration has passed — so that "a helper of
// this generation is serving" is a checked fact and not a premise.
func startPeer(t *testing.T, bin, home string, generation proto.GenerationID) *peerHelper {
	t.Helper()
	cmd := exec.Command(bin, endpoint.ServeCommand) //nolint:gosec // bin is this test's own build output
	cmd.Env = append(os.Environ(), "HOME="+home)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the peer helper: %v", err)
	}

	var stopped bool
	var stopErr error
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		// An interrupt rather than a kill: the helper's own shutdown is what
		// closes the PTYs it holds and what unlinks its endpoint, so a SIGKILL
		// here would stage something the product never does.
		_ = cmd.Process.Signal(os.Interrupt)
		stopErr = cmd.Wait()
	}
	t.Cleanup(func() {
		stop()
		if stopErr != nil {
			t.Logf("the peer helper ended with %v", stopErr)
		}
	})

	waitForEndpoint(t, endpoint.Dir(home), generation, true)
	return &peerHelper{cmd: cmd, stop: stop}
}

// retirePeer ends it the way D2 ends a daemon — its own shutdown — and waits
// until the endpoint REFUSES, which is the fact the caller is left with.
func retirePeer(t *testing.T, peer *peerHelper, dir string, generation proto.GenerationID) {
	t.Helper()
	peer.stop()
	waitForEndpoint(t, dir, generation, false)
}

// waitForEndpoint waits until the endpoint can be dialled (serving) or until a
// dial gets ErrNoEndpoint (nothing serving). Both are facts about the socket;
// the bound is the test's own context, never a duration.
func waitForEndpoint(t *testing.T, dir string, generation proto.GenerationID, serving bool) {
	t.Helper()
	for {
		conn, err := endpoint.Dial(t.Context(), dir, generation)
		if serving && err == nil {
			_ = conn.Close()
			return
		}
		if !serving && errors.Is(err, endpoint.ErrNoEndpoint) {
			return
		}
		if err != nil && !errors.Is(err, endpoint.ErrNoEndpoint) {
			t.Fatalf("dial the endpoint: %v", err)
		}
		if t.Context().Err() != nil {
			t.Fatalf("the endpoint never became serving=%v: last dial = %v", serving, err)
		}
		time.Sleep(time.Millisecond)
	}
}

// yieldingWrapper writes a `nocx-helper serve` stand-in that YIELDS on its
// first yieldRuns invocations and then becomes the real helper: it records the
// invocation count, and on the runs after the yields it execs bin — the same
// shim TestEnsureStartsAHelperWhenNoneIsServing uses, because the helper
// detaches and the test has to be able to end it again.
func yieldingWrapper(t *testing.T, bin, home string, yieldRuns int) (wrapper, countFile, pidFile string) {
	t.Helper()
	dir := t.TempDir()
	wrapper = filepath.Join(dir, "start-helper")
	countFile = filepath.Join(dir, "runs")
	pidFile = filepath.Join(dir, "helper.pid")

	script := fmt.Sprintf(`#!/bin/sh
n=0
if [ -f %[1]q ]; then n=$(cat %[1]q); fi
n=$((n+1))
echo "$n" > %[1]q
if [ "$n" -le %[2]d ]; then
	# A helper of this generation is serving, so this process yields to it and
	# changes nothing — the exit 0 cmd/nocx-helper's serve takes when its probe
	# or its bind finds a peer.
	exit 0
fi
echo $$ > %[3]q
export HOME=%[4]q
exec %[5]q "$@"
`, countFile, yieldRuns, pidFile, home, bin)
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil { //nolint:gosec // it is meant to be executable
		t.Fatalf("write the wrapper: %v", err)
	}
	return wrapper, countFile, pidFile
}

// wrapperRuns is how many times the wrapper was invoked, read from the file it
// counts in.
func wrapperRuns(t *testing.T, countFile string) int {
	t.Helper()
	raw, err := os.ReadFile(countFile) // #nosec G304 — this test's own wrapper wrote it
	if err != nil {
		t.Fatalf("read the wrapper's run count: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("the wrapper's run count %q: %v", strings.TrimSpace(string(raw)), err)
	}
	return n
}

func TestEnsureWaitsWhenTheHelperItStartedYieldsToARetiringPeer(t *testing.T) {
	// The failure this bead is: the helper the caller started yielded to a peer
	// that was on its way out, and the caller was told "the helper did not
	// start" while the generation could be served.
	bin, generation := helperBinary(t)
	home := helperHome(t)
	dir := endpoint.Dir(home)

	// A real helper of this generation serves, and answers a real handshake —
	// this is the peer the yield names, and it is checked rather than assumed.
	peer := startPeer(t, bin, home, generation)
	reach(t, dir, generation)

	// And then it retires: its own shutdown, its own unlink.
	retirePeer(t, peer, dir, generation)

	wrapper, countFile, pidFile := yieldingWrapper(t, bin, home, 1)
	t.Cleanup(func() { stopRecorded(t, pidFile) })

	conn, err := endpoint.Ensure(t.Context(), dir, generation, generation, wrapper)
	if err != nil {
		t.Fatalf("Ensure gave up on a generation it can serve: %v", err)
	}

	// The connection it answers with is a helper of this generation, not a
	// socket that merely accepts: the handshake is the only thing that decides
	// (D21).
	reached, err := client.Dial(t.Context(), client.Config{
		Exec: client.NewSocketConn(conn), ExpectHash: string(generation),
	})
	if err != nil {
		t.Fatalf("the helper the second start raised did not answer as this generation: %v", err)
	}
	_ = reached.Close()

	// The endpoint is served again for the next caller too.
	reach(t, dir, generation)

	// And the start was made again exactly once: one run yielded, one run is
	// the real helper.
	if runs := wrapperRuns(t, countFile); runs != 2 {
		t.Fatalf("the wrapper ran %d times, want 2: one yield and one replacement", runs)
	}
}

func TestEnsureStopsAndNamesTheYieldingWhenEveryStartYields(t *testing.T) {
	// The other end of the same rule: a start that keeps yielding must END, and
	// must say what happened. A bare ErrNoEndpoint would be rendered as "the
	// helper did not start" — true of the binary and false of the cause — and a
	// wait with no bound would be a pane that never opens.
	bin, _ := helperBinary(t)
	home := helperHome(t)
	wrapper, countFile, _ := yieldingWrapper(t, bin, home, 1<<30)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, err := endpoint.Ensure(ctx, endpoint.Dir(home), gen, gen, wrapper)
	if err == nil {
		t.Fatal("Ensure returned a connection although every helper it started yielded")
	}
	// The refusal's classification is built on ErrNoEndpoint: a start that never
	// produced a serving helper IS a start failure, and the pane's closed-set
	// reason must keep saying so.
	if !errors.Is(err, endpoint.ErrNoEndpoint) {
		t.Fatalf("Ensure = %v, want it to wrap ErrNoEndpoint", err)
	}
	if !strings.Contains(err.Error(), "yielded") {
		t.Fatalf("the refusal does not name what happened: %v", err)
	}
	if runs := wrapperRuns(t, countFile); runs < 2 {
		t.Fatalf("Ensure concluded from the first yield: the wrapper ran %d time(s)", runs)
	}
}

// reach proves the endpoint is served by a helper of this generation: a dial
// that answers the handshake, which is the only thing that decides it.
func reach(t *testing.T, dir string, generation proto.GenerationID) {
	t.Helper()
	conn, err := endpoint.Dial(t.Context(), dir, generation)
	if err != nil {
		t.Fatalf("nothing is serving the endpoint: %v", err)
	}
	c, err := client.Dial(t.Context(), client.Config{
		Exec: client.NewSocketConn(conn), ExpectHash: string(generation),
	})
	if err != nil {
		t.Fatalf("the endpoint answered and is not this generation: %v", err)
	}
	_ = c.Close()
}
