package content_test

// nocx-zg3k3.35: a stored completion's fence is kept on the open block it
// completes until the end marker seals it, so a coordinator replaced in
// between can still resolve the end the helper resends.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shady2k/nocx/internal/content"
	"github.com/shady2k/nocx/internal/emulator"
)

func blockPayloadFence(t *testing.T, led content.LedgerRepository, artifact string) string {
	t.Helper()
	art, err := led.Artifact(context.Background(), artifact)
	if err != nil || art == nil {
		t.Fatalf("read artifact %s: %v (%v)", artifact, err, art)
	}
	var payload struct {
		Fence string `json:"fence"`
	}
	if err := json.Unmarshal([]byte(art.Payload), &payload); err != nil {
		t.Fatalf("decode payload: %v (raw %s)", err, art.Payload)
	}
	return payload.Fence
}

func TestAStoredFenceSurvivesLaterAppendsAndGoesWithTheSeal(t *testing.T) {
	ctx := context.Background()
	_, led := newLedger(t)
	entryID := recordOne(t, led, "answer the question")
	artifact := "00000000-0000-7000-8000-0000000000fe"
	if _, err := led.OpenBlockOutput(ctx, content.OpenBlockOutput{EntryID: entryID, ArtifactID: artifact}); err != nil {
		t.Fatalf("OpenBlockOutput: %v", err)
	}
	if err := led.AppendBlockRows(ctx, content.AppendBlockRows{
		EntryID: entryID, ArtifactID: artifact, FromRow: 0, Rows: []emulator.Row{aTextRow("asked")},
	}); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	const fence = "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a"
	if err := led.RecordBlockRowsFence(ctx, content.RecordBlockRowsFence{
		EntryID: entryID, ArtifactID: artifact, Fence: fence,
	}); err != nil {
		t.Fatalf("RecordBlockRowsFence: %v", err)
	}
	// The tail the end marker is still owed lands after the completion.
	if err := led.AppendBlockRows(ctx, content.AppendBlockRows{
		EntryID: entryID, ArtifactID: artifact, FromRow: 1, Rows: []emulator.Row{aTextRow("answered")},
	}); err != nil {
		t.Fatalf("tail delivery: %v", err)
	}
	if got := blockPayloadFence(t, led, artifact); got != fence {
		t.Fatalf("fence after a later append = %q, want %q kept", got, fence)
	}

	if _, err := led.CloseBlockRows(ctx, content.CloseBlockRows{EntryID: entryID, ArtifactID: artifact}); err != nil {
		t.Fatalf("CloseBlockRows: %v", err)
	}
	if got := blockPayloadFence(t, led, artifact); got != "" {
		t.Fatalf("fence after the seal = %q, want none: nothing is owed", got)
	}
	// A sealed block's end already came: recording again changes nothing.
	if err := led.RecordBlockRowsFence(ctx, content.RecordBlockRowsFence{
		EntryID: entryID, ArtifactID: artifact, Fence: fence,
	}); err != nil {
		t.Fatalf("RecordBlockRowsFence on a sealed block: %v, want the idempotent nil", err)
	}
	if got := blockPayloadFence(t, led, artifact); got != "" {
		t.Fatalf("fence on the sealed block = %q, want it left as sealed", got)
	}
}

func TestAFenceIsRefusedForABlockItDoesNotName(t *testing.T) {
	ctx := context.Background()
	_, led := newLedger(t)
	entryID := recordOne(t, led, "one")
	other := recordOne(t, led, "two")
	artifact := "00000000-0000-7000-8000-0000000000fd"
	if _, err := led.OpenBlockOutput(ctx, content.OpenBlockOutput{EntryID: entryID, ArtifactID: artifact}); err != nil {
		t.Fatalf("OpenBlockOutput: %v", err)
	}
	const fence = "5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b"
	if err := led.RecordBlockRowsFence(ctx, content.RecordBlockRowsFence{
		EntryID: other, ArtifactID: artifact, Fence: fence,
	}); !errors.Is(err, content.ErrIDConflict) {
		t.Fatalf("fence for another entry's block: %v, want ErrIDConflict", err)
	}
	if err := led.RecordBlockRowsFence(ctx, content.RecordBlockRowsFence{
		EntryID: entryID, ArtifactID: "00000000-0000-7000-8000-0000000000fc", Fence: fence,
	}); !errors.Is(err, content.ErrBlockNotOpen) {
		t.Fatalf("fence for no block: %v, want ErrBlockNotOpen", err)
	}
	if got := blockPayloadFence(t, led, artifact); got != "" {
		t.Fatalf("a refused fence was written: %q", got)
	}
}
