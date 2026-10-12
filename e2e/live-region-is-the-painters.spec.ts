/**
 * The cell painter is the only live terminal renderer (nocx-zg3k3.8).
 * A user types and executes a command, sees its output in the painted grid,
 * and clicks a painted cell. The browser sends zero-based mouse intent; the
 * backend runtime encodes it for the program as one-based SGR coordinates.
 */

import { test, expect, promptReady } from './harness'
import type { Page } from './harness'

const LIVE = '.pane.active .terminal-live-grid'

test('painted output and backend mouse intent work without xterm or session.output replay', async ({
  page,
  browserName,
}) => {
  let sessionOutputCalls = 0
  const mouseIntents: string[] = []
  page.on('websocket', (socket) => {
    socket.on('framesent', (frame) => {
      if (typeof frame.payload !== 'string') return
      try {
        const decoded: unknown = JSON.parse(frame.payload)
        const envelopes = Array.isArray(decoded) ? decoded : [decoded]
        for (const envelope of envelopes) {
          if (typeof envelope !== 'object' || envelope === null || !('method' in envelope)) continue
          if (envelope.method === 'session.output') sessionOutputCalls += 1
          if (envelope.method !== 'session.intent' || !('params' in envelope)) continue
          const params = envelope.params
          if (
            typeof params === 'object' &&
            params !== null &&
            'kind' in params &&
            params.kind === 'mouse' &&
            'payload' in params &&
            typeof params.payload === 'string'
          ) {
            mouseIntents.push(Buffer.from(params.payload, 'base64').toString('utf8'))
          }
        }
      } catch {
        // Non-JSON text frames are unrelated to JSON-RPC requests.
      }
    })
  })

  await page.setViewportSize({ width: 1280, height: 800 })
  await page.goto('/')
  await promptReady(page)

  const nonce = Date.now().toString(36)
  const marker = `NOCX-PAINTED-${nonce}`
  await page.keyboard.type(`printf '%s\\n' 'NOCX-PAI''NTED-${nonce}'`)
  await page.keyboard.press('Enter')
  await expect
    .poll(
      async () =>
        page
          .locator(`${LIVE} .term-grid-row`)
          .evaluateAll(
            (rows, outputMarker) => rows.some((row) => row.textContent?.includes(outputMarker)),
            marker,
          ),
      { timeout: 15_000, message: 'the painted grid never carried the output marker' },
    )
    .toBe(true)
  await expect(page.locator(`${LIVE} .term-grid-row`, { hasText: marker })).toHaveCount(1)
  await expect(page.locator('.pane.active .xterm')).toHaveCount(0)
  await expect(page.locator('.pane.active canvas')).toHaveCount(0)

  // The zoom proof uses CDP's device-metrics override, available only in
  // Chromium. The first geometry still runs in every browser.
  await clickRound(page, 12, 'Z1-OK', mouseIntents)
  if (browserName === 'chromium') {
    // This spec proves painted mouse intent/SGR, not implicit click-to-focus.
    // Use the visible Interrupt control only to clean up cat -v.
    const interrupt = page.locator('.pane.active').getByRole('button', { name: 'Interrupt' })
    await expect(interrupt).toBeVisible()
    await interrupt.click()
    await promptReady(page)
    // cat -v does not restore the DEC mouse modes set by clickRound.
    await page.keyboard.type("printf '\\033[?1000l\\033[?1006l'")
    await page.keyboard.press('Enter')
    await promptReady(page)
    const cellWidthBefore = await page.evaluate(
      () =>
        (
          window as unknown as {
            __nocxPaneScreen?: () => { geometry: { cellWidthPx: number } | null } | null
          }
        ).__nocxPaneScreen?.()?.geometry?.cellWidthPx ?? 0,
    )
    const cdp = await page.context().newCDPSession(page)
    await cdp.send('Emulation.setDeviceMetricsOverride', {
      width: 1282,
      height: 802,
      deviceScaleFactor: 2,
      mobile: false,
    })
    expect(await page.evaluate(() => window.devicePixelRatio)).toBe(2)
    await expect
      .poll(
        async () =>
          page.evaluate(
            () =>
              (
                window as unknown as {
                  __nocxPaneScreen?: () => { geometry: { cellWidthPx: number } | null } | null
                }
              ).__nocxPaneScreen?.()?.geometry?.cellWidthPx ?? 0,
          ),
        { timeout: 15_000, message: 'the committed cell metric never moved with the zoom' },
      )
      .toBeGreaterThan(cellWidthBefore * 1.5)
    await clickRound(page, 30, 'Z2-OK', mouseIntents)
    await cdp.send('Emulation.clearDeviceMetricsOverride')
  }

  expect(sessionOutputCalls).toBe(0)
})

interface ClickTarget {
  x: number
  y: number
  row: number
  col: number
}

