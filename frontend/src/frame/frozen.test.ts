// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { SERIALIZER_VERSION } from '../scrollback/serializer'
import { mintFrozenFrame, frozenFrameSourceFromBlock } from './frozen'
import type { CapturedFrame, FrameRow } from './types'

function rowText(frame: CapturedFrame): string[] {
  return frame.rows.map((row: FrameRow) =>
    row.kind === 'text'
      ? row.text
      : row.cells
          .map((cell) => cell.char)
          .join('')
          .trimEnd(),
  )
}

describe('frozen frame source', () => {
  it('mints text rows with source=frozen and the serializer version', () => {
    const frame = mintFrozenFrame({
      text: () => 'row one\nrow two',
      serializerVersion: SERIALIZER_VERSION,
    })

    expect(rowText(frame)).toEqual(['row one', 'row two'])
    expect(frame.cursor).toBeNull()
    expect(frame.provenance).toEqual({
      source: 'frozen',
      serializerVersion: SERIALIZER_VERSION,
      transforms: 'wrapped lines joined; leading/trailing blanks dropped',
      closed: true,
    })
  })

  it('uses the block DOM and blockOutputText as the frozen source', () => {
    const el = document.createElement('div')
    el.className = 'cmd-block'
    el.innerHTML =
      '<div class="cmd-output"><span class="term-line">first</span><span class="term-line">second</span></div>'

    const frame = mintFrozenFrame(frozenFrameSourceFromBlock(el))
    expect(frame.provenance.source).toBe('frozen')
    if (frame.provenance.source === 'frozen') {
      expect(frame.provenance.serializerVersion).toBe(SERIALIZER_VERSION)
    }
  })

  it('returns an empty frozen frame when the block has no output', () => {
    const frame = mintFrozenFrame(frozenFrameSourceFromBlock(null))
    expect(rowText(frame)).toEqual([])
    expect(frame.provenance.source).toBe('frozen')
  })
})
