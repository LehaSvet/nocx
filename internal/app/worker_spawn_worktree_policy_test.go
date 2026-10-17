package app

// The nocx-xn63t.6.31 policy assertion: callerGrant is the grant every
// admitted external coordinator is minted with. Delegation and worktree
// creation are distinct effects (ADR-0053); the grant permits both for the
// coordinator's own session/environment so its worktree ask can reach the
// app's worktree planner.

import (
	"context"
	"os"
	"testing"

	"github.com/shady2k/nocx/internal/agenttools"
	"github.com/shady2k/nocx/internal/assistant"
	"github.com/shady2k/nocx/internal/content"
	"github.com/shady2k/nocx/internal/session"
	"github.com/shady2k/nocx/internal/workers"
)

// spawnPolicyRecord answers the worker seam just well enough for one spawn to
// travel the whole dispatch path: Register records what reached it — which is
// the observable the before-side-effects claim is judged on — and everything
// else answers zero, because this test decides policy, not record behaviour.
type spawnPolicyRecord struct {
	registered []workers.RegisterRequest
}

func (r *spawnPolicyRecord) Register(_ context.Context, req workers.RegisterRequest) (workers.Registration, error) {
	r.registered = append(r.registered, req)
	return workers.Registration{Participant: workers.Participant{ID: "p-1", State: workers.StateLive}}, nil
}

func (*spawnPolicyRecord) HeldBy(context.Context, string) ([]workers.Participant, error) {
	return nil, nil
}

func (*spawnPolicyRecord) Say(context.Context, workers.ID, workers.ReaderID, workers.ReaderID, string) (workers.Message, error) {
	return workers.Message{}, nil
}

func (*spawnPolicyRecord) Report(context.Context, workers.ParticipantID, workers.Report) (workers.Message, error) {
	return workers.Message{}, nil
}

func (*spawnPolicyRecord) Inbox(context.Context, workers.ReaderID, workers.ReaderID, int) (workers.Fetch, error) {
	return workers.Fetch{}, nil
}

func (*spawnPolicyRecord) Undelivered(context.Context, workers.ID) ([]workers.Message, error) {
	return nil, nil
}

func (*spawnPolicyRecord) Acknowledge(context.Context, workers.ReaderID, workers.ReaderID, int64) error {
	return nil
}

func (*spawnPolicyRecord) Close(context.Context, string, workers.ParticipantID) (workers.CloseResult, error) {
	return workers.CloseResult{}, nil
}
func (*spawnPolicyRecord) Undispatched() []workers.Fact { return nil }
func (*spawnPolicyRecord) LeftoverCheckouts(context.Context, string) workers.CheckoutSurvey {
	return workers.CheckoutSurvey{Complete: true}
}

func (*spawnPolicyRecord) RemoveCheckouts(context.Context, string, []workers.CheckoutRef) workers.CheckoutRemoval {
	return workers.CheckoutRemoval{}
}

func dispatchSpawnWithGrant(t *testing.T, rec *spawnPolicyRecord, grant content.Grant, params string) error {
	t.Helper()
	registry, err := agenttools.Assemble(os.DirFS("../../contracts/tools"))
	if err != nil {
		t.Fatalf("assemble tools: %v", err)
	}
	dispatcher, err := assistant.NewToolDispatcher(registry, rec, "env-local")
	if err != nil {
		t.Fatalf("new tool dispatcher: %v", err)
	}
	_, err = dispatcher.Dispatch(assistant.ToolInvocation{
		Context:    context.Background(),
		Method:     "workers.spawn",
		RunContext: agenttools.RunContext{Session: "sess-coordinator"},
		Grant:      grant,
		RawParams:  []byte(params),
	})
	return err
}

func TestSpawnUnderTheProductMintAllowsWorktreeAndPlainSpawn(t *testing.T) {
	grant := callerGrant(session.ID("sess-coordinator"), "env-local", workerTestWorkspace)
	for _, params := range []string{`{"command":"claude","task":"read it"}`, `{"command":"claude","task":"read it","worktree":{"branch":"feat/x"}}`} {
		rec := &spawnPolicyRecord{}
		if err := dispatchSpawnWithGrant(t, rec, grant, params); err != nil {
			t.Fatalf("spawn under product grant (%s): %v", params, err)
		}
		if len(rec.registered) != 1 {
			t.Fatalf("spawn registered %d workers, want 1", len(rec.registered))
		}
	}
}
