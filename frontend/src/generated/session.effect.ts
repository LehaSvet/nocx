/**
 * GENERATED FILE — do not edit.
 *
 * Source: contracts/session.effect.schema.json
 * Regenerate: cd frontend && npm run contracts
 *
 * Editing this file is editing the wrong end of the contract. If the renderer
 * needs a field the wire does not carry, the schema is what has to change, and
 * then the Go transport has to satisfy it.
 */

/**
 * One identity-bearing non-visual update emitted by the session runtime. A full session.frame never carries or replays effects; the client deduplicates events by generation and effectId. commandSnapshot is current-state data and is re-issued with a fresh identity for each attachment.
 */
export type SessionEffect = {
  [k: string]: unknown
} & {
  sessionId: string
  generation: string
  effectId: string
  kind:
    | 'bell'
    | 'notification'
    | 'clipboard'
    | 'title'
    | 'cwd'
    | 'promptBoundary'
    | 'recovery'
    | 'commandSnapshot'
  title: string
  body: string
  episodeId?: string
  /**
   * For promptBoundary only, the lower-bound full-frame revision that must be offered first. It is correlation, not acknowledgement or authority.
   */
  screenRevision?: number
  /**
   * The validated shell-local command names, present only for kind commandSnapshot.
   *
   * @minItems 1
   * @maxItems 4096
   */
  commandNames?: [string, ...string[]]
}
