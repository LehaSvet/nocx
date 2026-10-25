/**
 * GENERATED FILE — do not edit.
 *
 * Source: contracts/session.publishFailed.schema.json
 * Regenerate: cd frontend && npm run contracts
 *
 * Editing this file is editing the wrong end of the contract. If the renderer
 * needs a field the wire does not carry, the schema is what has to change, and
 * then the Go transport has to satisfy it.
 */

/**
 * Params of the session.publishFailed server-to-client notification: this session's shell-integration bundle was not copied to the host it runs on (nocx-1kejz.4). The publish happens before the spawn that creates a session, and its failure is deliberately NOT a refusal — the far side keeps any generation it already had, byte-identical, so the pane opens as an ordinary terminal (integration-delivery-carrier design §6.1 step 5, ADR-0004) — which is why this is a fact about delivery and not a status on the integration axis: a session whose copy did not land may still integrate from an older generation, and what the axis will answer is unknown when this is emitted. It is sent at most once per session, only when the copy failed, and it is retained and replayed to a subscriber that attaches later, because the fact settles before a pane can exist to hear it. A publish that landed says nothing at all: absence is how 'there is nothing to tell you' is expressed, and a notification that fired on success would make every healthy ssh pane speak. The raw error never travels: a coordinator-side publish failure is an SFTP status code and a path — meaningful in a log, meaningless in a pane (nocx-viil) — so the reason is a closed set the renderer has words for, and the chain stays in the backend log.
 */
export interface SessionPublishFailed {
  /**
   * The session whose bundle was not delivered. Server-authoritative (AD-7), the same id the open ack returned; the renderer attaches the notice to the pane that owns this id and filters nothing.
   */
  sessionId: string
  /**
   * The backend instance that minted the session (AD-7 — server-authoritative, never minted by the renderer). Compared against the pair the open ack carried, so a fact about this sessionId out of a previous backend instance is refused instead of drawn into the current pane.
   */
  instanceId: string
  /**
   * The session's epoch within its backend instance, as minted at open: distinguishes a later session that reuses a sessionId from the incarnation this failure belongs to. Same vocabulary as the open ack's sessionEpoch.
   */
  sessionEpoch: number
  /**
   * Why the copy did not land, from a closed set owned by internal/shellintegration (PublishFailureReason) — the step that failed names it, never a reader of the error's text. connection-failed: the connection the copy rides did not come up or ended during it, and nothing says the far side refused anything. home-unavailable: the far side did not report a home directory for the account, so nocx had nowhere to write. file-transfer-unavailable: the far side would not open, or would not speak, the file-transfer channel the copy needs — a server with that subsystem switched off, or an intermediary that permits interactive channels only. write-refused: the copy was attempted and the far side would not take it. unknown: the copy failed and this build cannot say why, which is a real answer rather than a synonym for success. The renderer has one sentence per member, so a member added here with no words is a type error rather than a blank card.
   */
  reason:
    | 'connection-failed'
    | 'home-unavailable'
    | 'file-transfer-unavailable'
    | 'write-refused'
    | 'unknown'
}
