// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { applyTerminalFont } from './terminal-font'
import { showToast } from './ui/toast'

vi.mock('./log', () => ({ log: { warn: vi.fn() } }))
vi.mock('./ui/toast', () => ({ showToast: vi.fn() }))

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('terminal font selection', () => {
  it('applies a loaded bundled font and restores the theme default on reset', async () => {
    const root = document.createElement('div')
    Object.defineProperty(document, 'fonts', {
      configurable: true,
      value: { load: vi.fn().mockResolvedValue([{}]) },
    })
    await applyTerminalFont('nordwand-mono', root)
    expect(root.dataset.terminalFont).toBe('nordwand-mono')
    await applyTerminalFont('system', root)
    expect(root.dataset.terminalFont).toBeUndefined()
  })

  it('keeps the latest selection when a bundled face finishes loading late', async () => {
    // The project's TypeScript lib predates Promise.withResolvers.
    let finish!: () => void
    const loading = new Promise<void>((resolve) => {
      finish = resolve
    })
    const root = document.createElement('div')
    const load = vi.fn(() => loading)
    Object.defineProperty(document, 'fonts', { configurable: true, value: { load } })
    const pending = applyTerminalFont('nordwand-mono', root)
    await applyTerminalFont('system', root)
    finish()
    await pending
    expect(root.dataset.terminalFont).toBeUndefined()
  })

  it('leaves the current font in place and reports a bundled font load failure', async () => {
    const root = document.createElement('div')
    Object.defineProperty(document, 'fonts', {
      configurable: true,
      value: { load: vi.fn().mockRejectedValue(new Error('font unavailable')) },
    })
    await applyTerminalFont('nordwand-mono', root)
    expect(root.dataset.terminalFont).toBeUndefined()
    expect(showToast).toHaveBeenCalledWith(expect.objectContaining({ level: 'danger' }))
  })
})
