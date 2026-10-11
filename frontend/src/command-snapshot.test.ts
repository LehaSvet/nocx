// @vitest-environment jsdom
// Command-existence state as delivered by authenticated backend effects and
// shared shell.commandNames results.
import { describe, it, expect, beforeEach } from 'vitest'
import {
  CommandSnapshotStore,
  MAX_SNAPSHOT_NAMES,
  MAX_SNAPSHOT_CHARS,
  MAX_SHARED_NAMES,
} from './command-snapshot'

function applyShared(store: CommandSnapshotStore, names: string[], state = 'ready' as const): void {
  store.applySharedNames({ state, names, ageMs: 0, reason: '', truncated: false })
}

describe('CommandSnapshotStore', () => {
  let store: CommandSnapshotStore

  beforeEach(() => {
    store = new CommandSnapshotStore()
  })

  it('starts unavailable and does not claim unknown commands exist', () => {
    expect(store.status).toBe('unavailable')
    expect(store.has('pwd')).toBe(false)
    expect(store.matching('p')).toEqual([])
  })

  it('combines authenticated session names with the shared PATH result', () => {
    store.applyBackendSnapshot(['alias', 'git'])
    expect(store.status).toBe('unavailable')
    applyShared(store, ['git', 'ls'])
    expect(store.status).toBe('ready')
    expect(store.has('alias')).toBe(true)
    expect(store.has('git')).toBe(true)
    expect(store.has('missing')).toBe(false)
    expect(store.matching('g')).toEqual(['git'])
  })

  it('refuses invalid backend snapshots without clearing the current names', () => {
    store.applyBackendSnapshot(['alias'])
    store.applyBackendSnapshot([])
    store.applyBackendSnapshot([''])
    store.applyBackendSnapshot(Array.from({ length: MAX_SNAPSHOT_NAMES + 1 }, (_, i) => `n${i}`))
    store.applyBackendSnapshot(['x'.repeat(MAX_SNAPSHOT_CHARS + 1)])
    expect(store.has('alias')).toBe(true)
    expect(store.has('')).toBe(false)
  })

  it('replaces the session-local set and notifies subscribers for valid snapshots', () => {
    const seen: string[][] = []
    store.applyBackendSnapshot(['pwd'])
    const unsubscribe = store.subscribe(() => seen.push(store.matching('')))
    store.applyBackendSnapshot(['ls'])
    expect(seen).toEqual([['ls']])
    expect(store.has('pwd')).toBe(false)
    expect(store.has('ls')).toBe(true)
    unsubscribe()
    store.applyBackendSnapshot(['cat'])
    expect(seen).toEqual([['ls']])
  })

  it('keeps session-local names isolated between panes', () => {
    const other = new CommandSnapshotStore()
    store.applyBackendSnapshot(['myalias'])
    other.applyBackendSnapshot(['kubectl'])
    applyShared(store, ['git'])
    applyShared(other, ['git'])
    expect(store.has('myalias')).toBe(true)
    expect(store.has('kubectl')).toBe(false)
    expect(other.has('kubectl')).toBe(true)
    expect(other.has('myalias')).toBe(false)
  })

  it('requires both halves before declaring a command missing', () => {
    store.applyBackendSnapshot(['myalias'])
    expect(store.status).toBe('unavailable')
    applyShared(store, ['git', 'ls'])
    expect(store.status).toBe('ready')
    expect(store.has('git')).toBe(true)
    expect(store.has('myalias')).toBe(true)
    expect(store.has('missing')).toBe(false)
  })

  it('completion offers the union of whichever name sets have arrived', () => {
    store.applyBackendSnapshot(['gitalias'])
    expect(store.matching('git')).toEqual(['gitalias'])
    applyShared(store, ['git', 'gitk'])
    expect(store.matching('git')).toEqual(['git', 'gitalias', 'gitk'])
  })

  it('a shared half that did not complete publishes no partial names', () => {
    store.applyBackendSnapshot(['myalias'])
    applyShared(store, ['git'])
    store.applySharedNames({
      state: 'timed-out',
      names: ['partial'],
      ageMs: 0,
      reason: 'the scan did not finish',
      truncated: false,
    })
    expect(store.commandNamesState).toBe('timed-out')
    expect(store.commandNamesReason).toBe('the scan did not finish')
    expect(store.has('partial')).toBe(false)
    expect(store.has('git')).toBe(false)
    expect(store.status).toBe('unavailable')
  })

  it('serves a stale shared half with its age', () => {
    store.applyBackendSnapshot(['myalias'])
    store.applySharedNames({
      state: 'stale',
      names: ['git'],
      ageMs: 90_000,
      reason: '',
      truncated: false,
    })
    expect(store.commandNamesState).toBe('stale')
    expect(store.commandNamesAgeMs).toBe(90_000)
    expect(store.has('git')).toBe(true)
    expect(store.status).toBe('ready')
  })

  it('refuses an over-long shared set instead of serving a misleading prefix', () => {
    applyShared(
      store,
      Array.from({ length: MAX_SHARED_NAMES + 1 }, (_, i) => `cmd${i}`),
    )
    expect(store.has('cmd0')).toBe(false)
    expect(store.status).toBe('unavailable')
  })

  it('starts shared discovery as running and resets both halves', () => {
    expect(store.commandNamesState).toBe('running')
    expect(store.commandNamesAgeMs).toBe(0)
    expect(store.commandNamesReason).toBe('')
    store.applyBackendSnapshot(['pwd'])
    applyShared(store, ['git'])
    store.reset()
    expect(store.status).toBe('unavailable')
    expect(store.has('pwd')).toBe(false)
    expect(store.has('git')).toBe(false)
    expect(store.commandNamesState).toBe('running')
  })
})
