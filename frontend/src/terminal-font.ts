import { log } from './log'
import { showToast } from './ui/toast'

export const TERMINAL_FONT_KEY = 'terminal.fontFamily'
let revision = 0
const subscribers = new Set<() => void>()

/** A font can change its ink metrics without changing the probe's size. */
export function subscribeTerminalFontChanges(fn: () => void): () => void {
  subscribers.add(fn)
  return () => {
    subscribers.delete(fn)
  }
}

function notifyFontChanged(): void {
  for (const subscriber of subscribers) subscriber()
}

/** Load all terminal faces before changing the shared font. A later choice
 * wins even when an earlier font request is still in flight. */
export async function applyTerminalFont(
  value: unknown,
  root: HTMLElement = document.documentElement,
): Promise<void> {
  const request = ++revision
  if (value !== 'nordwand-mono') {
    if (root.dataset.terminalFont === undefined) return
    delete root.dataset.terminalFont
    notifyFontChanged()
    return
  }
  if (root.dataset.terminalFont === value) return
  try {
    const faces = await Promise.all([
      document.fonts.load('400 14px "Nordwand Mono"'),
      document.fonts.load('italic 400 14px "Nordwand Mono"'),
      document.fonts.load('700 14px "Nordwand Mono"'),
      document.fonts.load('italic 700 14px "Nordwand Mono"'),
    ])
    if (request !== revision) return
    if (faces.some((loaded) => loaded.length === 0)) throw new Error('Missing bundled font face')
    root.dataset.terminalFont = value
    notifyFontChanged()
  } catch (error) {
    if (request !== revision) return
    log.warn('Nordwand Mono could not be loaded', { error: String(error) })
    showToast({
      level: 'danger',
      message: 'Nordwand Mono could not be loaded. The current font is unchanged.',
    })
  }
}
