package lifecycle

import (
	"testing"
)

// The shipped ordering nocx-1ebrs.39 measured on 0.6.1: the parent shell's
// accept-line widget sends domain_activated the moment the nested child's
// process returns, while the child's own teardown — the ssh child's
// forwarded channel EOF — is still in flight on ANOTHER transport. The
// protocol doc's own words order it this way: "The child owns the stream
// from its exec until the child process exits. The parent resumes at its
// next prompt boundary and sends domain_activated." The child's transport
// loss and the child process's exit are two facts on two channels, and no
// shell can wait for the one it cannot observe. The parent is the witness
// that cannot be wrong: its widget physically waited for the child process.
//
// Before the fix the kernel refused that activation (ErrDomainNotTop — a
// "live child" still sat on the stack), the shell never re-sends it, the
// suspended parent rejected every later frame, and the pane's ssh block
// stayed Running forever with the local prompt rendering inside it.
func TestActivationUnderALiveChildRevokesItAndCompletesTheParentBlock(t *testing.T) {
	k, _, _ := newTestKernel()
	p1, p2 := &fakePort{}, &fakePort{}
	_ = k.BindTransport("T1", p1) // the parent's own channel
	_ = k.BindTransport("T2", p2) // the ssh child's forwarded channel
	hA := establish(t, k, "T1", p1, "L", nil)

	// The user typed `ssh …` at an integrated local prompt: the parent's
	// own block opens before anything nested exists.
	mustIngest(t, k, "T1", env("L", hA, 2, promptReadyEvt()))
	mustIngest(t, k, "T1", env("L", hA, 3, startEvt(nil, "ssh far-host")))
	mustIngest(t, k, "T1", env("L", hA, 4, suspendEvt()))
	hB := establish(t, k, "T2", p2, "L", &hA.Domain)

	// A command on the far host completed normally on the child domain.
	mustIngest(t, k, "T2", env("L", hB, 2, promptReadyEvt()))
	far := AttemptID("s-far-1")
	mustIngest(t, k, "T2", env("L", hB, 3, startEvt(&far, "printf nocx-far")))
	mustIngest(t, k, "T2", env("L", hB, 4, completeEvt(far, 0, fence(1))))
	// And one is still open when the child's shell exits: `exit` destroys
	// the shell that would report it. Its block can never receive a
	// completion — it must end unknown, never run forever.
	mustIngest(t, k, "T2", env("L", hB, 5, promptReadyEvt()))
	exit := AttemptID("s-far-2")
	mustIngest(t, k, "T2", env("L", hB, 6, startEvt(&exit, "exit")))

	// The parent's widget boundary runs NOW — the child's forwarded channel
	// has not EOFed yet. The shipped script sends exactly this frame.
	mustIngest(t, k, "T1", env("L", hA, 5, activateEvt()))

	// The child ended, revoked exactly as its own goodbye would have:
	// removed from the stack, open attempts unknown.
	if dB, _ := k.Domain(hB.Domain); dB.State != DomainClosed {
		t.Fatalf("child revoked by the parent's activation = %v, want closed", dB.State)
	}
	if att, ok := k.Attempt(exit); !ok || att.State != AttemptUnknown {
		t.Fatalf("the child's open attempt after the revocation = %v (ok=%v), want unknown", att.State, ok)
	}
	if att, ok := k.Attempt(far); !ok || att.State != AttemptCompleted {
		t.Fatalf("the child's completed attempt disturbed = %v (ok=%v), want completed", att.State, ok)
	}

	// The parent owns the lane again, running its ssh attempt.
	st := mustState(t, k, "L")
	if st.Lifecycle != LifecycleRunning || st.Domain != hA.Domain {
		t.Fatalf("after the activation: %v domain=%s, want running on %s", st.Lifecycle, st.Domain, hA.Domain)
	}
	sshAtt := st.Attempt
	if sshAtt == "" {
		t.Fatal("no attempt named by the running lane")
	}

	// And the ssh block completes with the client's real exit status — the
	// local D of the ssh line, which the child's departure neither supplies
	// nor invalidates.
	mustIngest(t, k, "T1", env("L", hA, 6, completeEvt(sshAtt, 0, fence(2))))
	if a, _ := k.Attempt(sshAtt); a.State != AttemptCompleted || a.ExitCode == nil || *a.ExitCode != 0 {
		t.Fatalf("the ssh block = %v exit=%v, want completed 0", a.State, a.ExitCode)
	}
	mustIngest(t, k, "T1", env("L", hA, 7, promptReadyEvt()))
	if st := mustState(t, k, "L"); st.Lifecycle != LifecyclePromptReady {
		t.Fatalf("after the next prompt: %v, want prompt_ready", st.Lifecycle)
	}
}

