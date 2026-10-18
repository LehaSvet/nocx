/**
 * A double-click on a word in a command block's output selects that word and
 * keeps it (nocx-zg3k3.15.6).
 *
 * THE PERSON'S GESTURE, MEASURED. The owner double-clicked a word in frozen
 * command output and watched the whole block light up and the selection go.
 * The word is the thing the gesture is for.
 *
 * Two surfaces answer one press-and-release sequence here, which is the defect
 * AGENTS.md names: the block's own click-to-select paints the block (P1-7),
 * and the block's own double-click rule selects the token under the pointer
 * (nocx-w7h.11, word-selection.ts). This spec watches the whole gesture as a
 * person makes it, and asserts what a person can SEE and then copy: exactly the
 * word, still on screen after the release, and on the clipboard.
 *
 * The coordinates come from the word's own range — the text a person aims at —
 * so the spec never depends on a cell metric. Nothing here waits on a
 * duration: every wait is on a state the DOM or the clipboard shows.
 */
import { test, expect, promptReady } from './harness'
import type { Page } from './harness'

/** The frozen block's output body (its rows are the read-only text). */
const OUTPUT = '.pane.active .cmd-block .cmd-output'

/** The clipboard needs the browser's, not the Wails runtime's (clipboard.ts). */
async function disableWailsRuntime(page: Page) {
  await page.addInitScript(() => {
    Object.defineProperty(window, 'runtime', {
      get() {
        return undefined
      },
      set(_value: unknown) {
        void _value
      },
      configurable: true,
      enumerable: true,
    })
  })
}

/** Run `echo <word> tail` and leave its frozen block on screen, clear of the
 *  composer. Returns the block's output element. */
async function frozenOutputWithWord(page: Page, word: string) {
  await page.keyboard.type(`echo ${word} tail`)
  await page.keyboard.press('Enter')
  const output = page.locator(OUTPUT, { hasText: word }).first()
  await expect(output).toBeVisible({ timeout: 10_000 })
  await promptReady(page)
  // A second command moves the marker's block up, clear of the composer.
  await page.keyboard.type('echo spacer')
  await page.keyboard.press('Enter')
  await expect(page.locator(OUTPUT, { hasText: 'spacer' }).first()).toBeVisible()
  await promptReady(page)
  await output.scrollIntoViewIfNeeded()
  return output
}

/** The centre of the word, measured from the word's own range. */
async function centreOfWord(output: import('@playwright/test').Locator, word: string) {
  const at = await output.evaluate((el, w) => {
    const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT)
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      const i = n.textContent?.indexOf(w) ?? -1
      if (i < 0) continue
      const range = document.createRange()
      range.setStart(n, i)
      range.setEnd(n, i + w.length)
      const box = range.getBoundingClientRect()
      return { x: box.left + box.width / 2, y: box.top + box.height / 2, width: box.width }
    }
    return null
  }, word)
  if (at === null) throw new Error('the word is not in the block output')
  return at
}

/** What a person can see of the selection right now: its text, and whether it
 *  is painted (a non-empty rect). */
async function paintedSelection(page: Page) {
  return page.evaluate(() => {
    const sel = window.getSelection()
    if (!sel || sel.rangeCount === 0) return { text: '', painted: false }
    const rects = [...sel.getRangeAt(0).getClientRects()]
    return {
      text: sel.toString(),
      painted: rects.some((r) => r.width > 0 && r.height > 0),
    }
  })
}

test.describe('a double click on a word in a command block', () => {
  test('selects that word, keeps it after the release, and a plain click still selects the block', async ({
    page,
  }) => {
    await page.goto('/')
    await expect(page.locator('.nocx-tab')).toHaveCount(1)
    await promptReady(page)

    const word = `DBL-${Date.now().toString(36)}`
    const output = await frozenOutputWithWord(page, word)
    const at = await centreOfWord(output, word)

    await page.mouse.dblclick(at.x, at.y)

    // EXACTLY THE WORD, and not the whole block. The block's own click-select
    // paints the whole block; a double click is a selection of the word under
    // the pointer, so neither the selection nor the block carries the other.
    const selected = await paintedSelection(page)
    expect(selected.text, 'the double click selected exactly the word').toBe(word)
    expect(selected.painted, 'the selection is painted, not merely recorded').toBe(true)
    await expect(
      page.locator('.pane.active .cmd-block', { hasText: word }).first(),
      'a double click does not select the whole block',
    ).not.toHaveClass(/cmd-block-selected/)

    // AND IT IS STILL THERE AFTER THE RELEASE. The dblclick above ends with the
    // mouseup, and this reads the same two facts again after the pointer has
    // moved off the word — a state the pane publishes, not an elapsed time. The
    // release is what used to take the selection away.
    await page.mouse.move(at.x, at.y - 40)
    const after = await paintedSelection(page)
    expect(after.text, 'the selection survives the release').toBe(word)
    expect(after.painted, 'and is still on screen').toBe(true)

    // AND A PLAIN CLICK IS STILL A PLAIN CLICK: one press-and-release over the
    // block selects the block, as it always did (P1-7).
    const block = page.locator('.pane.active .cmd-block', { hasText: word }).first()
    await page.mouse.click(at.x, at.y)
    await expect(block).toHaveClass(/cmd-block-selected/)
  })

  // Chromium reads the clipboard under Playwright; WebKit grants neither
  // clipboard-read nor clipboard-write (clipboard.spec.ts's own note).
  test.describe('copy-on-select', () => {
    test.skip(
      ({ browserName }) => browserName !== 'chromium',
      'clipboard-read permission is Chromium-only; WebKit must be checked manually',
    )

    test('the double click copies exactly the word', async ({ page }) => {
      await disableWailsRuntime(page)
      await page.goto('/')
      await expect(page.locator('.nocx-tab')).toHaveCount(1)
      await promptReady(page)
      await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])

      const word = `DBLCOPY-${Date.now().toString(36)}`
      const output = await frozenOutputWithWord(page, word)
      const at = await centreOfWord(output, word)

      await page.mouse.dblclick(at.x, at.y)
      await expect
        .poll(() => page.evaluate(() => navigator.clipboard.readText()), { timeout: 5_000 })
        .toBe(word)
    })
  })
})
