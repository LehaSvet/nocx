// @vitest-environment node
/**
 * update-schedule — when the automatic check runs (nocx-1ebrs.66.1.1).
 *
 * The bead's decision: a release published while the window is open must be
 * noticed. Before this, the check ran at startup and every 24 hours, so an app
 * left open for days could sit through a whole release. These assert the rule
 * as a person experiences it — a check at startup, one on coming back to the
 * window if an hour has passed, and one every four hours — and the two
 * properties that make it safe: one check in flight, and a failure still
 * counting as the last check.
 *
 * No test here depends on real time: the clock, the wake signal and the timers
 * are all injected, which is the point of the module's shape.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  HOUR_MS,
  PERIOD_MS,
  startUpdateSchedule,
  type UpdateScheduleTimers,
} from './update-schedule'
import type { WakeSource } from './wake-source'

const { logWarn } = vi.hoisted(() => ({ logWarn: vi.fn() }))
vi.mock('./log', () => ({ log: { warn: logWarn, info: vi.fn(), error: vi.fn(), debug: vi.fn() } }))

interface FakeWake extends WakeSource {
  wake(): void
  listeners(): number
}

function fakeWake(): FakeWake {
  const handlers = new Set<() => void>()
  return {
    subscribe(onWake) {
      handlers.add(onWake)
      return () => {
        handlers.delete(onWake)
      }
    },
    wake() {
      for (const handler of [...handlers]) handler()
    },
    listeners: () => handlers.size,
  }
}

interface FakeTimers {
  api: UpdateScheduleTimers
  /** Fire every registered interval once, as a tick would. */
  tick(): void
  /** The intervals that are registered, in milliseconds. */
  intervals(): number[]
}

function fakeTimers(): FakeTimers {
  const registered = new Map<number, { handler: () => void; ms: number }>()
  let next = 1
  return {
    api: {
      setInterval(handler, ms) {
        const id = next++
        registered.set(id, { handler, ms })
        return id
      },
      clearInterval(handle) {
        registered.delete(handle as number)
      },
    },
    tick() {
      for (const { handler } of [...registered.values()]) handler()
    },
    intervals: () => [...registered.values()].map((entry) => entry.ms),
  }
}

function clock(start = 1_000_000) {
  let at = start
  return {
    now: () => at,
    advance(ms: number) {
      at += ms
    },
  }
}

/**
 * Let the schedule's own async work finish.
 *
 * A check runs inside one async closure: the promise it returns has to settle,
 * and the `finally` that clears the in-flight flag runs after that. One
 * microtask is not enough and three is the most the shape can need — draining
 * a fixed number keeps the test deterministic instead of racing it with a
 * timer, which is what "no test depends on timing" is about.
 */
async function flush(): Promise<void> {
  for (let hop = 0; hop < 6; hop++) await Promise.resolve()
}

let check: ReturnType<typeof vi.fn<() => Promise<unknown>>>

beforeEach(() => {
  check = vi.fn<() => Promise<unknown>>(() =>
    Promise.resolve({ kind: 'up-to-date', channel: 'stable' }),
  )
  logWarn.mockClear()
})