// The child's forwarded channel dies AFTER the parent re-activated (that is
// the ordering the shipped shell produces). Its EOF must not flip a live
// parent's lane to lost: the pane keeps the prompt the parent's activation
// restored, and the already-revoked child reads as the transport loss it
// finally is.
func TestTransportLossAfterReactivationDoesNotStompTheLane(t *testing.T) {
	k, _, _ := newTestKernel()
	p1, p2 := &fakePort{}, &fakePort{}
	_ = k.BindTransport("T1", p1)
	_ = k.BindTransport("T2", p2)
	hA := establish(t, k, "T1", p1, "L", nil)
	mustIngest(t, k, "T1", env("L", hA, 2, promptReadyEvt()))
	mustIngest(t, k, "T1", env("L", hA, 3, startEvt(nil, "ssh far-host")))
	mustIngest(t, k, "T1", env("L", hA, 4, suspendEvt()))
	hB := establish(t, k, "T2", p2, "L", &hA.Domain)

	// The shipped ordering: the activation lands first, the child is
	// revoked, the parent's block completes, the shell is at its prompt.
	mustIngest(t, k, "T1", env("L", hA, 5, activateEvt()))
	st := mustState(t, k, "L")
	mustIngest(t, k, "T1", env("L", hA, 6, completeEvt(st.Attempt, 0, fence(1))))
	mustIngest(t, k, "T1", env("L", hA, 7, promptReadyEvt()))

	// NOW the forwarded channel's EOF arrives.
	if err := k.TransportLost("T2"); err != nil {
		t.Fatal(err)
	}
	if dB, _ := k.Domain(hB.Domain); dB.State != DomainLost {
		t.Fatalf("the child after its transport died = %v, want lost", dB.State)
	}
	if st := mustState(t, k, "L"); st.Lifecycle != LifecyclePromptReady {
		t.Fatalf("the lane after the child's EOF = %v, want the parent's prompt_ready kept", st.Lifecycle)
	}
	if dA, _ := k.Domain(hA.Domain); dA.State != DomainEstablished {
		t.Fatalf("the parent after the child's EOF = %v, want established", dA.State)
	}
}

// The old ordered path keeps working: a child that says goodbye first (the
// su child on the same descriptor) pops, and the parent's activation then
// finds itself the top.
func TestChildCloseThenActivationStillActivates(t *testing.T) {
	k, _, _ := newTestKernel()
	p := &fakePort{}
	_ = k.BindTransport("T1", p)
	hA := establish(t, k, "T1", p, "L", nil)
	mustIngest(t, k, "T1", env("L", hA, 2, suspendEvt()))
	hB := establish(t, k, "T1", p, "L", &hA.Domain)
	mustIngest(t, k, "T1", env("L", hB, 2, closeEvt()))
	mustIngest(t, k, "T1", env("L", hA, 3, activateEvt()))
	if st := mustState(t, k, "L"); st.Lifecycle != LifecyclePromptReady || st.Domain != hA.Domain {
		t.Fatalf("after the ordered close and activation: %v domain=%s, want prompt_ready on %s",
			st.Lifecycle, st.Domain, hA.Domain)
	}
}
