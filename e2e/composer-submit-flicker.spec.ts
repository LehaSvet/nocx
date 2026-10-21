import { appReadyForInput, test, expect } from './harness'

/**
 * nocx-1ebrs.40 — the composer flickers on submit.
 *
 * The submit hand-off removes the composer's box in one frame
 * (`display:none`, editor.ts `hide()`) and the hand-back restores it in one
 * frame (`show()`), so a quick command's input panel vanishes for an instant;
 * meanwhile nothing re-pins the scroller, content accumulates below the fold,
 * and the fold snaps back when a later mutation pins at last — the jump.
 *
 * This spec drives the REAL hand-off (a real pane, a real shell command) and
 * asserts, from frames sampled every animation frame:
 *
 *  1. the box's collapse is a TRANSITION — it passes through intermediate
 *     heights instead of stepping full-height → gone;
 *  2. the box's return is a TRANSITION — it grows through intermediate
 *     heights instead of popping back;
 *  3. the tail never detaches — while the person is following, content does
 *     not accumulate below the fold on any painted frame, so no later pin
 *     can land as a jump.
 */

const TITLE = '.nocx-tab-title'
const INPUT = '.pane.active .nocx-editor-input'

interface Frame {
  t: number
  disp: string
  eH: number
  st: number
  ch: number
  sh: number
  iT: number
  gl: number
}

