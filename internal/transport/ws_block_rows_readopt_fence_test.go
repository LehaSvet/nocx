package transport

// nocx-zg3k3.35: a coordinator replaced between a command's completion and
// its interval end. The completion's frame is applied and stored — the
// lifecycle cursor the replacing coordinator resumes from is past it, so the
// completion is never offered again — but the fence it published lived only
// in the gone process's memory. The helper resends the unconfirmed end; the
// replacing coordinator must still be able to match it, or the re-bound block
// stays current forever and every later command's block queues behind it.

import (
	"context"
	"errors"
	"testing"

	"github.com/shady2k/nocx/internal/content"
	"github.com/shady2k/nocx/internal/emulator"
	"github.com/shady2k/nocx/internal/lifecycle"
	"github.com/shady2k/nocx/internal/session"
)

func TestAnEndWhoseCompletionTheGoneCoordinatorStoredSealsTheReboundBlock(t *testing.T) {
	e, pub, lane, h, sidStr, db := newLifecycleLedgerEnv(t, true)
	sid := session.ID(sidStr)
	createSessionRow(t, db, sidStr)
	e.ws.AttachBlockRows(sid)

	first := startsACommand(t, e, pub, lane, h, 2, "answer the question")
	if _, confirm := e.ws.BlockRowsArrived(sid, 0, 0, []emulator.Row{aStreamRow("asked")}, ""); !confirm {
		t.Fatal("the streamed row was not confirmed")
	}
	fence := lifecycleFence(0x5a)
	mustLifecycleIngest(t, pub, "T", lifecycleEnv(lane, h, 3, lifecycleCompleteEvt(lifecycle.AttemptID(first), 0, fence)))
	mustLifecycleIngest(t, pub, "T", lifecycleEnv(lane, h, 4, lifecyclePromptEvt()))

	// The coordinator is replaced before the end marker reached it: the
	// completion is stored, its in-memory fence is gone with the process.
	e.ws.DetachBlockRows(sid)
	e.ws.AttachBlockRows(sid)

	// The helper resends the end the gone coordinator never confirmed.
	e.ws.BlockIntervalEnded(sid, fence, 1, []emulator.Row{aStreamRow("$ ")}, false)

	if art := blockArtifact(t, db, first); art.State != content.ArtifactSealed {
		t.Fatalf("the re-bound block is %q after its resent end, want sealed", art.State)
	}

	// And what a person sees: the next command's block is not queued behind
	// a block that can never close — its own end seals it.
	second := startsACommand(t, e, pub, lane, h, 5, "echo AFTER")
	if _, confirm := e.ws.BlockRowsArrived(sid, 1, 0, []emulator.Row{aStreamRow("AFTER")}, ""); !confirm {
		t.Fatal("the next command's row was not confirmed")
	}
	next := lifecycleFence(0x5b)
	mustLifecycleIngest(t, pub, "T", lifecycleEnv(lane, h, 6, lifecycleCompleteEvt(lifecycle.AttemptID(second), 0, next)))
	e.ws.BlockIntervalEnded(sid, next, 2, []emulator.Row{aStreamRow("$ ")}, false)
	if art := blockArtifact(t, db, second); art.State != content.ArtifactSealed {
		t.Fatalf("the command after the replacement is %q after its end, want sealed", art.State)
	}
}

// fenceRefusingStore is the real ledger with the fence write refused.
type fenceRefusingStore struct {
	closeFailureBlockStore
}

func (s *fenceRefusingStore) RecordBlockRowsFence(context.Context, content.RecordBlockRowsFence) error {
	return errors.New("fence write refused")
}

// The paired failure half: a store that refuses the fence costs only the
// replacement case. The coordinator that saw the completion still holds the
// fence in memory, and the end it is owed seals the block as before.
func TestARefusedFenceWriteStillSealsTheBlockInTheSameCoordinator(t *testing.T) {
	e, pub, lane, h, sidStr, db := newLifecycleLedgerEnv(t, true)
	sid := session.ID(sidStr)
	createSessionRow(t, db, sidStr)
	e.ws.blockRowsStore = &fenceRefusingStore{closeFailureBlockStore{ledger: db.Ledger()}}
	e.ws.AttachBlockRows(sid)

	attempt := startsACommand(t, e, pub, lane, h, 2, "answer the question")
	if _, confirm := e.ws.BlockRowsArrived(sid, 0, 0, []emulator.Row{aStreamRow("asked")}, ""); !confirm {
		t.Fatal("the streamed row was not confirmed")
	}
	fence := lifecycleFence(0x5c)
	mustLifecycleIngest(t, pub, "T", lifecycleEnv(lane, h, 3, lifecycleCompleteEvt(lifecycle.AttemptID(attempt), 0, fence)))
	e.ws.BlockIntervalEnded(sid, fence, 1, []emulator.Row{aStreamRow("$ ")}, false)

	if art := blockArtifact(t, db, attempt); art.State != content.ArtifactSealed {
		t.Fatalf("the block is %q after its end with the fence write refused, want sealed", art.State)
	}
}
