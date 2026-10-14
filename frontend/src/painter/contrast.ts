// The readability floor the painter applies to every run (nocx-zg3k3.27).
//
// 0.4's xterm ran with minimumContrastRatio 4.5 (nocx-3lrm): a foreground
// that falls below WCAG AA against the background it actually sits on is
// stepped in luminance, away from that background, until it holds. Removing
// xterm removed the floor, and the programs that paint ANSI colours as large
// backgrounds became partly unreadable — htop's green-on-green tab brackets
// vanished outright. This is xterm's own algorithm (common/Color.ts,
// ensureContrastRatio), so the painter renders what 0.4 rendered. Only pairs
// below the floor change; a palette that is already legible renders exactly
// as declared.
//
// The floor is 3:1, not 0.4's 4.5 (the owner's report, 2026-10-15). The step moves a colour
// toward white or black, so at 4.5 it lifted mc's marked names (bright yellow,
// 2.79:1 on Graphite's blue) and its other names (bright white, 4.07:1) to
// the same near-white, and nobody could see which files were marked. 3:1
// still rescues what the floor was for — grey on Tokyo Night's light blue
// (1.19:1, nocx-3lrm) and htop's green on green (1:1) — and leaves a colour
// that is merely not AA its hue.

const MINIMUM_CONTRAST_RATIO = 3

type RGB = readonly [number, number, number]

function parse(css: string): RGB | null {
  const hex = /^#([0-9a-f]{6})$/i.exec(css.trim())
  if (hex) {
    const n = parseInt(hex[1], 16)
    return [(n >> 16) & 0xff, (n >> 8) & 0xff, n & 0xff]
  }
  const fn = /^rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*(?:,\s*[\d.]+\s*)?\)$/i.exec(css.trim())
  if (fn) return [Number(fn[1]), Number(fn[2]), Number(fn[3])]
  return null
}

function channel(c: number): number {
  const s = c / 255
  return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
}

function luminance([r, g, b]: RGB): number {
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)
}

function ratio(l1: number, l2: number): number {
  return l1 < l2 ? (l2 + 0.05) / (l1 + 0.05) : (l1 + 0.05) / (l2 + 0.05)
}

function reduce(bgL: number, fg: RGB, target: number): RGB {
  let [r, g, b] = fg
  let cr = ratio(luminance([r, g, b]), bgL)
  while (cr < target && (r > 0 || g > 0 || b > 0)) {
    r -= Math.max(0, Math.ceil(r * 0.1))
    g -= Math.max(0, Math.ceil(g * 0.1))
    b -= Math.max(0, Math.ceil(b * 0.1))
    cr = ratio(luminance([r, g, b]), bgL)
  }
  return [r, g, b]
}

function increase(bgL: number, fg: RGB, target: number): RGB {
  let [r, g, b] = fg
  let cr = ratio(luminance([r, g, b]), bgL)
  while (cr < target && (r < 0xff || g < 0xff || b < 0xff)) {
    r = Math.min(0xff, r + Math.ceil((255 - r) * 0.1))
    g = Math.min(0xff, g + Math.ceil((255 - g) * 0.1))
    b = Math.min(0xff, b + Math.ceil((255 - b) * 0.1))
    cr = ratio(luminance([r, g, b]), bgL)
  }
  return [r, g, b]
}

/** The foreground to paint over `bg`: `fg` itself when it already meets the
 *  floor or either colour is not one this can read, otherwise the nearest
 *  colour along xterm's luminance steps that does. */
export function legibleForeground(fg: string, bg: string, target = MINIMUM_CONTRAST_RATIO): string {
  const f = parse(fg)
  const b = parse(bg)
  if (!f || !b) return fg
  const bgL = luminance(b)
  const fgL = luminance(f)
  if (ratio(fgL, bgL) >= target) return fg
  const [first, second] = fgL < bgL ? [reduce, increase] : [increase, reduce]
  let best = first(bgL, f, target)
  let bestRatio = ratio(luminance(best), bgL)
  if (bestRatio < target) {
    const other = second(bgL, f, target)
    const otherRatio = ratio(luminance(other), bgL)
    if (otherRatio > bestRatio) {
      best = other
      bestRatio = otherRatio
    }
  }
  return `rgb(${best[0]}, ${best[1]}, ${best[2]})`
}
