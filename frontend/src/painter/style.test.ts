// Wire-style tests (nocx-zg3k3.2.4): the painter's merge key over
// emulator.Style, and its CSS resolution.
//
// styleEquals is what rule 2 compares — a colour is equal when its KIND and
// the fields that kind names agree, because the frame schema promises the
// other fields ride the wire zeroed and are meaningless. Two runs must not
// split because two zeroed fields happen to differ.

// @vitest-environment jsdom

import { describe, it, expect } from 'vitest'
import { DEFAULT_SNAPSHOT, paletteToRGB, type TerminalSnapshot } from '../scrollback/serializer'
import { cellAttrsOf, colorEquals, faceOf, resolveInk, styleEquals } from './style'
import { DEFAULT_COLOR, styleOf } from './fixtures'

const rgb = (r: number, g: number, b: number) => ({
  kind: 2 as const,
  palette: 0,
  rgb: { r, g, b },
})
const paletteOf = (palette: number) => ({ kind: 1 as const, palette, rgb: { r: 0, g: 0, b: 0 } })

describe('styleEquals — the merge key (ADR-0009 rule 2)', () => {
  it('splits runs whose foreground differs', () => {
    expect(styleEquals(styleOf({ foreground: rgb(1, 2, 3) }), styleOf())).toBe(false)
  })

  it('ignores the fields the colour kind does not name', () => {
    const a = styleOf({ foreground: paletteOf(5) })
    const b = styleOf({ foreground: { kind: 1, palette: 5, rgb: { r: 99, g: 99, b: 99 } } })
    expect(styleEquals(a, b)).toBe(true)
  })

  it('compares palette indexes by index, rgb colours by channel', () => {
    expect(colorEquals(paletteOf(5), paletteOf(5))).toBe(true)
    expect(colorEquals(paletteOf(5), paletteOf(6))).toBe(false)
    expect(colorEquals(rgb(1, 2, 3), rgb(1, 2, 3))).toBe(true)
    expect(colorEquals(rgb(1, 2, 3), rgb(1, 2, 4))).toBe(false)
    expect(colorEquals(paletteOf(0), DEFAULT_COLOR)).toBe(false)
  })

  it('splits runs on attributes and underline shape, and joins on everything equal', () => {
    expect(styleEquals(styleOf({ attributes: 1 }), styleOf())).toBe(false)
    expect(styleEquals(styleOf({ underline: 2 }), styleOf({ underline: 1 }))).toBe(false)
    const bold = styleOf({ attributes: 1, foreground: rgb(9, 9, 9), underline: 4 })
    const boldAgain = styleOf({ attributes: 1, foreground: rgb(9, 9, 9), underline: 4 })
    expect(styleEquals(bold, boldAgain)).toBe(true)
  })
})

describe('faceOf — what the measuring authority is asked with', () => {
  it('reads bold and italic from the attribute bitset', () => {
    expect(faceOf(styleOf({ attributes: 1 }))).toEqual({ bold: true, italic: false })
    expect(faceOf(styleOf({ attributes: 2 }))).toEqual({ bold: false, italic: true })
    expect(faceOf(styleOf({ attributes: 3 }))).toEqual({ bold: true, italic: true })
    expect(faceOf(styleOf())).toEqual({ bold: false, italic: false })
  })
})

