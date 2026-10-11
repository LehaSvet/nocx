// Command-existence snapshot state. The backend parses and authenticates the
// shell-local snapshot, then delivers names as a typed session effect. The
// renderer validates that shape and combines it with the target's shared PATH
// names returned by shell.commandNames.

/** Upper bound on the number of shell-local names accepted from the backend. */
export const MAX_SNAPSHOT_NAMES = 4096

/** Upper bound on the combined character count accepted from the backend. */
export const MAX_SNAPSHOT_CHARS = 65536

/** Upper bound on the shared PATH names returned by shell.commandNames. */
export const MAX_SHARED_NAMES = 8192

/** Validate the backend-owned shell-name snapshot at the IPC boundary. */
export function isValidCommandSnapshotNames(value: unknown): value is [string, ...string[]] {
  if (!Array.isArray(value) || value.length === 0 || value.length > MAX_SNAPSHOT_NAMES) return false
  let total = 0
  for (const name of value) {
    if (typeof name !== 'string' || name.length === 0) return false
    total += name.length
    if (total > MAX_SNAPSHOT_CHARS) return false
  }
  return true
}

/**
 * What the product can honestly say about the shared (PATH) half of command
 * discovery. `running` is the interval between asking and receiving the
 * backend answer; the backend never sends it.
 */
export type CommandNamesState = 'running' | 'ready' | 'stale' | 'timed-out' | 'failed'

/** The shared half as the renderer holds it. */
export interface SharedCommandNames {
  readonly state: CommandNamesState
  readonly names: string[]
  readonly ageMs: number
  readonly reason: string
  readonly truncated: boolean
}

/**
 * The per-tab store for session-local shell names and shared PATH names.
 * The backend authenticates shell snapshots before delivering a typed effect;
 * the client holds only the current name sets and their shared-query status.
 */
export class CommandSnapshotStore {
  private _names: Set<string> | null = null
  private _shared: Set<string> | null = null
  private _sharedState: CommandNamesState = 'running'
  private _sharedAgeMs = 0
  private _sharedReason = ''
  private _sharedTruncated = false
  private listeners = new Set<() => void>()

  applyBackendSnapshot(value: unknown): void {
    if (!isValidCommandSnapshotNames(value)) return
    this._names = new Set(value)
    for (const cb of this.listeners) cb()
  }

  /**
   * Apply the shared half — the answer to `shell.commandNames`.
   *
   * The two halves are kept apart because they are two different truths. The
   * session-local half is THIS shell's tables and may never be cached for
   * another session; the shared half is the target's PATH, identical for
   * every session to it. `has` and `matching` read the union; `status`
   * requires both, for the reason on `status` itself.
   */
  applySharedNames(result: SharedCommandNames): void {
    this._sharedState = result.state
    this._sharedAgeMs = result.ageMs
    this._sharedReason = result.reason
    this._sharedTruncated = result.truncated
    if (result.state === 'ready' || result.state === 'stale') {
      // Over-long is refused rather than truncated: a prefix of an
      // enumeration presented as the whole set marks real commands as
      // nonexistent, which is the failure the state machine exists to avoid.
      this._shared = result.names.length > MAX_SHARED_NAMES ? null : new Set(result.names)
    } else {
      // A scan that did not complete publishes nothing, and must not leave
      // the last one standing under a state that says otherwise.
      this._shared = null
    }
    for (const cb of this.listeners) cb()
  }

  /** The shared half's state, for the surface. */
  get commandNamesState(): CommandNamesState {
    return this._sharedState
  }

  /** How old the served shared snapshot is, in ms. Meaningful when stale. */
  get commandNamesAgeMs(): number {
    return this._sharedAgeMs
  }

  /** The backend's own words for a failure. Empty unless failed/timed-out. */
  get commandNamesReason(): string {
    return this._sharedReason
  }

  /** True when the shared set was cut at its bound. */
  get commandNamesTruncated(): boolean {
    return this._sharedTruncated
  }

  /**
   * Whether this store can be asked "does this command exist".
   *
   * BOTH halves are required, and that is the whole point of the getter. A
   * shell's own tables answer for aliases and functions; the PATH set answers
   * for everything else. With only one of them, every name the other half
   * owns would come back "does not exist" — the editor would strike through
   * `git` because the PATH scan had not landed yet. `unavailable` means
   * indeterminate, which is the honest verdict until both are in.
   *
   * Completion does not go through here: it offers what it has (`matching`)
   * and names the shared half's state in its own row, because offering a
   * partial list is useful where asserting a partial absence is a lie.
   */
  get status(): 'unavailable' | 'ready' {
    if (this._names === null || this._shared === null) return 'unavailable'
    return 'ready'
  }

  has(name: string): boolean {
    if (this._names !== null && this._names.has(name)) return true
    return this._shared !== null && this._shared.has(name)
  }

  /** Fires when a snapshot is applied (a discard notifies nobody). */
  subscribe(cb: () => void): () => void {
    this.listeners.add(cb)
    return () => {
      this.listeners.delete(cb)
    }
  }

  /**
   * The command names starting with `prefix`, sorted — the completion
   * provider's read seam (design §8.5: the command provider reads the
   * snapshot the running shell produced). A fresh array every call: the
   * store never hands out its internal set, so no consumer can corrupt the
   * snapshot for the rest of the session. Empty when no snapshot has
   * applied — "unavailable" and "no commands" must not look alike, so the
   * caller checks `status` first.
   */
  matching(prefix: string): string[] {
    if (this._names === null && this._shared === null) return []
    const out = new Set<string>()
    for (const n of this._names ?? []) if (n.startsWith(prefix)) out.add(n)
    for (const n of this._shared ?? []) if (n.startsWith(prefix)) out.add(n)
    return [...out].sort()
  }

  /** Whether either half has anything to offer — the completion provider's
   *  question, which is not `status`'s. */
  get hasAnyNames(): boolean {
    return this._names !== null || this._shared !== null
  }

  reset(): void {
    this._names = null
    this._shared = null
    this._sharedState = 'running'
    this._sharedAgeMs = 0
    this._sharedReason = ''
    this._sharedTruncated = false
  }
}
