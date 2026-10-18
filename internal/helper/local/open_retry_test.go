package local_test

// A PANE IS NOT REFUSED BECAUSE THE ENDPOINT ANSWERED AND THEN LEFT
// (nocx-xn63t.6.41).
//
// CI run 39149340188, ci-e2e (webkit, 1), trace 6b448fc00cda04c2b5f73fc2d314886d:
// the first pane open of a test was refused with
//
//	Nocx started this machine's helper and it did not answer as the build that
//	installed it: helper: connection lost: EOF. Open the pane again.
//
// and the open never issued a single helper op — no spawn, no attach, nothing
// (the shared stand's own log shows the dispatch and then silence until the
// spec's 15 s window expired). So the failure is inside reaching the endpoint:
// the dial WAS answered, and the peer closed the connection before it wrote
// the sentinel. That is one event seen from the caller, and the endpoint's
// owner is the only process that can produce it — a daemon whose shutdown
// closes the connections it accepted (endpoint.Serve: `ln.Close()`, then every
// connection it is tracking), or one whose host ended a connection it would not
// answer.
//
// WHAT THE FIX IS, and where it lives: reaching this machine's helper is
// `local.Open`'s own job — the dial, the start when nothing is serving, and the
// handshake — so the answer to "the peer went away before it answered" belongs
// at that join and nowhere above it. A handshake that ended because the peer
// left is not a verdict about the generation or about the install: the reach is
// made AGAIN, which is where a helper is started if nothing now serves, and the
// socket's own creation event is what that waits on (endpoint.Ensure). The
// refusal a person is shown is untouched for every case where retrying is
// genuinely wrong — a peer answering with another build's content hash, another
// program entirely, or a version this one refuses — and the last attempt's
// error is what leaves Open, so the classifier that renders it
// (internal/app/helper_refusal / transport) keeps its closed set and its
// sentence.
//
// The three tests below pin the three halves that matter:
//
//   - a peer that ACCEPTED and then went away costs one reach, not a pane
//     (TestAnOpenWhosePeerWentAwayMidHandshakeReachesTheEndpointAgain);
//   - an endpoint that keeps going away ends in the loss it produced, after a
//     BOUNDED number of reaches — never a loop, never a hang
//     (TestAnEndpointThatKeepsGoingAwayEndsInTheLossAfterABoundedNumberOfReaches);
//   - a definite refusal is still not retried
//     (TestADefiniteRefusalIsNotRetriedIntoAStart).
//
// WHAT IS MODELLED RATHER THAN RE-DERIVED: the reason the peer goes away. The
// fixture below IS endpoint.Serve — the code a retiring daemon runs — and the
// test triggers its shutdown itself instead of waiting for the daemon to drain
// its last session, because the drain is a session's event and this test is
// about what the caller sees when it happens. What is real is the accept, the
// shutdown's own `ln.Close()`-then-close-the-connections order, and the client's
// handshake over the socket.

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/shady2k/nocx/internal/helper/client"
	"github.com/shady2k/nocx/internal/helper/endpoint"
	"github.com/shady2k/nocx/internal/helper/host"
	"github.com/shady2k/nocx/internal/helper/local"
	"github.com/shady2k/nocx/internal/helper/proto"
)

// holdingPeer is an endpoint that ACCEPTS a connection, says so, and then goes
// away — the daemon-side event the caller met as "connection lost: EOF".
//
// It is endpoint.Serve, the same loop cmd/nocx-helper's serve runs, with a
// handler standing in for the host: the handler takes the connection and holds
// it, which is what a host does while it reads the hello. Nothing here writes
// to the connection, so the only thing that can end the caller's handshake is
// the peer closing it.
type holdingPeer struct {
	// accepted closes when the first connection is in hand: from that moment
	// the caller's handshake is on a connection this endpoint is tracking and
	// has written its hello into.
	accepted chan struct{}
	// release lets the handler return. A shutdown closes the connection
	// without waiting for its handler; this is only so the test can leave no
	// goroutine behind.
	release chan struct{}
	// served closes when Serve has returned: the endpoint is gone.
	served chan struct{}
	// cancel is the peer going away — what a daemon's drain does to its own
	// serve loop (stopDaemon), and what endpoint.Serve watches.
	cancel    context.CancelFunc
	mu        sync.Mutex
	acceptedN int
}

