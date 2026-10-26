package capability_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shady2k/nocx/internal/capability"
	"github.com/shady2k/nocx/internal/session"
	"github.com/shady2k/nocx/internal/transport/control"
)

type heldSessionEndRegistry struct {
	session.Registry
	entered chan struct{}
	release chan struct{}
	failure error
}

func (r *heldSessionEndRegistry) PrepareEndSession(id session.ID) (session.Session, func() error, error) {
	sess, finish, err := r.Registry.PrepareEndSession(id)
	if err != nil {
		return nil, nil, err
	}
	return sess, func() error {
		close(r.entered)
		<-r.release
		return errors.Join(finish(), r.failure)
	}, nil
}

func TestSessionEndRetiresBeforeTeardownAndKeepsItsLaneUntilCompletion(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "teardown_failure"
		}
		t.Run(name, func(t *testing.T) {
			registry := newFakeSessionRegistry()
			registry.sessions["A"] = &fakeSession{id: "A"}
			registry.sessions["B"] = &fakeSession{id: "B"}
			reg := &heldSessionEndRegistry{Registry: registry, entered: make(chan struct{}), release: make(chan struct{})}
			if fail {
				reg.failure = errors.New("helper close failed")
			}
			sessionGate := control.NewSemaphore(capability.GateSession, 1)
			lane := control.NewSemaphore("control", 1)
			ops := capability.NewSessionOperations(sessionGate, lane, reg, nil)
			op, err := ops.ForSessionEnd("A")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type outcome struct {
				sess session.Session
				err  error
			}
			result := make(chan outcome, 1)
			go func() {
				sess, err := op.Run(ctx, func(retired session.Session) {
					if retired.ID() != "A" {
						t.Errorf("retired %q, want A", retired.ID())
					}
					if _, err := reg.Get("A"); err == nil {
						t.Error("before-teardown callback could still look up the retired session")
					}
				})
				result <- outcome{sess, err}
			}()
			<-reg.entered
			// Cancellation after retirement cannot skip process completion.
			cancel()
			permit, rej := sessionGate.TryAcquire(context.Background())
			if rej != nil {
				t.Error("slow teardown still holds the whole-session gate")
			} else {
				if _, err := reg.Get("B"); err != nil {
					t.Errorf("unrelated session disappeared: %v", err)
				}
				permit.Release()
			}
			if permit, rej := lane.TryAcquire(context.Background()); rej == nil {
				permit.Release()
				t.Error("teardown released its execution allowance before completion")
			}
			close(reg.release)
			got := <-result
			if got.sess == nil || got.sess.ID() != "A" || !errors.Is(got.err, reg.failure) {
				t.Fatalf("completion = (%v, %v), want retired A and %v", got.sess, got.err, reg.failure)
			}
			if permit, rej := lane.TryAcquire(context.Background()); rej != nil {
				t.Fatal("finished teardown leaked its execution allowance")
			} else {
				permit.Release()
			}
		})
	}
}

func TestSessionEndAdmissionRefusalLeavesTheSessionLive(t *testing.T) {
	reg := newFakeSessionRegistry()
	reg.sessions["A"] = &fakeSession{id: "A"}
	ops := capability.NewSessionOperations(control.NewSemaphore(capability.GateSession, 1), control.NewSemaphore("control", 0), reg, nil)
	op, err := ops.ForSessionEnd("A")
	if err != nil {
		t.Fatal(err)
	}
	got, err := op.Run(context.Background(), func(session.Session) { t.Error("refused close reached teardown preparation") })
	if got != nil || !capability.IsRefused(err) {
		t.Fatalf("saturated close = (%v, %v), want refusal before retirement", got, err)
	}
	if _, err := reg.Get("A"); err != nil {
		t.Fatalf("refused close removed the session: %v", err)
	}
}
