package session

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/shady2k/nocx/internal/emulator"
	"github.com/shady2k/nocx/internal/helper/proto"
)

type lifecycleFailureProcess struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	closed bool
}

func newLifecycleFailureProcess() *lifecycleFailureProcess {
	r, w := io.Pipe()
	return &lifecycleFailureProcess{reader: r, writer: w, done: make(chan struct{})}
}
func (p *lifecycleFailureProcess) Read(b []byte) (int, error)  { return p.reader.Read(b) }
func (p *lifecycleFailureProcess) Write(b []byte) (int, error) { return p.writer.Write(b) }
func (p *lifecycleFailureProcess) Close() error {
	p.once.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
		_ = p.reader.Close()
		_ = p.writer.Close()
		close(p.done)
	})
	return nil
}

func (p *lifecycleFailureProcess) Resize(context.Context, uint16, uint16, uint16, uint16) error {
	return nil
}
func (p *lifecycleFailureProcess) Done() <-chan struct{}                { return p.done }
func (p *lifecycleFailureProcess) WaitErr() (error, bool)               { return nil, false }
func (p *lifecycleFailureProcess) Pid() int                             { return 1 }
func (p *lifecycleFailureProcess) Shell() string                        { return "test" }
func (p *lifecycleFailureProcess) ForegroundProcessGroup() (int, error) { return 1, nil }
func (p *lifecycleFailureProcess) isClosed() bool                       { p.mu.Lock(); defer p.mu.Unlock(); return p.closed }

type lifecycleFailureSpawner struct {
	proc     *lifecycleFailureProcess
	calls    int
	sshCalls int
}

func (s *lifecycleFailureSpawner) Spawn(SpawnRequest) (Process, error) {
	s.calls++
	return s.proc, nil
}

func (s *lifecycleFailureSpawner) SpawnSSH(context.Context, SSHSpawnRequest) (Process, error) {
	s.sshCalls++
	return s.proc, nil
}

type noScrollbackTerminal struct {
	emulator.Terminal
	onClose func()
}

func (t noScrollbackTerminal) Close() {
	if t.onClose != nil {
		t.onClose()
	}
	t.Terminal.Close()
}

func TestSpawnIDGenerationFailureReleasesReservationAndKey(t *testing.T) {
	proc := newLifecycleFailureProcess()
	spawner := &lifecycleFailureSpawner{proc: proc}
	attempts := 0
	svc := New(Options{Spawner: spawner, NewID: func() ([16]byte, error) {
		attempts++
		if attempts == 1 {
			return [16]byte{}, errors.New("random source failed")
		}
		return [16]byte{1}, nil
	}})
	t.Cleanup(svc.Close)
	params := proto.SpawnParams{IdempotencyKey: "retry-me"}
	if _, err := svc.Call(context.Background(), proto.OpSpawn, mustJSONTest(t, params)); err == nil {
		t.Fatal("ID failure reported successful spawn")
	}
	if got := svc.WindowBytesInUse(); got != 0 {
		t.Fatalf("budget after ID failure = %d, want 0", got)
	}
	assertNoInventory(t, svc, "ID failure")
	if spawner.calls != 0 {
		t.Fatalf("spawner called %d times before an ID existed", spawner.calls)
	}
	if _, err := svc.Call(context.Background(), proto.OpSpawn, mustJSONTest(t, params)); err != nil {
		t.Fatalf("retry after ID failure: %v", err)
	}
	if spawner.calls != 1 {
		t.Fatalf("spawner calls after retry = %d, want 1", spawner.calls)
	}
}

func TestSpawnSSHIDGenerationFailureReleasesReservationAndKey(t *testing.T) {
	proc := newLifecycleFailureProcess()
	spawner := &lifecycleFailureSpawner{proc: proc}
	attempts := 0
	svc := New(Options{SSHSpawner: spawner, NewID: func() ([16]byte, error) {
		attempts++
		if attempts == 1 {
			return [16]byte{}, errors.New("random source failed")
		}
		return [16]byte{2}, nil
	}})
	t.Cleanup(svc.Close)
	params := proto.SSHSpawnParams{
		IdempotencyKey: "retry-ssh-id",
		Destination:    proto.SSHDestination{Host: "example.com", Port: 22, User: "test", Identity: proto.SSHIdentity{Auth: proto.SSHAuthInteractive}},
	}
	if _, err := svc.Call(context.Background(), proto.OpSpawnSSH, mustJSONTest(t, params)); err == nil {
		t.Fatal("SSH ID failure reported successful spawn")
	}
	if got := svc.WindowBytesInUse(); got != 0 {
		t.Fatalf("budget after SSH ID failure = %d, want 0", got)
	}
	if spawner.sshCalls != 0 {
		t.Fatalf("SSH spawner called %d times before an ID existed", spawner.sshCalls)
	}
	if _, err := svc.Call(context.Background(), proto.OpSpawnSSH, mustJSONTest(t, params)); err != nil {
		t.Fatalf("SSH retry after ID failure: %v", err)
	}
	if spawner.sshCalls != 1 {
		t.Fatalf("SSH spawner calls after retry = %d, want 1", spawner.sshCalls)
	}
}

