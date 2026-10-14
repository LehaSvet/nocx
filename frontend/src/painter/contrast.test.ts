import { describe, expect, it } from 'vitest'
import { legibleForeground } from './contrast'

describe('legibleForeground — xterm’s readability floor (nocx-zg3k3.27)', () => {
  it('leaves a pair that already meets 4.5:1 exactly as declared', () => {
    expect(legibleForeground('#ffffff', '#000000')).toBe('#ffffff')
    expect(legibleForeground('rgb(0, 0, 0)', '#ffffff')).toBe('rgb(0, 0, 0)')
  })

  it('makes green on the same green readable (htop’s tab brackets)', () => {
    const out = legibleForeground('#9ece6a', '#9ece6a')
    expect(out).not.toBe('#9ece6a')
    expect(contrast(out, '#9ece6a')).toBeGreaterThanOrEqual(4.5)
  })

  it('darkens a light foreground over a light background, as xterm does', () => {
    // mc's yellow header over its light-blue panel in the 0.4 palette.
    const out = legibleForeground('#e0af68', '#7aa2f7')
    expect(lum(out)).toBeLessThan(lum('#e0af68'))
    expect(contrast(out, '#7aa2f7')).toBeGreaterThanOrEqual(4.5)
  })

  it('passes a colour it cannot read through unchanged', () => {
    expect(legibleForeground('var(--x)', '#000000')).toBe('var(--x)')
    expect(legibleForeground('transparent', '#000000')).toBe('transparent')
  })
})

function rgbOf(css: string): [number, number, number] {
  const hex = /^#([0-9a-f]{6})$/i.exec(css)
  if (hex) {
    const n = parseInt(hex[1], 16)
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255]
  }
  const m = /rgb\((\d+), (\d+), (\d+)\)/.exec(css)!
  return [Number(m[1]), Number(m[2]), Number(m[3])]
}
function lum(css: string): number {
  const c = (v: number) => {
    const s = v / 255
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
  }
  const [r, g, b] = rgbOf(css)
  return 0.2126 * c(r) + 0.7152 * c(g) + 0.0722 * c(b)
}
function contrast(a: string, b: string): number {
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p)
  return (x + 0.05) / (y + 0.05)
}
