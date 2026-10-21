import { appReadyForInput, test, expect } from './harness'

/**
 * nocx-1ebrs.40 — the composer flickers on submit.
 *
 * The submit hand-off must not blink the input panel. From painted frames
 * (one sample per animation frame across the hand-off) this asserts the
 * shape the fix is:
 *
 *  1. the pane-level geometry the grid and the pty are sized from never
 *     animates — the box's own height is always its full height or gone
 *     (display:none), never anything between;
 *  2. the INK fades — the panel's departure and its return pass through
 *     intermediate opacities instead of stepping visible↔gone in one frame;
 *  3. the tail never detaches — while the person is following, content does
 *     not stay below the fold across frames, so no late pin lands as a jump.
 */

const TITLE = '.nocx-tab-title'
const INPUT = '.pane.active .nocx-editor-input'

interface Frame {
  t: number
  disp: string
  eH: number
  op: number
  st: number
  ch: number
  sh: number
  iT: number
  gl: number
}

async function sampleAcrossSubmit(page: import('@playwright/test').Page, command: string) {
  await page.goto('/')
  await expect(page.locator(TITLE).first()).not.toHaveText('', { timeout: 15000 })
  await appReadyForInput(page)
  const input = page.locator(INPUT)
  await expect(input).toBeVisible({ timeout: 8000 })
  await input.fill(command)

  await page.evaluate(() => {
    const pane = document.querySelector('.pane.active') as HTMLElement
    const editor = pane.querySelector<HTMLElement>('.nocx-editor') as HTMLElement
    const area = pane.querySelector<HTMLElement>('.scrollback-area') as HTMLElement
    const inner = pane.querySelector<HTMLElement>('.scrollback-inner') as HTMLElement
    const w = window as unknown as { __frames: unknown[]; __t0: number; __stop?: boolean }
    w.__frames = []
    w.__t0 = performance.now()
    const sample = (): void => {
      if (w.__stop) return
      w.__frames.push({
        t: +(performance.now() - w.__t0).toFixed(1),
        disp: getComputedStyle(editor).display,
        eH: +editor.getBoundingClientRect().height.toFixed(1),
        op: +getComputedStyle(editor).opacity.slice(0, 5),
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

  // The panel's resting height and ink, read before anything moves.
  const fullH = await page.evaluate(
    () => document.querySelector('.pane.active .nocx-editor')?.getBoundingClientRect().height ?? 0,
  )

  await page.keyboard.press('Enter')

  // The hand-back: the box is back at its full height and its full ink. NOT
  // toBeVisible — a mid-fade box is non-empty and Playwright-visible.
  await page.waitForFunction(
    (full) => {
      const ed = document.querySelector('.pane.active .nocx-editor')
      return (
        ed !== null &&
        getComputedStyle(ed).display !== 'none' &&
        ed.getBoundingClientRect().height >= full * 0.9 &&
        Number(getComputedStyle(ed).opacity) >= 0.95
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
  return {
    fullH,
    frames: (await page.evaluate(
      () => (window as unknown as { __frames: unknown[] }).__frames,
    )) as Frame[],
  }
}

test.describe('the submit hand-off (nocx-1ebrs.40)', () => {
  test('a quick command dips the panel and turns it around; the geometry never animates', async ({
    page,
  }, testInfo) => {
    // The owner's case: a command that finishes while the departure is still
    // on screen. The panel must dim and come back — never vanish, never pop.
    const { fullH, frames } = await sampleAcrossSubmit(page, 'true')
    const dump = JSON.stringify(frames)
    await testInfo.attach('frames', { body: dump, contentType: 'application/json' })
    expect(frames.length, `frames sampled: ${dump}`).toBeGreaterThan(6)

    // 1. The box's own height is full or gone, never between: the pane-level
    // geometry the grid and the pty are fitted from does not animate.
    const midHeight = frames.filter((f) => f.eH > 1 && f.eH < fullH - 1)
    expect(
      midHeight,
      `box at intermediate heights: ${JSON.stringify(midHeight)}; all: ${dump}`,
    ).toEqual([])

    // 2. The ink moved gradually: it dipped below full and came back.
    const dipped = frames.some((f) => f.disp !== 'none' && f.op < 0.95)
    expect(dipped, `panel ink dipped during the hand-off: ${dump}`).toBe(true)
    const last = frames[frames.length - 1]
    expect(last.op, `panel back at full ink: ${dump}`).toBeGreaterThanOrEqual(0.95)
    expect(last.eH, `panel back at full height: ${dump}`).toBeGreaterThanOrEqual(fullH - 1)
  })

  test('a real command departs through a fade, and the tail never detaches', async ({
    page,
  }, testInfo) => {
    // Long enough that the departure completes and the panel is genuinely
    // gone for a while — the shape the owner sees on any ordinary command.
    const { fullH, frames } = await sampleAcrossSubmit(page, 'sleep 0.4')
    const dump = JSON.stringify(frames)
    await testInfo.attach('frames', { body: dump, contentType: 'application/json' })
    expect(frames.length, `frames sampled: ${dump}`).toBeGreaterThan(6)

    // 1. Two-valued layout height, all the way through.
    const midHeight = frames.filter((f) => f.eH > 1 && f.eH < fullH - 1)
    expect(
      midHeight,
      `box at intermediate heights: ${JSON.stringify(midHeight)}; all: ${dump}`,
    ).toEqual([])

    // 2. The departure fades: between the last full-ink frame and the first
    // gone frame there is an intermediate opacity. The old code stepped
    // visible → gone in one frame.
    let submitAt = frames.findIndex((f) => f.disp === 'none' || f.op < 0.95 || f.eH < fullH - 1)
    if (submitAt < 0) submitAt = frames.length - 1
    const absent = frames.map((f, i) => ({ f, i })).filter(({ f }) => f.disp === 'none')
    expect(absent.length, `panel fully departs: ${dump}`).toBeGreaterThan(0)
    const lastFullInk = frames
      .slice(0, absent[0].i)
      .map((f, i) => ({ f, i }))
      .filter(({ f }) => f.op >= 0.95 && f.eH >= fullH - 1)
      .pop()
    expect(lastFullInk, `panel was full before it left: ${dump}`).toBeTruthy()
    const fadeOut = frames
      .slice(lastFullInk!.i + 1, absent[0].i)
      .some((f) => f.disp !== 'none' && f.op > 0.05 && f.op < 0.9)
    expect(fadeOut, `departure passes through intermediate ink: ${dump}`).toBe(true)

    // And the return fades back in from nothing rather than popping.
    const lastAbsent = absent[absent.length - 1].i
    const restoredAt = frames.findIndex(
      (f, i) => i > lastAbsent && f.eH >= fullH - 1 && f.op >= 0.95,
    )
    expect(restoredAt, `panel returns at full ink: ${dump}`).toBeGreaterThan(0)
    const fadeIn = frames
      .slice(lastAbsent + 1, restoredAt)
      .some((f) => f.disp !== 'none' && f.op > 0.05 && f.op < 0.9)
    expect(fadeIn, `return passes through intermediate ink: ${dump}`).toBe(true)

    // 3. The tail never detaches while the panel is gone: content below the
    // fold on the ABSENT frames. The old code let ~130px accumulate there
    // (echo + process bar, nothing re-pinning) and sat on it for frames
    // before a late pin snapped scrollTop 98px in one frame. After the
    // panel is back the hand-off settle owns the transition and the idle
    // re-fit drains the content glided — painted steps stay small and the
    // fold metric decays without a scroll — so the drain is asserted by its
    // painted continuity below, not here.
    const belowAbsent = frames
      .slice(submitAt + 1)
      .map((f) => ({ disp: f.disp, below: +(f.sh - f.st - f.ch).toFixed(1) }))
      .filter(({ disp }) => disp === 'none')
    const stuck = belowAbsent.filter(({ below: b }, i) => {
      if (b <= 2) return false
      const next = belowAbsent[i + 1]
      return next !== undefined && next.below > 2
    })
    expect(
      stuck,
      `fold stayed detached while the panel was gone (px): ${JSON.stringify(stuck)}; all: ${dump}`,
    ).toEqual([])

    // 4. The restore is continuous in what was PAINTED: between the last
    // absent frame and the restored one, the stack's painted top never steps
    // far in one frame. The old code teleported it ~75px when the box popped
    // back; the glided restore moves it a few tens of px at most.
    const steps: { from: number; to: number; dy: number }[] = []
    for (let i = lastAbsent + 1; i < restoredAt; i++) {
      const dy = +(frames[i].iT - frames[i - 1].iT).toFixed(1)
      if (Math.abs(dy) > 40) steps.push({ from: frames[i - 1].iT, to: frames[i].iT, dy })
    }
    expect(
      steps,
      `painted stack stepped mid-restore: ${JSON.stringify(steps)}; all: ${dump}`,
    ).toEqual([])
    // And the hand-off ends with the tail where the reader can see it.
    const lastBelow = +(
      frames[frames.length - 1].sh -
      frames[frames.length - 1].st -
      frames[frames.length - 1].ch
    ).toFixed(1)
    expect(lastBelow, `fold at rest: ${dump}`).toBeLessThanOrEqual(2)
  })
})