test('submit hands the panel over as a transition, and the tail never detaches', async ({
  page,
}, testInfo) => {
  await page.goto('/')
  await expect(page.locator(TITLE).first()).not.toHaveText('', { timeout: 15000 })
  await appReadyForInput(page)
  const input = page.locator(INPUT)
  await expect(input).toBeVisible({ timeout: 8000 })
  // Long enough that the collapse completes and the panel is genuinely gone
  // for a while — the shape the owner sees on any ordinary command — short
  // enough that the spec stays quick.
  await input.fill('sleep 0.4')

  await page.evaluate(() => {
    const pane = document.querySelector('.pane.active') as HTMLElement
    const editor = pane.querySelector<HTMLElement>('.nocx-editor') as HTMLElement
    const area = pane.querySelector<HTMLElement>('.scrollback-area') as HTMLElement
    const inner = pane.querySelector<HTMLElement>('.scrollback-inner') as HTMLElement
    const w = window as unknown as { __frames: Frame[]; __t0: number; __stop?: boolean }
    w.__frames = []
    w.__t0 = performance.now()
    const sample = (): void => {
      if (w.__stop) return
      w.__frames.push({
        t: +(performance.now() - w.__t0).toFixed(1),
        disp: getComputedStyle(editor).display,
        eH: +editor.getBoundingClientRect().height.toFixed(1),
        st: +area.scrollTop.toFixed(1),
        ch: area.clientHeight,
        sh: area.scrollHeight,
        iT: +inner.getBoundingClientRect().top.toFixed(1),
        gl: inner.getAnimations().length,
      })
      requestAnimationFrame(sample)
    }
    requestAnimationFrame(sample)
  })

  // The panel's resting height, read before anything moves.
  const fullH = await page.evaluate(
    () => document.querySelector('.pane.active .nocx-editor')?.getBoundingClientRect().height ?? 0,
  )

  await page.keyboard.press('Enter')

  // The hand-back: the composer returns when the prompt does. NOT
  // toBeVisible — the animated collapse keeps the box non-empty (visible,
  // to Playwright) while it is leaving — the return is the box back at its
  // height. The settle glide lands before the pane is at rest.
  await page.waitForFunction(
    (full) => {
      const ed = document.querySelector('.pane.active .nocx-editor')
      return (
        ed !== null &&
        getComputedStyle(ed).display !== 'none' &&
        ed.getBoundingClientRect().height >= full * 0.9
      )
    },
    fullH,
    { timeout: 15000 },
  )
  await page.waitForFunction(
    () =>
      (document.querySelector('.pane.active .scrollback-inner')?.getAnimations().length ?? 0) === 0,
  )
  await page.evaluate(() => {
    const w = window as unknown as { __stop?: boolean }
    requestAnimationFrame(() => {
      w.__stop = true
    })
  })
  await page.waitForTimeout(120)
  const frames = (await page.evaluate(
    () => (window as unknown as { __frames: Frame[] }).__frames,
  )) as Frame[]

  const dump = JSON.stringify(frames)
  await testInfo.attach('frames', { body: dump, contentType: 'application/json' })
  expect(frames.length, `frames sampled: ${dump}`).toBeGreaterThan(6)

  // The panel's resting height before anything happened.
  expect(fullH, `panel has a box before submit: ${dump}`).toBeGreaterThan(20)

  // The submit frame: the first sample that differs from the pre-submit rest.
  let submitAt = frames.findIndex((f) => f.eH < fullH - 1 || f.disp === 'none' || f.gl > 0)
  if (submitAt < 0) submitAt = frames.length - 1

  // ── 1 + 2: the box leaves and returns as a transition ──
  const absent = frames.map((f, i) => ({ f, i })).filter(({ f }) => f.eH <= 1)
  if (absent.length === 0) {
    // The prompt returned before the collapse finished: the panel never
    // fully left. The flicker is dead only if the dip itself was gradual.
    const dipped = frames.slice(submitAt).some((f) => f.eH > 1 && f.eH < fullH * 0.75)
    expect(dipped, `panel dipped gradually, frames: ${dump}`).toBe(true)
  } else {
    const firstAbsent = absent[0].i
    const lastFull = frames
      .slice(0, firstAbsent)
      .map((f, i) => ({ f, i }))
      .filter(({ f }) => f.eH >= fullH * 0.9)
      .pop()
    expect(lastFull, `panel was full before it left: ${dump}`).toBeTruthy()
    // Collapse: between the last full-height frame and the first gone frame
    // there must be an intermediate height. The old code steps 130 → 0.
    const collapse = frames
      .slice(lastFull!.i + 1, firstAbsent)
      .some((f) => f.eH > 1 && f.eH < fullH * 0.75)
    expect(collapse, `collapse passes through intermediate heights, frames: ${dump}`).toBe(true)

    const lastAbsent = absent[absent.length - 1].i
    const restoredAt = frames.findIndex((f, i) => i > lastAbsent && f.eH >= fullH * 0.9)
    expect(restoredAt, `panel returns to full height, frames: ${dump}`).toBeGreaterThan(0)
    // Restore: between gone and full there must be an intermediate height.
    // The old code pops 0 → 130 in one frame.
    const restore = frames
      .slice(lastAbsent + 1, restoredAt)
      .some((f) => f.eH > 1 && f.eH < fullH * 0.75)
    expect(restore, `restore grows through intermediate heights, frames: ${dump}`).toBe(true)
  }

  // ── 3: the tail never detaches while the person is following ──
  // Content below the fold, per painted frame. The old code lets ~130px
  // accumulate during the vanish and sits there for frames before a late pin
  // snaps it back. A box change can show ONE sampled frame of small
  // under-flow — the sampler's animation-frame callback runs before that
  // frame's ResizeObserver delivery, and the pin lands before paint — so a
  // PERSISTENT fold is the defect, and so is any accumulation of the old
  // size. The old run: 127px, four consecutive samples; the fixed run's one
  // pre-RO sample can be a whole block tall for a single frame and is still
  // never painted.
  const below = frames
    .slice(submitAt + 1)
    .map((f) => ({ f, below: +(f.sh - f.st - f.ch).toFixed(1) }))
  const persistent = below.filter(({ below: b }, i) => {
    if (b <= 2) return false
    const next = below[i + 1]
    return next === undefined || next.below > 2
  })
  expect(
    persistent,
    `fold stayed detached across frames (px): ${JSON.stringify(persistent)}; all: ${dump}`,
  ).toEqual([])
})
