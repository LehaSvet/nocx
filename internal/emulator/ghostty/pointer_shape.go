package ghostty

/*
#include "bridge.h"
*/
import "C"

// mouseShapeName spells the pointer shape a program asked for with OSC 22 the
// way the frame contract and the stylesheet both spell it: the W3C cursor
// names the header's enum is built from (mouse.h, "Mouse pointer shapes based
// on the W3C cursor names"). Keyed on the header's names, never on its
// numbers, for the reason this file gives at the top.
//
// One library value has no name here and reads as the empty string, which is
// the port's "the program has asked for nothing":
//
//   - TEXT is the library's INITIAL shape, so it cannot be told apart from a
//     program that asked for the I-beam on purpose (terminal.h documents
//     GHOSTTY_TERMINAL_DATA_MOUSE_SHAPE as "Initially GHOSTTY_MOUSE_SHAPE_TEXT",
//     and OSC 22 with an empty payload is dropped rather than resetting it).
//     Reporting text as a request would take the arrow away from the state
//     nocx-1ebrs.24 fixed — a program that reports the mouse and never asked —
//     so the client answers text itself wherever the terminal owns the click.
//
// A shape the header grows later falls through to the same empty string, which
// is the honest answer: this adapter cannot spell a shape the wire does not
// declare, and the client's own fallback is the pointer a person saw before
// the program asked.
func mouseShapeName(shape C.GhosttyMouseShape) string {
	switch shape {
	case C.GHOSTTY_MOUSE_SHAPE_DEFAULT:
		return "default"
	case C.GHOSTTY_MOUSE_SHAPE_CONTEXT_MENU:
		return "context-menu"
	case C.GHOSTTY_MOUSE_SHAPE_HELP:
		return "help"
	case C.GHOSTTY_MOUSE_SHAPE_POINTER:
		return "pointer"
	case C.GHOSTTY_MOUSE_SHAPE_PROGRESS:
		return "progress"
	case C.GHOSTTY_MOUSE_SHAPE_WAIT:
		return "wait"
	case C.GHOSTTY_MOUSE_SHAPE_CELL:
		return "cell"
	case C.GHOSTTY_MOUSE_SHAPE_CROSSHAIR:
		return "crosshair"
	case C.GHOSTTY_MOUSE_SHAPE_VERTICAL_TEXT:
		return "vertical-text"
	case C.GHOSTTY_MOUSE_SHAPE_ALIAS:
		return "alias"
	case C.GHOSTTY_MOUSE_SHAPE_COPY:
		return "copy"
	case C.GHOSTTY_MOUSE_SHAPE_MOVE:
		return "move"
	case C.GHOSTTY_MOUSE_SHAPE_NO_DROP:
		return "no-drop"
	case C.GHOSTTY_MOUSE_SHAPE_NOT_ALLOWED:
		return "not-allowed"
	case C.GHOSTTY_MOUSE_SHAPE_GRAB:
		return "grab"
	case C.GHOSTTY_MOUSE_SHAPE_GRABBING:
		return "grabbing"
	case C.GHOSTTY_MOUSE_SHAPE_ALL_SCROLL:
		return "all-scroll"
	case C.GHOSTTY_MOUSE_SHAPE_COL_RESIZE:
		return "col-resize"
	case C.GHOSTTY_MOUSE_SHAPE_ROW_RESIZE:
		return "row-resize"
	case C.GHOSTTY_MOUSE_SHAPE_N_RESIZE:
		return "n-resize"
	case C.GHOSTTY_MOUSE_SHAPE_E_RESIZE:
		return "e-resize"
	case C.GHOSTTY_MOUSE_SHAPE_S_RESIZE:
		return "s-resize"
	case C.GHOSTTY_MOUSE_SHAPE_W_RESIZE:
		return "w-resize"
	case C.GHOSTTY_MOUSE_SHAPE_NE_RESIZE:
		return "ne-resize"
	case C.GHOSTTY_MOUSE_SHAPE_NW_RESIZE:
		return "nw-resize"
	case C.GHOSTTY_MOUSE_SHAPE_SE_RESIZE:
		return "se-resize"
	case C.GHOSTTY_MOUSE_SHAPE_SW_RESIZE:
		return "sw-resize"
	case C.GHOSTTY_MOUSE_SHAPE_EW_RESIZE:
		return "ew-resize"
	case C.GHOSTTY_MOUSE_SHAPE_NS_RESIZE:
		return "ns-resize"
	case C.GHOSTTY_MOUSE_SHAPE_NESW_RESIZE:
		return "nesw-resize"
	case C.GHOSTTY_MOUSE_SHAPE_NWSE_RESIZE:
		return "nwse-resize"
	case C.GHOSTTY_MOUSE_SHAPE_ZOOM_IN:
		return "zoom-in"
	case C.GHOSTTY_MOUSE_SHAPE_ZOOM_OUT:
		return "zoom-out"
	case C.GHOSTTY_MOUSE_SHAPE_TEXT:
		return ""
	default:
		return ""
	}
}