describe('startUpdateSchedule', () => {
  it('checks once at startup, before anything else happens', async () => {
    const time = clock()
    startUpdateSchedule({ check, wake: fakeWake(), timers: fakeTimers().api, now: time.now })
    await flush()

    expect(check).toHaveBeenCalledTimes(1)
  })

  it('does not check again when the window regains focus within the hour', async () => {
    const time = clock()
    const wake = fakeWake()
    startUpdateSchedule({ check, wake, timers: fakeTimers().api, now: time.now })
    await flush()

    // The person alt-tabs away and back ten minutes after the startup check.
    time.advance(10 * 60 * 1000)
    wake.wake()
    await flush()

    expect(check).toHaveBeenCalledTimes(1)
  })

  it('checks when the window comes back after an hour — a window open for days learns of a release', async () => {
    const time = clock()
    const wake = fakeWake()
    startUpdateSchedule({ check, wake, timers: fakeTimers().api, now: time.now })
    await flush()

    time.advance(70 * 60 * 1000)
    wake.wake()
    await flush()

    expect(check).toHaveBeenCalledTimes(2)
  })

  it('checks every four hours even when nobody touches the window', async () => {
    const time = clock()
    const timers = fakeTimers()
    startUpdateSchedule({ check, wake: fakeWake(), timers: timers.api, now: time.now })
    await flush()

    expect(timers.intervals()).toEqual([PERIOD_MS])

    // Four hours on: the tick is due, and the hourly floor is long satisfied.
    time.advance(PERIOD_MS)
    timers.tick()
    await flush()

    expect(check).toHaveBeenCalledTimes(2)
  })

  it('holds one check in flight: a focus during a slow check starts no second one', async () => {
    const time = clock()
    const wake = fakeWake()
    let finish: (() => void) | undefined
    const slow = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = () => resolve()
        }),
    )
    startUpdateSchedule({ check: slow, wake, timers: fakeTimers().api, now: time.now })
    await flush()
    expect(slow).toHaveBeenCalledTimes(1)

    // An hour later the window regains focus while the first check still runs.
    time.advance(2 * HOUR_MS)
    wake.wake()
    await flush()
    expect(slow).toHaveBeenCalledTimes(1)

    finish?.()
    await flush()

    // Once it is done, the next hour's wake is free to run again.
    time.advance(2 * HOUR_MS)
    wake.wake()
    await flush()
    expect(slow).toHaveBeenCalledTimes(2)
  })

  it('counts a failed check as the last check, and says the failure happened', async () => {
    const time = clock()
    const wake = fakeWake()
    const failing = vi.fn(() =>
      Promise.reject(new Error('manifest fetch exploded outside its own outcome owner')),
    )
    startUpdateSchedule({ check: failing, wake, timers: fakeTimers().api, now: time.now })
    await flush()

    expect(logWarn).toHaveBeenCalledWith(
      'update schedule: the check threw outside its outcome owner',
      expect.objectContaining({ reason: 'manifest fetch exploded outside its own outcome owner' }),
    )

    // A broken network must not be re-probed on every focus.
    time.advance(5 * 60 * 1000)
    wake.wake()
    await flush()
    expect(failing).toHaveBeenCalledTimes(1)
  })

  it('stops: no wake and no tick reaches the check afterwards', async () => {
    const time = clock()
    const wake = fakeWake()
    const timers = fakeTimers()
    const stop = startUpdateSchedule({ check, wake, timers: timers.api, now: time.now })
    await flush()
    expect(wake.listeners()).toBe(1)

    stop()

    expect(wake.listeners()).toBe(0)
    expect(timers.intervals()).toEqual([])
    time.advance(10 * HOUR_MS)
    wake.wake()
    timers.tick()
    await flush()
    expect(check).toHaveBeenCalledTimes(1)
  })

  it('states the owner’s two numbers where they cannot drift silently', () => {
    expect(HOUR_MS).toBe(60 * 60 * 1000)
    expect(PERIOD_MS).toBe(4 * HOUR_MS)
  })
})

describe('the composition root', () => {
  // The module above is the rule; this is the rule being USED. Without it the
  // window could keep the 24-hour interval that made the bead and every test
  // above would still pass — the failure AGENTS.md calls "a reachable read
  // path hiding an unreachable write path".
  // Comments are stripped first: a mutation that comments the call out must fail
  // this test, and a match against the raw source would pass on
  // `// void startUpdateSchedule(...)` — the mutation a reviewer plants to
  // falsify an assertion of exactly this shape.
  const mainSource = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'main.tsx'), 'utf8')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|\s)\/\/[^\n]*/g, '$1')

  it('runs the check through the schedule, not through an interval of its own', () => {
    expect(mainSource).toContain('startUpdateSchedule(')
    expect(mainSource).not.toMatch(/setInterval\(\s*\(\)\s*=>\s*void updateCheck\.run\(\)/)
    expect(mainSource).not.toContain('24 * 60 * 60 * 1000')
  })
})
