// The pointer's SHAPE is a stylesheet fact the painter's state decides
// (nocx-1ebrs.24), and jsdom computes neither — so this pins the shipped
// cascade the way scrollback/live-clip.test.ts does: read style.css, find the
// rule, assert the declaration.
//
// The state has two sources and one answer. WHO OWNS THE CLICK is the frame's
// `mouseTracking` bit, the same one selection-gesture.ts reads; WHAT THE
// PROGRAM ASKED FOR is the frame's `pointerShape`, set by OSC 22. `pointerFor`
// is that one answer, and these rules turn it into the pointer a person sees.
// A missing rule is silently an I-beam, which is the defect, so the stylesheet
// half has to be asserted somewhere — including for every shape the wire can
// carry, which is what the last case walks.
import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { POINTER_ATTR, pointerFor, type PointerShape } from './painter'

/** Every top-level rule of the shipped stylesheet, comments stripped. */
function rules(): Array<{ selector: string; body: string }> {
  const source = readFileSync(
    resolve(import.meta.dirname ?? '.', '..', 'style.css'),
    'utf8',
  ).replace(/\/\*[\s\S]*?\*\//g, '')
  const out: Array<{ selector: string; body: string }> = []
  for (const match of source.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    out.push({ selector: match[1].trim(), body: match[2] })
  }
  return out
}

/** The `cursor` the cascade declares for one exact selector. */
function cursorFor(selector: string): string | null {
  let found: string | null = null
  for (const rule of rules()) {
    if (rule.selector !== selector) continue
    const declared = /(?:^|;)\s*cursor\s*:\s*([^;]+)/.exec(rule.body)
    if (declared) found = declared[1].trim()
  }
  return found
}

const stateSelector = (shape: PointerShape): string => `.term-grid[${POINTER_ATTR}='${shape}']`

/** The shapes the FRAME CONTRACT declares, minus null: one declaration of the
 *  vocabulary, read rather than copied, so a shape added to the wire and left
 *  without a rule here fails instead of showing the base I-beam. */
function wireShapes(): PointerShape[] {
  const schema = JSON.parse(
    readFileSync(
      resolve(
        import.meta.dirname ?? '.',
        '..',
        '..',
        '..',
        'contracts',
        'session.frame.schema.json',
      ),
      'utf8',
    ),
  ) as { properties: { state: { properties: { pointerShape: { enum: Array<string | null> } } } } }
  return schema.properties.state.properties.pointerShape.enum.filter(
    (name): name is PointerShape => name !== null,
  )
}

/** One accepted frame's pointer facts, as the backend measured them. */
const pointed = (mouseTracking: boolean, pointerShape: PointerShape | null = null) =>
  pointerFor({ mouseTracking, pointerShape })

describe('the pointer shows who owns the click (nocx-1ebrs.24)', () => {
  it('is the arrow while the program reports the mouse', () => {
    expect(pointed(true)).toBe('default')
    expect(cursorFor(stateSelector('default'))).toBe('default')
  })

  it('is the I-beam while the terminal owns selection', () => {
    // The bare surface, which is what a `text` state resolves through too.
    expect(pointed(false)).toBe('text')
    expect(cursorFor('.term-grid')).toBe('text')
    expect(cursorFor(stateSelector('text'))).toBe('text')
  })

  it('states the state instead of inheriting `auto`', () => {
    // `auto` over rows of text IS an I-beam, so a rule that says nothing looks
    // correct in the one state that was ever noticed and is wrong in the other.
    expect(cursorFor('.term-grid')).not.toBe('auto')
  })
})

describe('the pointer the program asked for with OSC 22 (nocx-1ebrs.24)', () => {
  it('is the shape the program named, whether or not it reports the mouse', () => {
    // The request wins in both states: a full-screen program that asked for
    // `wait` is busy, and one that asked for `crosshair` is drawing, whether
    // or not its clicks are its own.
    expect(pointed(true, 'wait')).toBe('wait')
    expect(pointed(false, 'crosshair')).toBe('crosshair')
  })

  it('takes the request back when the program stops asking', () => {
    // `null` is the wire's "asked for nothing", and the click owner answers
    // again — the whole point of the fallback.
    expect(pointed(false, 'wait')).toBe('wait')
    expect(pointed(false, null)).toBe('text')
    expect(pointed(true, null)).toBe('default')
  })

  it('gives every shape the wire can carry a cursor of its own', () => {
    // The names ARE the CSS cursor vocabulary (W3C cursor names, which is what
    // the emulator's own enumeration is built from), so the rule for a shape
    // must declare exactly that name. A shape the sheet has no rule for would
    // fall back to the base I-beam under a program that asked for something
    // else, which is a capability we would be advertising and not
    // implementing.
    const shapes = wireShapes()
    expect(shapes.length).toBeGreaterThan(1)
    for (const shape of shapes) {
      expect(cursorFor(stateSelector(shape)), `no cursor rule for ${shape}`).toBe(shape)
    }
  })
})
