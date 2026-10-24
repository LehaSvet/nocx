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

async function send(
  page: import('@playwright/test').Page,
  command: string,
  outputMarker: string,
): Promise<void> {
  const input = page.locator('.pane.active .nocx-editor-input')
  await input.fill(command)
  await page.keyboard.press('Enter')
  const block = page
    .locator('.pane.active .cmd-block[data-block-kind="command"]')
    .filter({ hasText: outputMarker })
  await expect(block).toHaveCount(1)
  await expect(block.locator('.cmd-output')).toContainText(outputMarker)
  await expect(block).not.toHaveClass(/cmd-block-running/)
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

test('renderer memory soak: retained objects return to live tab count after churn (nocx-1ebrs.71)', async ({
  page,
  browserName,
}, info) => {
  test.setTimeout(180000)
  const startedAt = Date.now()
  await page.goto('/')
  await openSix(page)

  // Exercise output and redraw in every live tab before churning replacements.
  for (let tab = 0; tab < 6; tab++) {
    await page.locator(TAB).nth(tab).click()
    await promptReady(page)
    const marker = `NOCX_SOAK_${tab}_${Date.now().toString(36)}`
    const command = `i=0; while [ $i -lt ${OUTPUT_ROUNDS} ]; do i=$((i+1)); printf '\\033[?1049h\\033[H\\033[2J${marker} %s\\n\\033[?1049l\\033[H\\033[2J' "$i"; done; printf '\\n%s_ROUNDS=%s\\n' '${marker}' "$i"`
    await send(page, command, `${marker}_ROUNDS=${OUTPUT_ROUNDS}`)
  }

  await awaitFrame(page)
  const retainedObjects = async () =>
    page.evaluate(() => {
      const host = window as typeof window & {
        __nocxRendererRetainedObjects?: () => { panes: number; terminalContents: number }
      }
      return host.__nocxRendererRetainedObjects?.() ?? null
    })
  const expectedLiveCount = await page.locator(TAB).count()
  const beforeRetainedObjects = await retainedObjects()
  expect(beforeRetainedObjects, 'in-page retained-object observable is installed').not.toBeNull()
  expect(beforeRetainedObjects).toEqual({
    panes: expectedLiveCount,
    terminalContents: expectedLiveCount,
  })
  const beforeDOM = await page.evaluate(() => ({
    nodes: document.querySelectorAll('*').length,
    detachedPaneNodes: [...document.querySelectorAll('.pane')].filter((node) => !node.isConnected)
      .length,
    detachedEditors: [...document.querySelectorAll('.nocx-editor-input')].filter(
      (node) => !node.isConnected,
    ).length,
  }))

  const sample: {
    cycle: number
    dom: { nodes: number; paneNodes: number }
    retainedObjects: { panes: number; terminalContents: number } | null
  }[] = []
  for (let cycle = 1; cycle <= CYCLES; cycle++) {
    await page.locator(CLOSE_TAB).last().click()
    await expect(page.locator(TAB)).toHaveCount(5)
    await page.locator(NEW_TAB).click()
    await expect(page.locator(TAB)).toHaveCount(6)
    await promptReady(page)
    const marker = `NOCX_CYCLE_${cycle}_${Date.now().toString(36)}`
    await send(
      page,
      `printf '\\033[?1049h\\033[H\\033[2J${marker}\\n'; printf '\\033[?1049l\\033[H\\033[2J'; echo ${marker}`,
      marker,
    )
    await awaitFrame(page)
    if (cycle % 3 === 0 || cycle === CYCLES) {
      sample.push({
        cycle,
        dom: await page.evaluate(() => ({
          nodes: document.querySelectorAll('*').length,
          paneNodes: document.querySelectorAll('.pane').length,
        })),
        retainedObjects: await retainedObjects(),
      })
    }
  }

  const afterDOM = await page.evaluate(() => ({
    nodes: document.querySelectorAll('*').length,
    detachedPaneNodes: [...document.querySelectorAll('.pane')].filter((node) => !node.isConnected)
      .length,
    detachedEditors: [...document.querySelectorAll('.nocx-editor-input')].filter(
      (node) => !node.isConnected,
    ).length,
  }))
  const afterRetainedObjects = await retainedObjects()
  const artifacts = path.join(info.outputDir, 'renderer-memory')
  mkdirSync(artifacts, { recursive: true })
  const report: Record<string, unknown> = {
    browser: browserName,
    browserVersion: page.context().browser()?.version() ?? 'unknown',
    durationMs: Date.now() - startedAt,
    cycles: CYCLES,
    outputRoundsPerTab: OUTPUT_ROUNDS,
    tabs: expectedLiveCount,
    beforeDOM,
    afterDOM,
    beforeRetainedObjects,
    afterRetainedObjects,
    sample,
  }
  expect(
    afterRetainedObjects,
    'in-page retained-object observable remains available',
  ).not.toBeNull()
  expect(afterRetainedObjects).toEqual({
    panes: expectedLiveCount,
    terminalContents: expectedLiveCount,
  })
  expect(afterDOM.detachedPaneNodes).toBe(0)
  expect(afterDOM.detachedEditors).toBe(0)
  const nodeGrowth = afterDOM.nodes - beforeDOM.nodes
  expect(nodeGrowth, `DOM node count grew ${nodeGrowth}; report: ${artifacts}`).toBeLessThanOrEqual(
    RETAINED_DOM_GROWTH_LIMIT,
  )

  // Heap snapshots and GC metrics use Chromium DevTools Protocol. WKWebView has
  // no CDP, so keep its product-critical retained-object bound above in-page.
  if (browserName === 'chromium') {
    const cdp = await page.context().newCDPSession(page)
    const beforeSnapshot = await snapshot(page)
    const beforeRetained = snapshotObjectCounts(beforeSnapshot)
    writeFileSync(path.join(artifacts, 'before.heapsnapshot'), beforeSnapshot)
    const beforeMetrics = await metrics(page)
    const afterSnapshot = await snapshot(page)
    const afterRetained = snapshotObjectCounts(afterSnapshot)
    writeFileSync(path.join(artifacts, 'after.heapsnapshot'), afterSnapshot)
    const afterMetrics = await metrics(page)
    const settledMetrics = await metrics(page)
    const heapGrowth =
      (settledMetrics.JSHeapUsedSize ?? afterMetrics.JSHeapUsedSize ?? 0) -
      (beforeMetrics.JSHeapUsedSize ?? 0)
    report.beforeMetrics = beforeMetrics
    report.afterMetrics = afterMetrics
    report.beforeRetained = beforeRetained
    report.afterRetained = afterRetained
    report.heapGrowth = heapGrowth
    writeFileSync(path.join(artifacts, 'measurements.json'), JSON.stringify(report, null, 2))
    expect(
      heapGrowth,
      `retained JS heap grew ${heapGrowth} bytes; report: ${artifacts}`,
    ).toBeLessThanOrEqual(RETAINED_HEAP_GROWTH_LIMIT)
    expect(afterRetained.Pane ?? 0).toBeLessThanOrEqual((beforeRetained.Pane ?? 0) + 1)
    expect(afterRetained.TerminalContent ?? 0).toBeLessThanOrEqual(
      (beforeRetained.TerminalContent ?? 0) + 1,
    )
    await cdp.detach()
  } else {
    writeFileSync(path.join(artifacts, 'measurements.json'), JSON.stringify(report, null, 2))
  }
})
