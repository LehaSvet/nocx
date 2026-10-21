// @vitest-environment jsdom
/**
 * update-check — what the last automatic check did (nocx-1ebrs.36).
 *
 * The bead: 0.6.0's updater panicked on every check and the startup catch
 * swallowed it, so no user was ever told anything — not about an available
 * update, and not about a check that failed. These assert the owner of that
 * outcome: a failure is recorded with its cause and logged at warn, a
 * successful check clears it, and an available update still reaches the
 * notice.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createUpdateCheck } from './update-check'
import type { UpdateNoticeController } from './update-notice'

const checkForUpdate = vi.fn<() => Promise<unknown>>()
const { logWarn } = vi.hoisted(() => ({ logWarn: vi.fn() }))

vi.mock('./log', () => ({
  log: { warn: logWarn, info: vi.fn(), error: vi.fn(), debug: vi.fn() },
}))

vi.mock('../bindings/github.com/shady2k/nocx/wailsapp', () => ({
  CheckForUpdate: () => checkForUpdate(),
}))

function fakeNotice() {
  return {
    showAvailable: vi.fn(),
    showDownloading: vi.fn(),
    showPendingRestart: vi.fn(),
    showError: vi.fn(),
  } satisfies UpdateNoticeController
}

beforeEach(() => {
  checkForUpdate.mockReset()
  logWarn.mockClear()
})

describe('createUpdateCheck', () => {
  it('records the reason when the check fails, and logs it at warn', async () => {
    checkForUpdate.mockRejectedValue(new Error('fetch manifest: unexpected status 500'))
    const notice = fakeNotice()
    const check = createUpdateCheck(notice)

    await check.run()

    expect(check.error()).toBe('fetch manifest: unexpected status 500')
    expect(logWarn).toHaveBeenCalledWith(
      'update check failed',
      expect.objectContaining({ reason: 'fetch manifest: unexpected status 500' }),
    )
    expect(notice.showAvailable).not.toHaveBeenCalled()
  })

  it('reports an available update through the notice and clears a stale failure', async () => {
    const notice = fakeNotice()
    const check = createUpdateCheck(notice)

    // A first, failing check leaves its reason behind.
    checkForUpdate.mockRejectedValueOnce(new Error('dns is down'))
    await check.run()
    expect(check.error()).toBe('dns is down')

    // The next successful check reports the update and clears it.
    checkForUpdate.mockResolvedValue({ Version: '0.6.1', NotesURL: 'https://example.com/n' })
    await check.run()

    expect(notice.showAvailable).toHaveBeenCalledWith('0.6.1', 'https://example.com/n')
    expect(check.error()).toBeNull()
  })

  it('treats a check that found nothing as a success, not a failure', async () => {
    checkForUpdate.mockResolvedValue(null)
    const notice = fakeNotice()
    const check = createUpdateCheck(notice)

    await check.run()

    expect(check.error()).toBeNull()
    expect(notice.showAvailable).not.toHaveBeenCalled()
  })
})

describe('createUpdateCheck with a channel bridge (nocx-1ebrs.35.1)', () => {
  it('applies the stored channel before checking, so the check fetches what the store names', async () => {
    checkForUpdate.mockResolvedValue(null)
    const apply = vi.fn<(channel: string) => Promise<void>>().mockResolvedValue(undefined)
    const notice = fakeNotice()
    const check = createUpdateCheck(notice, {
      resolve: () => Promise.resolve('nightly'),
      apply,
    })

    await check.run()

    expect(apply).toHaveBeenCalledWith('nightly')
    expect(checkForUpdate).toHaveBeenCalled()
    expect(check.error()).toBeNull()
  })

  it('surfaces a channel that could not be applied instead of checking the wrong manifest', async () => {
    const apply = vi
      .fn<(channel: string) => Promise<void>>()
      .mockRejectedValue(new Error('the updater is not available in this window'))
    const notice = fakeNotice()
    const check = createUpdateCheck(notice, {
      resolve: () => Promise.resolve('nightly'),
      apply,
    })

    await check.run()

    // Not checked at all: a check against the previous channel would make
    // the stored setting decorative — the exact failure the bead names.
    expect(checkForUpdate).not.toHaveBeenCalled()
    expect(check.error()).toBe('update channel: the updater is not available in this window')
    expect(logWarn).toHaveBeenCalled()
    expect(notice.showAvailable).not.toHaveBeenCalled()
  })

  it('re-resolves the channel on every run, so a switch reaches the next check', async () => {
    checkForUpdate.mockResolvedValue(null)
    const apply = vi.fn<(channel: string) => Promise<void>>().mockResolvedValue(undefined)
    const current = 'stable'
    const values = ['stable', 'nightly', 'nightly']
    let i = 0
    const check = createUpdateCheck(fakeNotice(), {
      resolve: () => {
        const v = values[Math.min(i, values.length - 1)]
        i++
        return Promise.resolve(v)
      },
      apply,
    })

    await check.run()
    await check.run()
    await check.run()

    expect(apply).toHaveBeenNthCalledWith(1, 'stable')
    expect(apply).toHaveBeenNthCalledWith(2, 'nightly')
    expect(apply).toHaveBeenNthCalledWith(3, 'nightly')
    expect(current).toBe('stable')
  })
})
