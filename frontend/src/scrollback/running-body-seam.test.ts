// ONE BODY PAYS ITS PADDING ONCE (nocx-zg3k3.14.26).
//
// A running command's rows are painted in two boxes and both are its body: the
// block paints the rows the durable record has already taken (the ones that
// left the screen) into its own `.cmd-output`, and the sibling live region
// below paints the screen the command is still writing. Each box used to wear
// the WHOLE body padding — `.cmd-output` its bottom, the live region its top —
// so two consecutive rows of one body were separated by both: 6px + 2px = 8px,
// one extra half-row. Measured on the owner's stand as a 25px step where every
// other row pair steps 17px, with 6px of bare block body under the block's last
// row.
//
// The layout half of that is e2e/running-block-body-seam.spec.ts's, which reads
// the real boxes. This is the other half and the reason it is worth pinning
// here: jsdom computes no layout, so a rule that quietly comes back is invisible
// to every unit test until the stand shows it again. Same treatment as
// live-clip.test.ts — read the shipped stylesheet and assert the contract.
import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

type Rule = { selectors: string[]; body: string }

/** Top-level rules only, comments stripped; an at-rule block is skipped whole.
 *  Lifted from live-clip.test.ts, which lifted it from cmd-output-wrap.test.ts. */
function topLevelRules(css: string): Rule[] {
  const rules: Rule[] = []
  const source = css.replace(/\/\*[\s\S]*?\*\//g, '')
  let depth = 0
  let head = ''
  let body = ''
  for (const ch of source) {
    if (ch === '{') {
      depth++
      if (depth === 1) {
        body = ''
        continue
      }
    } else if (ch === '}') {
      depth--
      if (depth === 0) {
        const selector = head.trim()
        if (!selector.startsWith('@')) {
          rules.push({ selectors: selector.split(',').map((s) => s.trim()), body })
        }
        head = ''
        continue
      }
    }
    if (depth === 0) head += ch
    else body += ch
  }
  return rules
}

const RULES = topLevelRules(
  readFileSync(resolve(import.meta.dirname ?? '.', '..', 'style.css'), 'utf8'),
)

/** What the shipped cascade resolves `property` to for one exact selector,
 *  whitespace folded so a declaration wrapped across lines compares equal.
 *  `property` is a literal name (the queries below pass `padding-bottom` and
 *  its kin), so it is matched as written. */
function declared(selector: string, property: string): string | null {
  let found: string | null = null
  const pattern = new RegExp(`(?:^|;)\\s*${property}\\s*:\\s*([^;]+)`)
  for (const rule of RULES) {
    if (!rule.selectors.includes(selector)) continue
    const m = rule.body.match(pattern)
    if (m) found = m[1].replace(/\s+/g, ' ').trim()
  }
  return found
}

describe('a running block whose body outgrew its screen (nocx-zg3k3.14.26)', () => {
  it('ends the block at its last own row instead of paying the body bottom there', () => {
    // The body continues into the live region, so this output is not its end.
    expect(declared('.cmd-block.cmd-block-running > .cmd-output', 'padding-bottom')).toBe('0')
  })

  it('starts the live region without the body top padding it already paid above', () => {
    const rule = RULES.find((r) =>
      r.selectors.some(
        (s) =>
          s.includes(':has(> .cmd-block.cmd-block-running > .cmd-output)') &&
          s.includes('.terminal-live-container.live-running'),
      ),
    )
    expect(rule, 'no rule continues the body into the live region').toBeDefined()
    expect(rule!.body).toMatch(/(?:^|;)\s*padding-top\s*:\s*0\s*(?:;|$)/)
  })

  it('still pays the body bottom once, at the live region that ends it', () => {
    // The outer edges are what the freeze's own height rests on: the live
    // region must shrink into the frozen body without moving the pane.
    expect(declared('.terminal-live-container.live-running', 'padding-block')).toBe(
      'var(--cmd-output-pad-top) calc(var(--cmd-output-pad-bottom) + var(--terminal-block-trailing-space))',
    )
    // ... and a frozen block's body still wears both of its own pads.
    expect(declared('.cmd-output', 'padding')).toBe(
      'var(--cmd-output-pad-top) 0 var(--cmd-output-pad-bottom)',
    )
  })
})
