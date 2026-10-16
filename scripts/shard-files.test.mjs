import test from 'node:test'
import assert from 'node:assert/strict'
import { planShards } from '../e2e/shard-files.mjs'

test('duration-balanced shard plan assigns every measured spec exactly once', () => {
  const project = 'chromium'
  const files = ['e2e/a.spec.ts', 'e2e/b.spec.ts', 'e2e/c.spec.ts', 'e2e/d.spec.ts']
  const seconds = {
    'e2e/a.spec.ts': 100,
    'e2e/b.spec.ts': 80,
    'e2e/c.spec.ts': 20,
    'e2e/d.spec.ts': 10,
  }
  const plan = planShards(project, files, seconds)

  assert.deepEqual(plan.shards.flatMap((shard) => shard.files).sort(), [...files].sort())
  assert.equal(new Set(plan.shards.flatMap((shard) => shard.files.flat())).size, files.length)
  assert.ok(Math.abs(plan.shards[0].seconds - plan.shards[1].seconds) <= 20)
})

test('duration-balanced shard plan is deterministic and rejects missing measurements', () => {
  const files = ['e2e/a.spec.ts', 'e2e/b.spec.ts', 'e2e/c.spec.ts']
  const seconds = { 'e2e/a.spec.ts': 15, 'e2e/b.spec.ts': 10, 'e2e/c.spec.ts': 5 }
  assert.deepEqual(planShards('chromium', files, seconds), planShards('chromium', files, seconds))
  assert.throws(() => planShards('chromium', files, { 'e2e/a.spec.ts': 15 }), /missing duration/)
})