// startHoldingPeer binds the endpoint for gen and serves it until the returned
// peer is taken away. It fails rather than skips if something already serves
// the path: the fixture is the only thing that may.
func startHoldingPeer(t *testing.T, dir string, gen proto.GenerationID) *holdingPeer {
	t.Helper()
	ln, err := endpoint.Listen(dir, gen)
	if err != nil {
		t.Fatalf("listen on the endpoint: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &holdingPeer{
		accepted: make(chan struct{}),
		release:  make(chan struct{}),
		served:   make(chan struct{}),
		cancel:   cancel,
	}
	var once sync.Once
	go func() {
		defer close(p.served)
		_ = endpoint.Serve(ctx, ln, func(conn net.Conn) {
			p.mu.Lock()
			p.acceptedN++
			p.mu.Unlock()
			once.Do(func() { close(p.accepted) })
			<-p.release
		})
	}()
	t.Cleanup(func() {
		cancel()
		close(p.release)
		<-p.served
	})
	return p
}

// goAway is the peer leaving: the shutdown a drain triggers. It is
// asynchronous, and the test does not wait for it — what it waits on is the
// caller's own answer, and endpoint.Serve closes the LISTENER before it closes
// the connections it accepted, so the socket is already gone by the time the
// caller can have seen anything.
func (p *holdingPeer) goAway() { p.cancel() }

// acceptedConnections is how many connections this endpoint took.
func (p *holdingPeer) acceptedConnections() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.acceptedN
}

// hangingUpPeer is an endpoint that KEEPS listening and closes every
// connection it accepts, after reading what the caller wrote. It is the
// pathological end of the same event: something is serving the path and will
// never answer the handshake, so a caller that retries must stop.
//
// The handler reads first, so the caller's hello write SUCCEEDS and the only
// failure left is the peer's close — which is what makes the assertion about
// the caller's error deterministic rather than a race between two observers.
type hangingUpPeer struct {
	served chan struct{}
	cancel context.CancelFunc

	mu        sync.Mutex
	acceptedN int
}

