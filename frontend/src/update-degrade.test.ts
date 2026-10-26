// @vitest-environment node
/**
 * update-degrade — when a failed update-state read is worth a person's
 * attention (review, nocx-1ebrs.66.1.1).
 *
 * The case that made this a module: the web and e2e builds have no updater, so
 * every launch rejected the read and warned, and two e2e specs then found two
 * toast messages where they expected one. The case that must survive: the
 * installed app failing a real read, which leaves the offer invisible and is
 * exactly the soft degrade AGENTS.md says must be visible.
 */
import { describe, expect, it } from 'vitest'
import { shouldAnnounceUpdateStateFailure } from './update-degrade'

describe('shouldAnnounceUpdateStateFailure', () => {
  it('says nothing in an embedding with no updater, whatever the error says', () => {
    for (const reason of [
      'the updater is not available in this window',
      'socket closed',
      'method not found: GetUpdateState',
      '',
    ]) {
      expect(shouldAnnounceUpdateStateFailure(reason, 'web')).toBe(false)
    }
  })

  it('says nothing in a desktop build whose own guard reports no updater', () => {
    expect(
      shouldAnnounceUpdateStateFailure('the updater is not available in this window', 'darwin'),
    ).toBe(false)
    expect(shouldAnnounceUpdateStateFailure('method not found: GetUpdateState', 'linux')).toBe(
      false,
    )
  })

  it('tells the person when the installed app cannot read a state it should be able to', () => {
    for (const platform of ['darwin', 'linux', 'windows'] as const) {
      expect(shouldAnnounceUpdateStateFailure('socket closed', platform)).toBe(true)
      expect(shouldAnnounceUpdateStateFailure('transport refused: EOF', platform)).toBe(true)
    }
  })
})
