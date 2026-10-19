// The wire style → CSS, and the merge key over wire styles (nocx-zg3k3.2.4).
//
// Run-geometry's rule 2 merges adjacent cells only when foreground,
// background, extended attributes AND spacing agree. The first three are
// compared here, in the wire's own vocabulary (emulator.Style): a colour
// equals another when its KIND and the fields that kind names agree — the
// frame schema promises the other fields ride the wire zeroed and are
// meaningless, so they must never split a run.
//
// The CSS half paints what the program asked for and nothing else: a
// default-coloured cell inherits the surface (the nocx-6w4z lesson, carried
// over the wire vocabulary), and `inverse` swaps the two RESOLVED colours
// before the default check, matching attrsToStyle's frozen-cell semantics.
// The live painter and accepted-frame capture share this wire-style owner;
// the stored-row serializer retains its adapter for its separate cell input.

import type { ModelColor as Color, ModelStyle as Style } from '../cell-model'
import type { FitFace } from '../scrollback/cell-fit'
import { paletteToRGB, type CellAttrs, type TerminalSnapshot } from '../scrollback/serializer'
import { inkOf } from './contrast'

// emulator.Attributes: bit 0 bold, 1 italic, 2 faint, 3 blink, 4 inverse,
// 5 invisible, 6 strikethrough, 7 overline. The set is closed — a terminal
// has these eight and no ninth (the frame schema's words).
const BOLD = 1 << 0
const ITALIC = 1 << 1
const FAINT = 1 << 2
// Bit 3 (blink) is carried and never painted: CSS has no blink the live
// region could honour, and the frozen path paints none either.
const INVERSE = 1 << 4
const INVISIBLE = 1 << 5
const STRIKETHROUGH = 1 << 6
const OVERLINE = 1 << 7

/** Rule 2's colour comparison, kind-sensitive by construction. */
export function colorEquals(a: Color, b: Color): boolean {
  if (a.kind !== b.kind) return false
  if (a.kind === 1) return a.palette === b.palette
  if (a.kind === 2) return a.rgb.r === b.rgb.r && a.rgb.g === b.rgb.g && a.rgb.b === b.rgb.b
  return true
}

/** Rule 2's attribute comparison: the whole style, in the wire's vocabulary. */
export function styleEquals(a: Style, b: Style): boolean {
  return (
    colorEquals(a.foreground, b.foreground) &&
    colorEquals(a.background, b.background) &&
    colorEquals(a.underlineColor, b.underlineColor) &&
    a.attributes === b.attributes &&
    a.underline === b.underline
  )
}

/** The face the measuring authority measures with — the only thing it asks
 *  about a style, whichever vocabulary the style arrived in. */
export function faceOf(style: Style): FitFace {
  return { bold: (style.attributes & BOLD) !== 0, italic: (style.attributes & ITALIC) !== 0 }
}

function colorCSS(color: Color, palette: TerminalSnapshot): string | null {
  if (color.kind === 1) return paletteToRGB(palette, color.palette)
  if (color.kind === 2) return `rgb(${color.rgb.r}, ${color.rgb.g}, ${color.rgb.b})`
  return null
}

/** The foreground as a terminal draws it: bold text in one of the eight base
 *  colours takes its bright twin — xterm's drawBoldTextInBrightColors, on in
 *  0.4's renderer (nocx-zg3k3.27). The background is never brightened. */
function foregroundCSS(style: Style, palette: TerminalSnapshot): string | null {
  const color = style.foreground
  if (color.kind === 1 && color.palette < 8 && (style.attributes & BOLD) !== 0) {
    return paletteToRGB(palette, color.palette + 8)
  }
  return colorCSS(color, palette)
}

export interface ResolvedInk {
  /** The CSS declarations for the run; empty when the cell asked for
   *  nothing and a bare text node may carry it. */
  readonly css: string
  /** True when the run declares a background — the painter's cue for rule
   *  4's measured vertical padding. */
  readonly hasBackground: boolean
}

/** One run's style, resolved. The decoration half maps the wire's closed
 *  attribute set and its underline enumeration; the richer underline
 *  colours ride text-decoration-color, so nothing is dropped on the way to
 *  CSS. */
export function resolveInk(style: Style, palette: TerminalSnapshot): ResolvedInk {
  const parts: string[] = []
  let fg = foregroundCSS(style, palette) ?? palette.defaultFg
  let bg = colorCSS(style.background, palette) ?? palette.defaultBg
  const attrs = style.attributes
  if ((attrs & INVERSE) !== 0) [fg, bg] = [bg, fg]
  // Invisible suppresses the ink, never the background. Otherwise the ink
  // meets the readability floor against the background it sits on.
  // A cell that asked for no colour at all inherits the surface (nocx-6w4z)
  // and is left out of the floor, which would otherwise write the theme's own
  // pair onto it.
  const asked =
    style.foreground.kind !== 0 || style.background.kind !== 0 || (attrs & INVERSE) !== 0
  const ink =
    (attrs & INVISIBLE) !== 0 ? 'transparent' : asked ? inkOf(fg, bg, (attrs & FAINT) !== 0) : fg
  if (ink !== palette.defaultFg) parts.push(`color:${ink}`)
  if (bg !== palette.defaultBg) parts.push(`background:${bg}`)
  if ((attrs & BOLD) !== 0) parts.push('font-weight:bold')
  if ((attrs & ITALIC) !== 0) parts.push('font-style:italic')
  if ((attrs & FAINT) !== 0) parts.push('opacity:0.5')
  const lines: string[] = []
  // The wire enumerates six underline shapes (SGR 4:0-4:5); CSS spells
  // each one, so all but the none-shape are painted.
  if (style.underline !== 0) lines.push('underline')
  if ((attrs & STRIKETHROUGH) !== 0) lines.push('line-through')
  if ((attrs & OVERLINE) !== 0) lines.push('overline')
  if (lines.length > 0) {
    const shape =
      style.underline === 2
        ? ' double'
        : style.underline === 3
          ? ' wavy'
          : style.underline === 4
            ? ' dotted'
            : style.underline === 5
              ? ' dashed'
              : ''
    parts.push(`text-decoration:${lines.join(' ')}${shape}`)
  }
  const underlineColor = colorCSS(style.underlineColor, palette)
  if (underlineColor !== null) parts.push(`text-decoration-color:${underlineColor}`)
  return { css: parts.join(';'), hasBackground: bg !== palette.defaultBg }
}

/** Convert one accepted backend style to the frozen-cell attribute shape.
 *  The live painter and pinned-screen capture share this owner, so color and
 *  attribute mapping cannot drift between them. */
export function cellAttrsOf(style: Style, palette: TerminalSnapshot): CellAttrs {
  const attributes = style.attributes
  return {
    fg: foregroundCSS(style, palette),
    bg: colorCSS(style.background, palette),
    bold: (attributes & BOLD) !== 0,
    italic: (attributes & ITALIC) !== 0,
    dim: (attributes & FAINT) !== 0,
    underline: style.underline !== 0,
    inverse: (attributes & INVERSE) !== 0,
    blink: (attributes & (1 << 3)) !== 0,
    strikethrough: (attributes & STRIKETHROUGH) !== 0,
    overline: (attributes & OVERLINE) !== 0,
  }
}
