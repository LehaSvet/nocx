package sessionruntime

import (
	"testing"

	"github.com/shady2k/nocx/internal/emulator"
)

// TestKeyNamesCarryTheKeypadsEnterAsItsOwnKey is the WIRE half of
// nocx-1ebrs.32. The pane reads a physical key and names it; this table is the
// whole of what the runtime accepts, and a name it does not carry is refused
// rather than guessed. Before the fix the table had "enter" only, so the
// keypad's Enter had no spelling to send and the program received nothing.
//
// The two names are asserted to be DIFFERENT identities on purpose. Folding
// the keypad's Enter into "enter" would make the pane's own silence into a
// plausible-looking CR and lose the case where the program enabled application
// keypad mode, which the emulator decides from the key it is given.
func TestKeyNamesCarryTheKeypadsEnterAsItsOwnKey(t *testing.T) {
	returnKey, err := keyEvent([]byte("enter"))
	if err != nil {
		t.Fatalf("enter: %v", err)
	}
	if returnKey.Key != emulator.KeyEnter {
		t.Errorf("\"enter\" parsed as key %d, want KeyEnter", returnKey.Key)
	}

	keypad, err := keyEvent([]byte("numpadenter"))
	if err != nil {
		t.Fatalf("numpadenter: %v", err)
	}
	if keypad.Key != emulator.KeyNumpadEnter {
		t.Errorf("\"numpadenter\" parsed as key %d, want KeyNumpadEnter", keypad.Key)
	}
	if keypad.Key == returnKey.Key {
		t.Error("the keypad's Enter and Return parse to one key: the mode that tells them apart could never be applied")
	}

	// The name a person says, spelled the way they say it, is the same key: the
	// table matches case-insensitively and the modifiers are carried, not
	// swallowed.
	chorded, err := keyEvent([]byte("Shift+NumpadEnter"))
	if err != nil {
		t.Fatalf("Shift+NumpadEnter: %v", err)
	}
	if chorded.Key != emulator.KeyNumpadEnter || !chorded.Mods.Shift {
		t.Errorf("\"Shift+NumpadEnter\" parsed as key %d with mods %+v, want the keypad's Enter with Shift", chorded.Key, chorded.Mods)
	}
}