describe('resolveInk — the wire vocabulary to CSS', () => {
  it('paints nothing for a default cell: it inherits the surface (nocx-6w4z)', () => {
    expect(resolveInk(styleOf(), DEFAULT_SNAPSHOT)).toEqual({ css: '', hasBackground: false })
  })

  it('resolves exact rgb and palette colours', () => {
    expect(resolveInk(styleOf({ foreground: rgb(250, 240, 230) }), DEFAULT_SNAPSHOT).css).toContain(
      'color:rgb(250, 240, 230)',
    )
    expect(resolveInk(styleOf({ foreground: paletteOf(2) }), DEFAULT_SNAPSHOT).css).toContain(
      'color:#9ece6a',
    )
    const bg = resolveInk(styleOf({ background: rgb(0, 1, 2) }), DEFAULT_SNAPSHOT)
    expect(bg.css).toContain('background:rgb(0, 1, 2)')
    expect(bg.hasBackground).toBe(true)
  })
  it('swaps the resolved colours on inverse: an inverse cell asks for both', () => {
    const ink = resolveInk(
      styleOf({ foreground: rgb(255, 255, 255), attributes: 16 }),
      DEFAULT_SNAPSHOT,
    )
    // The swapped-in foreground is the theme's own background colour — an
    // explicit ask, painted as such, exactly as the frozen path's
    // attrsToStyle resolves an inverse cell (nocx-6w4z omits what a cell
    // never asked for; this one asked).
    expect(ink.css).toContain('background:rgb(255, 255, 255)')
    expect(ink.css).toContain(`color:${DEFAULT_SNAPSHOT.defaultBg}`)
    expect(ink.hasBackground).toBe(true)
  })

  it('hides the ink for invisible without losing the background', () => {
    const ink = resolveInk(styleOf({ background: rgb(0, 1, 2), attributes: 32 }), DEFAULT_SNAPSHOT)
    expect(ink.css).toContain('color:transparent')
    expect(ink.css).toContain('background:rgb(0, 1, 2)')
  })

  it('carries the attribute asks the program made', () => {
    expect(resolveInk(styleOf({ attributes: 1 }), DEFAULT_SNAPSHOT).css).toContain(
      'font-weight:bold',
    )
    expect(resolveInk(styleOf({ attributes: 2 }), DEFAULT_SNAPSHOT).css).toContain(
      'font-style:italic',
    )
    expect(resolveInk(styleOf({ attributes: 4 }), DEFAULT_SNAPSHOT).css).toContain('opacity:0.5')
  })

  // xterm's drawBoldTextInBrightColors, on by default in the 0.4 renderer:
  // bold text in one of the eight base colours is drawn in its bright twin.
  // Programs rely on it — htop's "[Main]" brackets are bold green on green,
  // and without it they vanished (nocx-zg3k3.27).
  it('draws bold text in the base eight colours in their bright twins', () => {
    // Distinct twins, and every colour legible on the default background, so
    // the floor below changes none of them and the brightening is all that is
    // asserted.
    const SNAP: TerminalSnapshot = {
      ...DEFAULT_SNAPSHOT,
      defaultBg: '#000000',
      palette: [
        '#80ffff',
        '#84ffff',
        '#88ffff',
        '#8cffff',
        '#90ffff',
        '#94ffff',
        '#98ffff',
        '#9cffff',
        '#a0ffff',
        '#a4ffff',
        '#a8ffff',
        '#acffff',
        '#b0ffff',
        '#b4ffff',
        '#b8ffff',
        '#bcffff',
      ],
    }
    const bright = paletteToRGB(SNAP, 10)
    expect(resolveInk(styleOf({ attributes: 1, foreground: paletteOf(2) }), SNAP).css).toContain(
      `color:${bright}`,
    )
    expect(cellAttrsOf(styleOf({ attributes: 1, foreground: paletteOf(2) }), SNAP).fg).toBe(bright)
    // Not bold, or a 256-colour index: as asked.
    expect(resolveInk(styleOf({ foreground: paletteOf(2) }), SNAP).css).toContain(
      `color:${paletteToRGB(SNAP, 2)}`,
    )
    expect(resolveInk(styleOf({ attributes: 1, foreground: paletteOf(196) }), SNAP).css).toContain(
      `color:${paletteToRGB(SNAP, 196)}`,
    )
    // The background is never brightened.
    expect(resolveInk(styleOf({ attributes: 1, background: paletteOf(2) }), SNAP).css).toContain(
      `background:${paletteToRGB(SNAP, 2)}`,
    )
  })

  // The readability floor (nocx-zg3k3.27): text the same colour as its
  // background is drawn readable, as 0.4's xterm drew it.
  it('raises a foreground that would vanish into its background', () => {
    const css = resolveInk(
      styleOf({ foreground: paletteOf(2), background: paletteOf(2) }),
      DEFAULT_SNAPSHOT,
    ).css
    expect(css).not.toContain(`color:${paletteToRGB(DEFAULT_SNAPSHOT, 2)}`)
    expect(css).toMatch(/color:rgb\(/)
  })

  // FAINT IS INTENSITY, AND INTENSITY IS THE PROGRAM'S TO CHOOSE
  // (nocx-1ebrs.16). The floor above is right for text a person must read. A
  // cell the program marked SGR 2 is the program saying the opposite, and the
  // order the two were applied in made it worse than either: the colour was
  // lifted to the floor FIRST and cut to half opacity afterwards, so a
  // deliberately faint grey separator painted brighter than the plain grey
  // beside it. Every other terminal honours the program here.
  it('leaves a faint cell at the intensity the program asked for', () => {
    // Below the floor on the default background, so the plain cell is lifted
    // and the faint one must not be.
    const grey = rgb(90, 90, 90)
    const plain = resolveInk(styleOf({ foreground: grey }), DEFAULT_SNAPSHOT)
    expect(plain.css).not.toContain('color:rgb(90, 90, 90)')
    const ink = resolveInk(styleOf({ foreground: grey, attributes: 4 }), DEFAULT_SNAPSHOT)
    expect(ink.css).toContain('color:rgb(90, 90, 90)')
    expect(ink.css).toContain('opacity:0.5')
  })

  it('paints a faint cell dimmer than the same cell without the bit', () => {
    // The owner's assertion, stated as the pair it is about rather than as a
    // string: however the ink is resolved, the SGR 2 cell cannot come out at
    // more intensity than the identical cell without it.
    const grey = rgb(128, 128, 128)
    const plain = rgbOf(resolveInk(styleOf({ foreground: grey }), DEFAULT_SNAPSHOT).css)
    const faint = rgbOf(
      resolveInk(styleOf({ foreground: grey, attributes: 4 }), DEFAULT_SNAPSHOT).css,
    )
    const intensity = (c: readonly [number, number, number]) => c[0] + c[1] + c[2]
    expect(intensity(faint)).toBeLessThanOrEqual(intensity(plain))
    // And the reduction is real: half opacity over the same ink.
    expect(
      resolveInk(styleOf({ foreground: grey, attributes: 4 }), DEFAULT_SNAPSHOT).css,
    ).toContain('opacity:0.5')
  })

  it('maps the underline shapes the wire enumerates', () => {
    expect(resolveInk(styleOf({ underline: 1 }), DEFAULT_SNAPSHOT).css).toContain(
      'text-decoration:underline',
    )
    expect(resolveInk(styleOf({ underline: 2 }), DEFAULT_SNAPSHOT).css).toContain(
      'text-decoration:underline double',
    )
    expect(resolveInk(styleOf({ underline: 3 }), DEFAULT_SNAPSHOT).css).toContain(
      'text-decoration:underline wavy',
    )
    expect(resolveInk(styleOf({ underline: 5 }), DEFAULT_SNAPSHOT).css).toContain(
      'text-decoration:underline dashed',
    )
    const both = resolveInk(styleOf({ underline: 1, attributes: 64 }), DEFAULT_SNAPSHOT)
    expect(both.css).toContain('text-decoration:underline line-through')
  })

  it('colours the decoration when the wire names an underline colour', () => {
    const ink = resolveInk(
      styleOf({ underline: 1, underlineColor: rgb(1, 2, 3) }),
      DEFAULT_SNAPSHOT,
    )
    expect(ink.css).toContain('text-decoration:underline')
    expect(ink.css).toContain('text-decoration-color:rgb(1, 2, 3)')
  })
})

/** The `color:` a resolved ink declares, as channels. The tests above compare
 *  strings; the faint pair compares INTENSITY, which needs the numbers. */
function rgbOf(css: string): readonly [number, number, number] {
  const match = /color:rgb\((\d+),\s*(\d+),\s*(\d+)\)/.exec(css)
  if (match === null) throw new Error(`no rgb colour in ${css}`)
  return [Number(match[1]), Number(match[2]), Number(match[3])]
}
