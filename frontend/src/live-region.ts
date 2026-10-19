// THE TERMINAL'S OWN PAINTED SURFACE, WHEREVER IT IS SEATED (ADR-0091).
//
// One question, asked by every gesture that meets the pane's DOM: does this
// node belong to the TERMINAL — the cell painter's rows and the grid they are
// drawn in, the hidden input element that carries the keyboard — or to a
// command BLOCK?
//
// Before ADR-0091 the answer was a fact about the tree, and so it was never
// named: the live region was a direct child of `.scrollback-inner`, no block
// contained it, and `closest('.cmd-block')` agreed with "is this the terminal"
// everywhere it was asked. ADR-0091 makes the region a RUNNING BLOCK'S BODY,
// because that is what puts the block's box and its ground over every row the
// command writes — and the two questions come apart. A press on a live row now
// has a block ancestor, so every guard written as `closest('.cmd-block')`
// answered "a block owns this" about a row the painter owns.
//
// THE SEAT IS NOT THE OWNER. The DOM position moves; which module answers a
// press, a drag, a word, a wheel and a keystroke does not. The grid's gestures
// read the CELL SNAPSHOT (painter/), the block's gestures read its own rows
// (scrollback/blocks.ts), and the keyboard is the pane's input element — none
// of which changes with the region's seat. So the question gets one name, one
// implementation and one answer for every reader, here.
//
// MEASURED, all three on the branch that moved the region (nocx-zg3k3.14.26):
// the block's click-to-select claimed a press on the grid, so the block-
// selection keyboard branch ate the next character typed into a running
// program (e2e/live-to-card-joined.spec.ts) and the Escape that should have
// stopped a streaming answer deselected the block instead
// (e2e/ask-about-a-running-command.spec.ts); and the painter's own word rule
// stood down for a row it took for card content, so a double click copied one
// DOM run of cells — `dbl` — instead of `~/dbl-mvf8dc0n.txt`
// (e2e/clipboard.spec.ts).
export const LIVE_REGION_CLASS = 'terminal-live-container'

/** The same identity as a selector, for the readers that ask the DOM rather
 *  than a node: `closest`, `querySelector`, `matches`. */
export const LIVE_REGION_SELECTOR = `.${LIVE_REGION_CLASS}`

/**
 * True when this node is inside the terminal's own painted surface.
 *
 * Takes a `Node` or an event target because that is what a handler holds: a
 * listener's `event.target` may be the element, a text node inside a painted
 * row, or something that is not a node at all. A non-element resolves to the
 * element that contains it, which is the same question one level up.
 */
export function insideLiveRegion(node: Node | EventTarget | null): boolean {
  const el = node instanceof Element ? node : node instanceof Node ? node.parentElement : null
  return el !== null && el.closest(LIVE_REGION_SELECTOR) !== null
}
