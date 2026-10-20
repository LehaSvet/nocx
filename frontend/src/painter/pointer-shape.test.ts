// The pointer's SHAPE is a stylesheet fact the painter's state decides
// (nocx-1ebrs.24), and jsdom computes neither — so this pins the shipped
// cascade the way scrollback/live-clip.test.ts does: read style.css, find the
// rule, assert the declaration.
//
// The two halves are one behaviour and are tested apart on purpose. The
// painter writes `data-pointer` from the accepted frame's `mouseTracking` bit
// (painter.test.ts), and these rules turn that state into the pointer a person
// sees. A missing rule is silently an I-beam, which is the defect, so the
// stylesheet half has to be asserted somewhere.
import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { POINTER_ATTR, type PointerOwner } from './painter'

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

const stateSelector = (owner: PointerOwner): string => `.term-grid[${POINTER_ATTR}='${owner}']`

describe('the pointer shows who owns the click (nocx-1ebrs.24)', () => {
  it('is the arrow while the program reports the mouse', () => {
    expect(cursorFor(stateSelector('program'))).toBe('default')
  })

  it('is the I-beam while the terminal owns selection', () => {
    // The bare surface, which is what a `text` cell resolves through too.
    expect(cursorFor('.term-grid')).toBe('text')
  })

  it('states the state instead of inheriting `auto`', () => {
    // `auto` over rows of text IS an I-beam, so a rule that says nothing looks
    // correct in the one state that was ever noticed and is wrong in the other.
    expect(cursorFor('.term-grid')).not.toBe('auto')
  })
})