func TestSpawnRuntimeSetupFailureClosesProcessAndReleasesState(t *testing.T) {
	proc := newLifecycleFailureProcess()
	spawner := &lifecycleFailureSpawner{proc: proc}
	screenErr := errors.New("emulator allocation failed")
	svc := New(Options{Spawner: spawner, Screen: func(emulator.Geometry) (emulator.Terminal, error) { return nil, screenErr }})
	t.Cleanup(svc.Close)
	params := proto.SpawnParams{IdempotencyKey: "retry-runtime"}
	if _, err := svc.Call(context.Background(), proto.OpSpawn, mustJSONTest(t, params)); err == nil {
		t.Fatal("runtime setup failure reported success")
	}
	if !proc.isClosed() {
		t.Fatal("runtime setup failure left the spawned process open")
	}
	if got := svc.WindowBytesInUse(); got != 0 {
		t.Fatalf("budget after runtime setup failure = %d, want 0", got)
	}
	assertNoInventory(t, svc, "runtime setup failure")
	svc.screen = defaultScreen
	spawner.proc = newLifecycleFailureProcess()
	if _, err := svc.Call(context.Background(), proto.OpSpawn, mustJSONTest(t, params)); err != nil {
		t.Fatalf("retry after runtime setup failure: %v", err)
	}
}

func TestSpawnScrollbackSetupFailureClosesScreenProcessAndReleasesState(t *testing.T) {
	proc := newLifecycleFailureProcess()
	spawner := &lifecycleFailureSpawner{proc: proc}
	screenClosed := false
	screenCloseCalls := 0
	screenClosedBeforeProcess := false
	screenAttempts := 0
	svc := New(Options{Spawner: spawner, Screen: func(g emulator.Geometry) (emulator.Terminal, error) {
		screenAttempts++
		if screenAttempts > 1 {
			return defaultScreen(g)
		}
		terminal, err := defaultScreen(g)
		if err != nil {
			return nil, err
		}
		// Embedding Terminal intentionally omits the optional ScrollbackBudget
		// capability, making runtime setup refuse this otherwise valid screen.
		return noScrollbackTerminal{Terminal: terminal, onClose: func() {
			screenCloseCalls++
			screenClosed = true
			screenClosedBeforeProcess = !proc.isClosed()
		}}, nil
	}})
	t.Cleanup(svc.Close)
	params := proto.SpawnParams{IdempotencyKey: "retry-scrollback"}
	if _, err := svc.Call(context.Background(), proto.OpSpawn, mustJSONTest(t, params)); err == nil {
		t.Fatal("unsupported scrollback setup reported success")
	}
	if !proc.isClosed() {
		t.Fatal("scrollback setup failure left the spawned process open")
	}
	if !screenClosed {
		t.Fatal("scrollback setup failure left the created screen open")
	}
	if screenCloseCalls != 1 {
		t.Fatalf("created screen was closed %d times, want exactly once", screenCloseCalls)
	}
	if !screenClosedBeforeProcess {
		t.Fatal("created screen was not closed before its process")
	}
	if got := svc.WindowBytesInUse(); got != 0 {
		t.Fatalf("budget after scrollback setup failure = %d, want 0", got)
	}
	assertNoInventory(t, svc, "scrollback setup failure")
	spawner.proc = newLifecycleFailureProcess()
	if _, err := svc.Call(context.Background(), proto.OpSpawn, mustJSONTest(t, params)); err != nil {
		t.Fatalf("retry after scrollback setup failure: %v", err)
	}
}

// Keep test requests on the same JSON boundary as the production service.
func mustJSONTest(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

var _ Process = (*lifecycleFailureProcess)(nil)

func assertNoInventory(t *testing.T, svc *Service, failure string) {
	t.Helper()
	value, err := svc.Call(context.Background(), proto.OpSessions, mustJSONTest(t, proto.SessionsParams{}))
	if err != nil {
		t.Fatal(err)
	}
	inventory, ok := value.(proto.SessionsResult)
	if !ok {
		t.Fatalf("sessions result has type %T, want proto.SessionsResult", value)
	}
	if len(inventory.Sessions) != 0 {
		t.Fatalf("%s left inventory: %+v", failure, inventory)
	}
}
