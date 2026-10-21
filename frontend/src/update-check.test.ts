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
