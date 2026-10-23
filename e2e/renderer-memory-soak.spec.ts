import { mkdirSync, writeFileSync } from 'node:fs'
import path from 'node:path'

import { test, expect, promptReady } from './harness'

const TAB = '.nocx-tab'
const NEW_TAB = '[aria-label="New tab"]'
const CLOSE_TAB = '[aria-label="Close tab"]'
const CYCLES = Number(process.env.NOCX_MEMORY_CYCLES ?? 18)
const OUTPUT_ROUNDS = Number(process.env.NOCX_MEMORY_OUTPUT_ROUNDS ?? 12)
const RETAINED_HEAP_GROWTH_LIMIT = Number(
  process.env.NOCX_MEMORY_HEAP_GROWTH_LIMIT ?? 24 * 1024 * 1024,
)
const RETAINED_DOM_GROWTH_LIMIT = Number(process.env.NOCX_MEMORY_DOM_GROWTH_LIMIT ?? 250)

type Metrics = {
  JSHeapUsedSize?: number
  JSHeapTotalSize?: number
  Nodes?: number
  Documents?: number
  LayoutCount?: number
  RecalcStyleCount?: number
}

async function metrics(page: import('@playwright/test').Page): Promise<Metrics> {
  const cdp = await page.context().newCDPSession(page)
  try {
    await cdp.send('Performance.enable')
    const result = (await cdp.send('Performance.getMetrics')) as {
      metrics: { name: string; value: number }[]
    }
    return Object.fromEntries(result.metrics.map(({ name, value }) => [name, value])) as Metrics
  } finally {
    await cdp.detach()
  }
}

async function snapshot(page: import('@playwright/test').Page): Promise<string> {
  const cdp = await page.context().newCDPSession(page)
  try {
    await cdp.send('HeapProfiler.enable')
    await cdp.send('HeapProfiler.collectGarbage')
    const chunks: string[] = []
    cdp.on('HeapProfiler.addHeapSnapshotChunk', (event: { chunk: string }) =>
      chunks.push(event.chunk),
    )
    await cdp.send('HeapProfiler.takeHeapSnapshot', {
      reportProgress: false,
      captureNumericValue: true,
    })
    return chunks.join('')
  } finally {
    await cdp.detach()
  }
}

async function settled(page: import('@playwright/test').Page): Promise<void> {
  await expect(page.locator('.pane.active .cmd-block-running')).toHaveCount(0, { timeout: 15000 })
}

async function send(page: import('@playwright/test').Page, command: string): Promise<void> {
  const input = page.locator('.pane.active .nocx-editor-input')
  await input.fill(command)
  await page.keyboard.press('Enter')
  await expect(page.locator('.pane.active .cmd-block-running')).toHaveCount(1, { timeout: 15000 })
  await settled(page)
}

async function openSix(page: import('@playwright/test').Page): Promise<void> {
  await expect(page.locator(TAB)).toHaveCount(1)
  await promptReady(page)
  for (let i = 1; i < 6; i++) {
    await page.locator(NEW_TAB).click()
    await expect(page.locator(TAB)).toHaveCount(i + 1)
    await promptReady(page)
  }
}

function snapshotObjectCounts(snapshotText: string): Record<string, number> {
  const snapshot = JSON.parse(snapshotText) as {
    snapshot: { meta: { node_fields: string[]; node_types: string[][] } }
    nodes: number[]
    strings: string[]
  }
  const fields = snapshot.snapshot.meta.node_fields
  const stride = fields.length
  const nameIndex = fields.indexOf('name')
  const typeIndex = fields.indexOf('type')
  const detachedIndex = fields.indexOf('detachedness')
  const typeNames = snapshot.snapshot.meta.node_types[typeIndex]
  const counts: Record<string, number> = {}
  let detachedNodes = 0
  for (let index = 0; index < snapshot.nodes.length; index += stride) {
    if (snapshot.nodes[index + detachedIndex] > 0) detachedNodes++
    if (typeNames[snapshot.nodes[index + typeIndex]] !== 'object') continue
    const name = snapshot.strings[snapshot.nodes[index + nameIndex]]
    if (name === 'Pane' || name === 'TerminalContent') counts[name] = (counts[name] ?? 0) + 1
  }
  counts.DetachedHeapNodes = detachedNodes
  return counts
}

function awaitFrame(page: import('@playwright/test').Page): Promise<void> {
  return page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())))
}