func startHangingUpPeer(t *testing.T, dir string, gen proto.GenerationID) *hangingUpPeer {
	t.Helper()
	ln, err := endpoint.Listen(dir, gen)
	if err != nil {
		t.Fatalf("listen on the endpoint: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &hangingUpPeer{served: make(chan struct{}), cancel: cancel}
	go func() {
		defer close(p.served)
		_ = endpoint.Serve(ctx, ln, func(conn net.Conn) {
			p.mu.Lock()
			p.acceptedN++
			p.mu.Unlock()
			_, _ = conn.Read(make([]byte, 1024))
			_ = conn.Close()
		})
	}()
	t.Cleanup(func() {
		cancel()
		<-p.served
	})
	return p
}

func (p *hangingUpPeer) acceptedConnections() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.acceptedN
}

// peerLeavesBound is how many times one Open may reach for the endpoint
// (internal/helper/local's own openAttempts). A test in another package cannot
// read that constant, and naming it here means a change to it fails this
// assertion loudly instead of making the bound untested.
const peerLeavesBound = 3

// openResult is one Open's answer, carried off its goroutine.
type openResult struct {
	c   *client.Client
	err error
}

// installedLocalHelper puts the real helper in a home of its own and answers
// the two facts an open of that generation needs: the generation, and the
// wrapper `local.Open` may start — a helper is detached from whoever started it
// (D1), so a test that could not stop one again would leave a daemon on this
// machine.
func installedLocalHelper(t *testing.T, home string) (local.Installed, string) {
	t.Helper()
	installed, err := local.Install(context.Background(), helperSource(t), home)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	return installed, killableStarter(t, home, installed.Binary)
}

func TestAnOpenWhosePeerWentAwayMidHandshakeReachesTheEndpointAgain(t *testing.T) {
	// The CI failure, at the boundary that produced it: the endpoint answered
	// and its owner went away under our handshake. Before the fix this Open
	// returns `helper: connection lost: EOF` and the pane is refused; after it,
	// the reach is made again and the generation is served.
	home := helperHome(t)
	installed, starter := installedLocalHelper(t, home)
	dir := endpoint.Dir(home)

	peer := startHoldingPeer(t, dir, installed.Generation)

	answer := make(chan openResult, 1)
	go func() {
		c, err := local.Open(context.Background(), local.Config{
			Dir:        dir,
			Generation: installed.Generation,
			Binary:     starter,
			Log:        discardLog(),
		})
		answer <- openResult{c: c, err: err}
	}()

	// Ordering, not timing: the handshake is in flight on a connection this
	// endpoint accepted, and only then does the peer go away.
	<-peer.accepted
	peer.goAway()

	got := <-answer
	if got.err != nil {
		t.Fatalf("a peer that went away mid-handshake refused the pane: %v", got.err)
	}
	defer func() { _ = got.c.Close() }()

	// The connection it answered with is a helper of THIS generation, which is
	// the only thing that decides it (D21): the fixture never writes a
	// sentinel, so an instance id can only have come from the real binary the
	// retry started.
	if got.c.InstanceID() == "" {
		t.Fatal("the client carries no instance id: no helper answered the hello-ok")
	}
	var inv proto.SessionsResult
	if err := got.c.Call(t.Context(), proto.ServiceSession, proto.OpSessions, proto.SessionsParams{}, &inv); err != nil {
		t.Fatalf("the session service is not reachable through the carrier: %v", err)
	}

	// And the endpoint is served by something that is NOT the peer that left:
	// the peer took exactly the one connection it went away on, and the socket
	// it held is served by a helper the retry started (the wrapper records the
	// pid of every helper it execs).
	if n := peer.acceptedConnections(); n != 1 {
		t.Fatalf("the peer that went away took %d connections, want the 1 it left on", n)
	}
	if _, statErr := os.Lstat(filepath.Join(home, "helper.pid")); statErr != nil {
		t.Fatalf("the retry reached nothing that started: %v", statErr)
	}
}

func TestAnEndpointThatKeepsGoingAwayEndsInTheLossAfterABoundedNumberOfReaches(t *testing.T) {
	// The other end of the rule, and the half the criterion names as "not a
	// retry loop that hides it": an endpoint that answers and leaves, every
	// time, must END, must say what happened, and must not hang. The refusal a
	// person reads is the one this produced — the loss — because that is what
	// reaching this machine's helper actually failed with.
	home := helperHome(t)
	installed, starter := installedLocalHelper(t, home)
	dir := endpoint.Dir(home)
	peer := startHangingUpPeer(t, dir, installed.Generation)

	_, err := local.Open(t.Context(), local.Config{
		Dir:        dir,
		Generation: installed.Generation,
		Binary:     starter,
		Log:        discardLog(),
	})
	if err == nil {
		t.Fatal("Open returned a client for an endpoint that never answered")
	}
	if !errors.Is(err, client.ErrLost) {
		t.Fatalf("Open = %v, want the transport loss the peer produced", err)
	}

	// Bounded, and not by a duration: the endpoint counts how many times it was
	// reached. One is the first attempt; every further one is a retry, and
	// there is a last one.
	if n := peer.acceptedConnections(); n != peerLeavesBound {
		t.Fatalf("the endpoint was reached %d times, want %d: a retry loop with no bound, or none at all",
			n, peerLeavesBound)
	}

	// AND NOTHING WAS STARTED. Something is serving the socket, so the reach
	// needs no helper; a retry that spawned one anyway would be a process per
	// gesture on a machine whose endpoint is merely confused. The wrapper
	// records the pid of every helper it execs, and there is none.
	if _, statErr := os.Lstat(filepath.Join(home, "helper.pid")); statErr == nil {
		t.Fatal("a retry started a helper although the endpoint was still answering")
	}
}

func TestADefiniteRefusalIsNotRetriedIntoAStart(t *testing.T) {
	// What must NOT become a retry: something is serving this generation's
	// socket and it is not the build this one installed. That is a fact about
	// the install, not about a race (ADR-0057), and the remedy is the person's
	// — quitting the other copy of nocx — so it is refused on the first answer.
	home := helperHome(t)
	installed, starter := installedLocalHelper(t, home)
	dir := endpoint.Dir(home)
	other := hashOf([]byte("some other build entirely"))

	answered := startAnsweringPeer(t, dir, installed.Generation, other)

	_, err := local.Open(t.Context(), local.Config{
		Dir:        dir,
		Generation: installed.Generation,
		Binary:     starter,
		Log:        discardLog(),
	})
	if !errors.Is(err, client.ErrHashMismatch) {
		t.Fatalf("Open = %v, want ErrHashMismatch", err)
	}
	if n := answered.acceptedConnections(); n != 1 {
		t.Fatalf("the mismatched peer was asked %d times, want exactly 1: a build mismatch is not a race", n)
	}
	if _, statErr := os.Lstat(filepath.Join(home, "helper.pid")); statErr == nil {
		t.Fatal("a build mismatch was retried by starting this build's helper")
	}
}

// answeringPeer is the real host answering a hello-ok with somebody else's
// content hash — the fixture the mismatched-binary assertion needs
// (respondAs's shape, with a count).
type answeringPeer struct {
	served chan struct{}

	mu        sync.Mutex
	acceptedN int
}

func startAnsweringPeer(t *testing.T, dir string, gen proto.GenerationID, contentHash string) *answeringPeer {
	t.Helper()
	ln, err := endpoint.Listen(dir, gen)
	if err != nil {
		t.Fatalf("listen on the endpoint: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &answeringPeer{served: make(chan struct{})}
	go func() {
		defer close(p.served)
		_ = endpoint.Serve(ctx, ln, func(conn net.Conn) {
			p.mu.Lock()
			p.acceptedN++
			p.mu.Unlock()
			h := host.New(conn, conn, contentHash, "instance-under-test", discardLog())
			_ = h.Serve(ctx)
		})
	}()
	t.Cleanup(func() {
		cancel()
		<-p.served
	})
	return p
}

func (p *answeringPeer) acceptedConnections() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.acceptedN
}