async function clickTarget(
  page: Page,
  marker: string,
  colWithin: number,
): Promise<{ target: ClickTarget | null; diagnostic: string }> {
  return page.evaluate(
    ({ marker, colWithin }) => {
      const live = document.querySelector<HTMLElement>('.pane.active .terminal-live-grid')
      const clip = document.querySelector<HTMLElement>('.pane.active .terminal-live-viewport')
      const rows = live ? [...live.querySelectorAll<HTMLElement>('.term-grid-row')] : []
      const row = rows.find((candidate) => candidate.textContent?.includes(marker))
      const rowIndex = row ? rows.indexOf(row) : -1
      const screen = (
        window as Window & {
          __nocxPaneScreen?: () => { geometry: { cols: number; rows: number } | null } | null
          __nocxPaneCellToPixel?: (col: number, row: number) => { x: number; y: number } | null
        }
      ).__nocxPaneScreen?.()
      const geometry = screen?.geometry
      if (!live || !clip || !row || rowIndex < 0 || !geometry) {
        return {
          target: null,
          diagnostic: JSON.stringify({
            live: Boolean(live),
            clip: Boolean(clip),
            row: Boolean(row),
            rowIndex,
            geometry,
          }),
        }
      }
      const clipRect = clip.getBoundingClientRect()
      const liveRect = live.getBoundingClientRect()
      const rowRect = row.getBoundingClientRect()
      const col = Math.min(colWithin, geometry.cols - 1)
      const point = (
        window as Window & {
          __nocxPaneCellToPixel?: (col: number, row: number) => { x: number; y: number } | null
        }
      ).__nocxPaneCellToPixel?.(col, rowIndex)
      const cellWidth = geometry.cols > 0 ? rowRect.width / geometry.cols : 0
      if (!point || cellWidth <= 0 || rowRect.height <= 0) {
        return {
          target: null,
          diagnostic: JSON.stringify({
            point,
            cols: geometry.cols,
            rows: geometry.rows,
            rowIndex,
            cellWidth,
            rowHeight: rowRect.height,
            rowWidth: rowRect.width,
          }),
        }
      }
      const x = liveRect.left + point.x + cellWidth / 2
      const y = liveRect.top + point.y + rowRect.height / 2
      if (rowRect.top < clipRect.top || rowRect.bottom > clipRect.bottom) {
        return {
          target: null,
          diagnostic: JSON.stringify({
            reason: 'row clipped',
            row: { top: rowRect.top, bottom: rowRect.bottom },
            clip: { top: clipRect.top, bottom: clipRect.bottom },
          }),
        }
      }
      if (x < clipRect.left || x > clipRect.right) {
        return {
          target: null,
          diagnostic: JSON.stringify({
            reason: 'x clipped',
            x,
            clip: { left: clipRect.left, right: clipRect.right },
          }),
        }
      }
      return { target: { x, y, row: rowIndex, col }, diagnostic: 'click target ready' }
    },
    { marker, colWithin },
  )
}

async function clickRound(
  page: Page,
  colWithin: number,
  roundMarker: string,
  mouseIntents: string[],
): Promise<void> {
  const splitAt = Math.ceil(roundMarker.length / 2)
  const shellMarker = `${roundMarker.slice(0, splitAt)}''${roundMarker.slice(splitAt)}`
  await page.keyboard.type(
    `printf '%s\\n' '${shellMarker}'; printf '\\033[?1000h\\033[?1006h'; cat -v`,
  )
  await page.keyboard.press('Enter')
  await expect(page.locator(LIVE)).toContainText(roundMarker, { timeout: 15_000 })

  let target: ClickTarget | null = null
  let diagnostic = 'no probe result'
  try {
    await expect
      .poll(
        async () => {
          const probe = await clickTarget(page, roundMarker, colWithin)
          target = probe.target
          diagnostic = probe.diagnostic
          return target !== null
        },
        { timeout: 15_000, message: 'no clickable painted row ever appeared' },
      )
      .toBe(true)
  } catch {
    throw new Error(`no clickable painted row ever appeared; last probe: ${diagnostic}`)
  }

  const intentStart = mouseIntents.length
  await page.mouse.click(target!.x, target!.y)

  // The pane emits a zero-based intent for the exact cell the painted grid
  // selected. No browser-side code constructs a terminal escape sequence.
  const expectedIntent = `press left ${target!.col} ${target!.row}`
  await expect.poll(() => mouseIntents.slice(intentStart).includes(expectedIntent)).toBe(true)
  const expectedReleaseIntent = `release left ${target!.col} ${target!.row}`
  await expect
    .poll(() => mouseIntents.slice(intentStart).includes(expectedReleaseIntent))
    .toBe(true)

  // cat -v prints both backend-generated SGR reports back into the painted
  // screen. Wait for the release too, so cleanup cannot race the click's tail.
  const expectedSGR = `^[[<0;${target!.col + 1};${target!.row + 1}M`
  const expectedReleaseSGR = `^[[<0;${target!.col + 1};${target!.row + 1}m`
  for (const sgr of [expectedSGR, expectedReleaseSGR]) {
    await expect
      .poll(() =>
        page.locator(LIVE).evaluate((live, wanted) => live.textContent?.includes(wanted), sgr),
      )
      .toBe(true)
  }
}
