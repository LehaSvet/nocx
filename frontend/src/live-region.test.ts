// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { LIVE_REGION_CLASS, LIVE_REGION_SELECTOR, insideLiveRegion } from './live-region'

describe('the live region identity (ADR-0091)', () => {
  it('names the surface the controller builds', () => {
    const region = document.createElement('div')
    region.className = LIVE_REGION_CLASS
    expect(region.matches(LIVE_REGION_SELECTOR)).toBe(true)
  })

  it('answers for a node inside the region, at any depth', () => {
    const region = document.createElement('div')
    region.className = LIVE_REGION_CLASS
    const viewport = document.createElement('div')
    const row = document.createElement('div')
    const cell = document.createElement('span')
    const text = document.createTextNode('out')
    cell.appendChild(text)
    row.appendChild(cell)
    viewport.appendChild(row)
    region.appendChild(viewport)
    document.body.appendChild(region)
    try {
      expect(insideLiveRegion(region)).toBe(true)
      expect(insideLiveRegion(viewport)).toBe(true)
      expect(insideLiveRegion(row)).toBe(true)
      expect(insideLiveRegion(cell)).toBe(true)
      // A TEXT node is a node a handler can target: it resolves to the
      // element that contains it, which is the same question one level up.
      expect(insideLiveRegion(text)).toBe(true)
    } finally {
      region.remove()
    }
  })

  it('answers NO for the block around the region and for a sibling of it', () => {
    // The seat does not change the answer: the block around the region owns
    // its own rows, never the terminal's.
    const block = document.createElement('div')
    block.className = 'cmd-block'
    const output = document.createElement('div')
    output.className = 'cmd-output'
    const region = document.createElement('div')
    region.className = LIVE_REGION_CLASS
    block.append(output, region)
    const behind = document.createElement('div')
    document.body.append(block, behind)
    try {
      expect(insideLiveRegion(block)).toBe(false)
      expect(insideLiveRegion(output)).toBe(false)
      expect(insideLiveRegion(behind)).toBe(false)
      expect(insideLiveRegion(document.body)).toBe(false)
      // Nothing at all is an answer too — a listener may fire with a target
      // that is not a node (a window, a synthetic event).
      expect(insideLiveRegion(null)).toBe(false)
      expect(insideLiveRegion(window)).toBe(false)
    } finally {
      block.remove()
      behind.remove()
    }
  })
})
