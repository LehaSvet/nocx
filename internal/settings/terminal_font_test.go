package settings_test

import (
	"errors"
	"testing"

	"github.com/shady2k/nocx/internal/settings"
)

func TestTerminalFont_SelectionSurvivesReloadAndRejectsUnknownFaces(t *testing.T) {
	doc := &fakeDoc{}
	reg := settings.New(doc, &fakeSecretStore{})
	if err := reg.ApplyValues(map[string]any{"terminal.fontFamily": "nordwand-mono"}); err != nil {
		t.Fatal(err)
	}
	reloaded := settings.New(doc, &fakeSecretStore{})
	snapshot, err := reloaded.GetSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.Values["terminal.fontFamily"]; got != "nordwand-mono" {
		t.Fatalf("font after reload = %v, want nordwand-mono", got)
	}
	if applyErr := reloaded.ApplyValues(map[string]any{"terminal.fontFamily": "missing-font"}); !errors.Is(applyErr, settings.ErrValidation) {
		t.Fatalf("unknown font: %v, want validation error", applyErr)
	}
	snapshot, err = reloaded.GetSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.Values["terminal.fontFamily"]; got != "nordwand-mono" {
		t.Fatalf("refused font changed selection to %v", got)
	}
	if applyErr := reloaded.ApplyValues(map[string]any{"terminal.fontFamily": "system"}); applyErr != nil {
		t.Fatal(applyErr)
	}
}
