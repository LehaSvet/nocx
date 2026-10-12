// ═══════════════════════════════════════════════════════════════════════════
// The LIVE region's policy — applied to links derived from accepted cells.
//
// Same grammar, same opener, same armed tracker as the frozen scrollback;
// only the plumbing differs, because one half of the terminal is a grid and
// the other is DOM. If this file ever grows a second opinion about what a
// link is, the two halves of one terminal will disagree about one row as it
// scrolls from one to the other.
//
// OSC 8 arrives here and NOT through the grammar: the url a program declared
// is not the text it printed — that is the entire point of the sequence — so
// it cannot be re-derived from the row. ADR-0029 governs what happens next:
// the program may ask, and nocx chooses. It chooses to honour http(s) and to
// refuse everything else out loud, because a program that can make the
// terminal follow `file://` or a custom scheme has been handed a capability
// nobody granted it.
// ═══════════════════════════════════════════════════════════════════════════

import type { ActiveOrigin } from '../pane-content'
import { detectLinks } from './detect'
import type { LinkOpener } from './open'
import type { ArmedTracker } from './armed'

export interface LivePolicyDeps {
  readonly opener: LinkOpener
  readonly origin: () => Omit<ActiveOrigin, 'paneId'> | null
  readonly armed: ArmedTracker
  /** Say why an OSC 8 link was not followed. */
  readonly notify: (message: string) => void
}

/** Schemes a declared hyperlink may use. Deliberately the same two the
 *  grammar detects and the same two `shell.openUrl` will accept — a third
 *  place that decides this would be a third answer. */
const ALLOWED_SCHEME = /^https?:\/\//i

interface RendererLinkRange {
  from: number
  to: number
}

export interface LivePolicy {
  ranges(lineText: string): RendererLinkRange[]
  activate(lineText: string, from: number, to: number): void
  activateAt(lineText: string, offset: number): void
  activateHyperlink(url: string): void
}

export function createLivePolicy(deps: LivePolicyDeps): LivePolicy {
  const activate = (lineText: string, from: number, to: number): void => {
    const span = detectLinks(lineText).find((s) => s.from === from && s.to === to)
    if (span === undefined) return
    void deps.opener.open(span.target, deps.origin())
  }
  return {
    ranges(lineText: string): RendererLinkRange[] {
      // Unarmed means no links exist, not "links that ignore clicks": the
      // engine underlines whatever this returns, and an underline under a
      // dead click is a promise the terminal cannot keep.
      if (!deps.armed.armed()) return []
      return detectLinks(lineText).map((s) => ({ from: s.from, to: s.to }))
    },

    activate(lineText: string, from: number, to: number): void {
      // Re-derived rather than carried: the grammar is deterministic over
      // the row's text, and holding a target on the renderer would fork it.
      activate(lineText, from, to)
    },

    activateAt(lineText: string, offset: number): void {
      const span = detectLinks(lineText).find((s) => offset >= s.from && offset < s.to)
      if (span !== undefined) activate(lineText, span.from, span.to)
    },

    activateHyperlink(url: string): void {
      if (!ALLOWED_SCHEME.test(url)) {
        deps.notify(`Refused a link the program declared: nocx opens http and https only (${url})`)
        return
      }
      void deps.opener.open({ kind: 'url', url }, deps.origin())
    },
  }
}
