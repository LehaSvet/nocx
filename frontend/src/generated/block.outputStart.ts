/**
 * GENERATED FILE — do not edit.
 *
 * Source: contracts/block.outputStart.schema.json
 * Regenerate: cd frontend && npm run contracts
 *
 * Editing this file is editing the wrong end of the contract. If the renderer
 * needs a field the wire does not carry, the schema is what has to change, and
 * then the Go transport has to satisfy it.
 */

/**
 * Params of the session-scoped block.outputStart notification. The row is the anchor's screen row, not the ordered output rows stream coordinate.
 */
export interface BlockOutputStart {
  entryId: string
  row: number
  screenRevision: number
  bufferKind: 'normal' | 'alternate'
  bufferInstance: string
  cols: number
  rows: number
}
