package ghostty

// THE POINTER THE PROGRAM ASKS FOR (nocx-1ebrs.24). OSC 22 is the escape a
// program uses to ask for a specific pointer shape, and a terminal that
// honours it reads the request out of its own terminal state — never out of
// the byte stream, which the backend does not sniff (AD-6). So the assertion
// is made where the request lands: ingest the escape, read the modes.
//
// The vocabulary is the wire's, not this package's: the names below are read
// out of contracts/session.frame.schema.json, so a shape the adapter can
// spell and the wire cannot carry — or one the wire declares and the adapter
// drops — fails here rather than in a pane nobody can explain.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const pointerShapeContract = "../../../contracts/session.frame.schema.json"

// wirePointerShapes is the `pointerShape` vocabulary the frame contract
// declares, minus null: the shapes a program can ask for.
func wirePointerShapes(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(pointerShapeContract)
	if err != nil {
		t.Fatalf("read the frame contract: %v", err)
	}
	var schema struct {
		Properties struct {
			State struct {
				Properties struct {
					PointerShape struct {
						Enum []*string `json:"enum"`
					} `json:"pointerShape"`
				} `json:"properties"`
			} `json:"state"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("parse the frame contract: %v", err)
	}
	var names []string
	for _, name := range schema.Properties.State.Properties.PointerShape.Enum {
		if name != nil {
			names = append(names, *name)
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s declares no pointerShape vocabulary", filepath.Base(pointerShapeContract))
	}
	return names
}

// TestThePointerShapeIsWhatTheProgramAskedFor is the state the person sees:
// nothing asked before an OSC 22 arrives, the requested shape afterwards, and
// the last good request still standing when the program asks for a shape the
// terminal has never heard of.
func TestThePointerShapeIsWhatTheProgramAskedFor(t *testing.T) {
	term := newTerminal(t, 20, 4)
	// A program that has asked for nothing reads as nothing: the library's
	// INITIAL shape is text, and "text" is what the client answers on its own
	// when no program asks, so reporting text here would claim a request
	// nobody made.
	if got, err := term.Modes(); err != nil || got.PointerShape != "" {
		t.Fatalf("the pointer shape before any request = %q, %v; want none", got.PointerShape, err)
	}

	// Two spellings of one request, both legal: BEL-terminated and ST-terminated.
	ingest(t, term, "\x1b]22;wait\x07")
	if got, err := term.Modes(); err != nil || got.PointerShape != "wait" {
		t.Fatalf("after OSC 22 wait the shape = %q, %v; want wait", got.PointerShape, err)
	}
	ingest(t, term, "\x1b]22;crosshair\x1b\\")
	if got, err := term.Modes(); err != nil || got.PointerShape != "crosshair" {
		t.Fatalf("after OSC 22 crosshair the shape = %q, %v; want crosshair", got.PointerShape, err)
	}

	// A name the terminal does not know is not a shape: the library drops it
	// with a warning and the previous request stands, which is what a person
	// keeps seeing instead of a pointer that silently changed under them.
	ingest(t, term, "\x1b]22;not-a-cursor\x07")
	if got, err := term.Modes(); err != nil || got.PointerShape != "crosshair" {
		t.Fatalf("after an unknown shape the pointer shape = %q, %v; want the last request kept", got.PointerShape, err)
	}

	// The request the client must NOT report: text is the state of a program
	// that asked for nothing, and the C API cannot separate an explicit text
	// request from it (GHOSTTY_TERMINAL_DATA_MOUSE_SHAPE is documented as
	// "Initially GHOSTTY_MOUSE_SHAPE_TEXT"). The client's own fallback answers
	// text wherever the terminal owns the click, so reporting a request here
	// would only take the arrow away from a program that reports the mouse.
	ingest(t, term, "\x1b]22;text\x07")
	if got, err := term.Modes(); err != nil || got.PointerShape != "" {
		t.Fatalf("after OSC 22 text the shape = %q, %v; want none: the client answers text itself", got.PointerShape, err)
	}
}

// TestEveryShapeOnTheWireRoundTripsThroughOSC22 is the vocabulary: every name
// the frame contract declares is a name the emulator accepts and reports back
// unchanged. A shape added to the wire without a spelling here, or spelled
// differently, is a request a program makes that never reaches the pane.
func TestEveryShapeOnTheWireRoundTripsThroughOSC22(t *testing.T) {
	for _, name := range wirePointerShapes(t) {
		if name == "text" {
			// Not a request the client can act on — see the case above.
			continue
		}
		term := newTerminal(t, 20, 4)
		ingest(t, term, "\x1b]22;"+name+"\x07")
		got, err := term.Modes()
		if err != nil {
			t.Fatalf("OSC 22 %s: %v", name, err)
		}
		if got.PointerShape != name {
			t.Errorf("OSC 22 %s reported as %q; want the same name back", name, got.PointerShape)
		}
	}
}
