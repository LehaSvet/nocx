/**
 * GENERATED FILE — do not edit.
 *
 * Source: contracts/session.recoveryChanged.schema.json
 * Regenerate: cd frontend && npm run contracts
 *
 * Editing this file is editing the wrong end of the contract. If the renderer
 * needs a field the wire does not carry, the schema is what has to change, and
 * then the Go transport has to satisfy it.
 */

/**
 * Params for the session.recoveryChanged notification: the recording's own answer to session.recoveryStatus, pushed when that answer changes. It is metadata only — the recorded stream's end offset and the ranges the recording does not hold — and carries no recorded bytes. A reclaim reads session.recoveryStatus once, before it attaches, while the recording is still catching up on the replay; this notification is how the number a person is shown keeps being the ledger's number rather than the one the reclaim happened to catch.
 */
export interface SessionRecoveryChanged {
  /**
   * The server-minted session id whose recording changed.
   */
  sessionId: string
  /**
   * End offset recorded for this session, as session.recoveryStatus reports it at the same instant.
   */
  produced: number
  /**
   * The ranges this session's recording does not hold, in stream order and disjoint, exactly as session.recoveryStatus answers them. Always an array; contains metadata only.
   */
  gaps: Gap[]
}
export interface Gap {
  /**
   * First dropped byte offset.
   */
  start: number
  /**
   * Offset just past the last dropped byte.
   */
  end: number
  /**
   * Why the range is missing.
   */
  reason: string
}