test('renderer memory soak: six live tabs, output/redraw and tab churn (nocx-1ebrs.71)', async ({
  page,
}, info) => {
  test.setTimeout(180000)
  const startedAt = Date.now()
  await page.goto('/')
  await openSix(page)

  // Ensure every tab owns a ready PTY and receives repeated continuous output plus
  // full-screen redraws. Observable command completion replaces fixed sleeps.
  for (let tab = 0; tab < 6; tab++) {
    await page.locator(TAB).nth(tab).click()
    await promptReady(page)
    const marker = `NOCX_SOAK_${tab}_${Date.now().toString(36)}`
    const command = `i=0; while [ $i -lt ${OUTPUT_ROUNDS} ]; do i=$((i+1)); printf '\\033[?1049h\\033[H\\033[2J${marker} %s\\n\\033[?1049l\\033[H\\033[2J' "$i"; done`
    await send(page, command)
  }

  // Allow pending paints and microtasks to reach an observable frame boundary.
  await awaitFrame(page)
  const cdp = await page.context().newCDPSession(page)
  const artifacts = path.join(info.outputDir, 'renderer-memory')
  mkdirSync(artifacts, { recursive: true })
  const beforeDOM = await page.evaluate(() => ({
    nodes: document.querySelectorAll('*').length,
    detachedPaneNodes: [...document.querySelectorAll('.pane')].filter((node) => !node.isConnected)
      .length,
    detachedEditors: [...document.querySelectorAll('.nocx-editor-input')].filter(
      (node) => !node.isConnected,
    ).length,
  }))
  const beforeSnapshot = await snapshot(page)
  const beforeRetained = snapshotObjectCounts(beforeSnapshot)
  writeFileSync(path.join(artifacts, 'before.heapsnapshot'), beforeSnapshot)
  // Snapshot capture forces GC; measure the baseline after it so the later
  // retained-heap comparison uses the same post-GC condition.
  const beforeMetrics = await metrics(page)

  const sample: { cycle: number; metrics: Metrics; dom: { nodes: number; paneNodes: number } }[] =
    []
  for (let cycle = 1; cycle <= CYCLES; cycle++) {
    // Close the active sixth tab and wait for the renderer's observable DOM
    // teardown, then open a replacement and wait for its prompt. This preserves
    // six simultaneous live sessions while exercising disposal repeatedly.
    await page.locator(CLOSE_TAB).last().click()
    await expect(page.locator(TAB)).toHaveCount(5)
    await page.locator(NEW_TAB).click()
    await expect(page.locator(TAB)).toHaveCount(6)
    await promptReady(page)
    const marker = `NOCX_CYCLE_${cycle}_${Date.now().toString(36)}`
    await send(
      page,
      `printf '\\033[?1049h\\033[H\\033[2J${marker}\\n'; printf '\\033[?1049l\\033[H\\033[2J'; echo ${marker}`,
    )
    await awaitFrame(page)
    if (cycle % 3 === 0 || cycle === CYCLES) {
      sample.push({
        cycle,
        metrics: await metrics(page),
        dom: await page.evaluate(() => ({
          nodes: document.querySelectorAll('*').length,
          paneNodes: document.querySelectorAll('.pane').length,
        })),
      })
    }
  }
  await cdp.detach()
  const afterDOM = await page.evaluate(() => ({
    nodes: document.querySelectorAll('*').length,
    detachedPaneNodes: [...document.querySelectorAll('.pane')].filter((node) => !node.isConnected)
      .length,
    detachedEditors: [...document.querySelectorAll('.nocx-editor-input')].filter(
      (node) => !node.isConnected,
    ).length,
  }))
  const afterSnapshot = await snapshot(page)
  const afterRetained = snapshotObjectCounts(afterSnapshot)
  writeFileSync(path.join(artifacts, 'after.heapsnapshot'), afterSnapshot)
  const afterMetrics = await metrics(page)
  const report = {
    browser: info.project.name,
    browserVersion: page.context().browser()?.version() ?? 'unknown',
    durationMs: Date.now() - startedAt,
    cycles: CYCLES,
    outputRoundsPerTab: OUTPUT_ROUNDS,
    tabs: 6,
    beforeMetrics,
    afterMetrics,
    beforeDOM,
    afterDOM,
    beforeRetained,
    afterRetained,
    sample,
  }
  writeFileSync(path.join(artifacts, 'measurements.json'), JSON.stringify(report, null, 2))

  // Compare like-for-like counts on the live renderer after explicit GC. CDP
  // heap snapshots and retained heap are saved for attribution.
  const gc = await page.context().newCDPSession(page)
  await gc.send('HeapProfiler.collectGarbage')
  await gc.detach()
  const settledMetrics = await metrics(page)
  const heapGrowth =
    (settledMetrics.JSHeapUsedSize ?? afterMetrics.JSHeapUsedSize ?? 0) -
    (beforeMetrics.JSHeapUsedSize ?? 0)
  const nodeGrowth = afterDOM.nodes - beforeDOM.nodes
  expect(
    heapGrowth,
    `retained JS heap grew ${heapGrowth} bytes; report: ${artifacts}`,
  ).toBeLessThanOrEqual(RETAINED_HEAP_GROWTH_LIMIT)
  expect(nodeGrowth, `DOM node count grew ${nodeGrowth}; report: ${artifacts}`).toBeLessThanOrEqual(
    RETAINED_DOM_GROWTH_LIMIT,
  )
  expect(
    afterRetained.Pane ?? 0,
    `heap snapshot retains ${afterRetained.Pane ?? 0} Pane objects (baseline ${beforeRetained.Pane ?? 0})`,
  ).toBeLessThanOrEqual((beforeRetained.Pane ?? 0) + 1)
  expect(
    afterRetained.TerminalContent ?? 0,
    `heap snapshot retains ${afterRetained.TerminalContent ?? 0} TerminalContent objects (baseline ${beforeRetained.TerminalContent ?? 0})`,
  ).toBeLessThanOrEqual((beforeRetained.TerminalContent ?? 0) + 1)
  expect(afterDOM.detachedPaneNodes).toBe(0)
  expect(afterDOM.detachedEditors).toBe(0)
})
