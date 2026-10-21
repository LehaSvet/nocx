/**
 * GENERATED FILE — do not edit.
 *
 * Source: contracts/connections.passwordRequest.schema.json
 * Regenerate: cd frontend && npm run contracts
 *
 * Editing this file is editing the wrong end of the contract. If the renderer
 * needs a field the wire does not carry, the schema is what has to change, and
 * then the Go transport has to satisfy it.
 */

/**
 * Params of the connections.passwordRequest server-to-client notification. The backend asks the renderer for a connection password, naming which connection and account it is asking about (nocx-s8jn — every password prompt must say which password it is asking for); the renderer turns this into the connection-password prompt and answers via connections.passwordResolved.
 */
export interface ConnectionsPasswordRequest {
  /**
   * Server-assigned opaque id that the connections.passwordResolved response echoes back.
   */
  requestId: string
  /**
   * The saved profile's display name the prompt is asking for. Empty for a DIRECT-HOST open (nocx-1ebrs.44): the connect UI's unsaved entry names no saved connection, there is nothing for a remember to bind to, and the renderer shows no remember checkbox for an ask that carries no connection name.
   */
  connection: string
  /**
   * The account on the host the password belongs to. The same host with a different user is a different password.
   */
  user: string
  /**
   * The resolved host the password will authenticate against.
   */
  host: string
  /**
   * Why the password is being asked: no password is stored for the connection, or the stored one was rejected. A human-readable sentence, never a code.
   */
  reason: string
}
