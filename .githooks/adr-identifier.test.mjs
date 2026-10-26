/**
 * A decision record's identifier is DRAWN, not counted (nocx-hyh2p).
 *
 * The property that matters is the one a central counter cannot have: two
 * branches that both add a record to the same base must not be able to choose
 * the same identifier, because neither of them consulted anything the other
 * could see. So the first test draws from two INDEPENDENT processes rather than
 * twice from one — a module-scope counter seeded at import passes two
 * sequential calls on one instance and fails here on the very first identifier.
 *
 * The second test is what stops a counter dressed up as randomness: a sequence
 * is monotonic, and half of any random pair is not.
 */
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { test } from 'node:test'

const generator = new URL('./adr-identifier.mjs', import.meta.url).href

/** One process, its own draw, no shared state with any other call. */
function drawInAChild(count) {
  const code = `
    import { drawIdentifier } from ${JSON.stringify(generator)};
    const out = [];
    for (let i = 0; i < ${count}; i += 1) out.push(drawIdentifier());
    process.stdout.write(out.join('\\n'));
  `
  const stdout = execFileSync(process.execPath, ['--input-type=module', '-e', code], {
    encoding: 'utf8',
  })
  return stdout.trim().split('\n')
}

test('two independent draws from the same base never collide', () => {
  const left = drawInAChild(500)
  const right = drawInAChild(500)

  assert.equal(left.length, 500)
  assert.equal(right.length, 500)
  assert.equal(new Set([...left, ...right]).size, 1000)
})

test('two module instances draw independently of each other', async () => {
  const first = await import(generator)
  const second = await import(`${generator}?instance=${process.pid}`)

  const drawn = []
  for (let i = 0; i < 200; i += 1) {
    drawn.push(first.drawIdentifier(), second.drawIdentifier())
  }

  assert.equal(new Set(drawn).size, drawn.length)
})

test('a draw does not consult a counter', async () => {
  const { drawIdentifier } = await import(generator)
  const draws = Array.from({ length: 200 }, () => drawIdentifier())

  assert.equal(new Set(draws).size, draws.length, 'a draw repeated inside one process')

  const inversions = draws.slice(1).filter((id, i) => id < draws[i]).length
  assert.ok(
    inversions > draws.length / 4,
    `expected about half the successive draws to descend; saw ${inversions} of ${draws.length - 1}`,
  )
})

test('a draw that comes out all digits is never used as an identifier', async () => {
  const { drawIdentifier } = await import(generator)

  // The first eight symbols spell `01234567`, which is a number and could be
  // read as a legacy identifier; the draw must discard it and take the next.
  const queue = [...'01234567', ...'k3f9a2bc']
  assert.equal(
    drawIdentifier(() => queue.shift()),
    'k3f9a2bc',
  )

  const draws = Array.from({ length: 2000 }, () => drawIdentifier())
  assert.deepEqual(
    draws.filter((id) => /^\d+$/.test(id)),
    [],
    'an all-digit draw reached a caller',
  )
})

test('an identifier is eight Crockford base32 symbols, lower case', async () => {
  const { drawIdentifier, IDENTIFIER_PATTERN } = await import(generator)

  for (let i = 0; i < 50; i += 1) {
    const id = drawIdentifier()
    assert.match(id, IDENTIFIER_PATTERN, `${id} is not an identifier`)
    assert.equal(id.length, 8)
    assert.doesNotMatch(id, /[ilou]/, `${id} carries a symbol the alphabet excludes`)
  }
})
