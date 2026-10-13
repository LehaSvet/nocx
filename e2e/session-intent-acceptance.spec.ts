import { clickIntoEditor, expect, promptReady, test } from './harness'
import {
  beginClipboardWriteCaptureForTest,
  recordClipboardWriteForTest,
  type ClipboardWriteObservation,
} from './failure-context'

type EffectRecord = {
  effects: number
  effectPayloads: Array<{ kind?: string; title?: string; body?: string }>
  clipboardWriteEffects: ClipboardWriteObservation[]
  activeClipboardWrite?: ClipboardWriteObservation | null
  pendingClipboardWrite?: ClipboardWriteObservation | null
  replayedFrames: number
  raises: Array<{ title: string; body: string }>
  clipboardWrites: string[]
  replayNotification?: () => void
  replayClipboard?: () => void
  replayFrame?: () => void
  seedClipboard?: (text: string) => Promise<void>
}

declare global {
  interface Window {
    __intentAcceptance?: EffectRecord
  }
}

const GRID = '.pane.active .terminal-live-container'
const INTENT_INPUT = '.pane.active .ui-terminal-input'

async function command(page: import('./harness').Page, source: string): Promise<void> {
  await promptReady(page)
  await clickIntoEditor(page)
  await page.keyboard.type(source)
  await page.keyboard.press('Enter')
}

async function focusLiveGrid(page: import('./harness').Page): Promise<void> {
  // A person clicks the visible terminal. Its own focus path gives the real
  // input element the keyboard; the test never calls the intent encoder.
  await page.locator(GRID).click()
  await expect(page.locator(INTENT_INPUT)).toBeFocused()
}

async function outputContains(page: import('./harness').Page, text: string): Promise<void> {
  await expect(page.locator(GRID)).toContainText(text, { timeout: 15_000 })
}

// These are true browser events on the pane's mounted input element. The
// runtime sees their committed text only through the ordinary WebSocket
// session.intent route. Chromium has no hardware IME, so its native event
// sequence is represented explicitly, including the provisional input event.
async function compose(page: import('./harness').Page, committed: string): Promise<void> {
  await page.locator(INTENT_INPUT).evaluate((node, value) => {
    const input = node as HTMLTextAreaElement
    input.dispatchEvent(new CompositionEvent('compositionstart', { data: '', bubbles: true }))
    input.dispatchEvent(new CompositionEvent('compositionupdate', { data: value, bubbles: true }))
    input.dispatchEvent(
      new InputEvent('input', {
        inputType: 'insertCompositionText',
        data: value,
        isComposing: true,
        bubbles: true,
      }),
    )
    input.dispatchEvent(new CompositionEvent('compositionend', { data: value, bubbles: true }))
  }, committed)
}

async function cancelComposition(
  page: import('./harness').Page,
  provisional: string,
): Promise<void> {
  await page.locator(INTENT_INPUT).evaluate((node, value) => {
    const input = node as HTMLTextAreaElement
    input.dispatchEvent(new CompositionEvent('compositionstart', { data: '', bubbles: true }))
    input.dispatchEvent(new CompositionEvent('compositionupdate', { data: value, bubbles: true }))
    input.dispatchEvent(
      new InputEvent('input', {
        inputType: 'insertCompositionText',
        data: value,
        isComposing: true,
        bubbles: true,
      }),
    )
    input.dispatchEvent(new CompositionEvent('compositionend', { data: '', bubbles: true }))
  }, provisional)
}

async function recordAndReplayEffects(
  page: import('./harness').Page,
  testId: string,
): Promise<void> {
  beginClipboardWriteCaptureForTest(testId)
  await page.addInitScript(() => {
    const state: EffectRecord = {
      effects: 0,
      effectPayloads: [],
      clipboardWriteEffects: [],
      activeClipboardWrite: null,
      pendingClipboardWrite: null,
      replayedFrames: 0,
      raises: [],
      clipboardWrites: [],
    }
    window.__intentAcceptance = state
    // This spec checks effect dispatch and replay deduplication, not browser
    // permission UI or the OS clipboard. Stub the browser API at its seam so
    // Chromium and WebKit observe the same write calls.
    const nativeClipboard = navigator.clipboard
    state.seedClipboard = async (text: string) => {
      if (!nativeClipboard) throw new Error('browser clipboard API is unavailable')
      await nativeClipboard.writeText(text)
    }
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {
        readText: async () => (nativeClipboard ? nativeClipboard.readText() : ''),
        writeText: async (text: string) => {
          const observation: ClipboardWriteObservation = state.activeClipboardWrite ??
            state.pendingClipboardWrite ?? {
              generation: null,
              effectId: null,
              source: 'unattributed',
            }
          if (!state.activeClipboardWrite) state.pendingClipboardWrite = null
          state.clipboardWriteEffects.push(observation)
          state.clipboardWrites.push(text)
        },
      },
    })

    // Duplicate each arriving metadata screen frame after its first delivery.
    // Browser frame byte 1 == 0x02 is MSG_TYPE_METADATA, not helper
    // TypeSessionEffect (0x12, on the helper connection). Effects reach this
    // page as JSON-RPC session.effect and ipc.ts deduplicates their identities.
    // Capture real effect events for explicit replay after clipboard permission
    // is granted; this does not replay effects automatically.
    const descriptor = Object.getOwnPropertyDescriptor(WebSocket.prototype, 'onmessage')
    if (descriptor?.get && descriptor.set) {
      Object.defineProperty(WebSocket.prototype, 'onmessage', {
        configurable: true,
        enumerable: descriptor.enumerable,
        get: descriptor.get,
        set(handler: ((event: MessageEvent) => void) | null) {
          if (typeof handler !== 'function') {
            descriptor.set!.call(this, handler)
            return
          }
          descriptor.set!.call(this, function (this: WebSocket, event: MessageEvent) {
            handler.call(this, event)
            if (event.data instanceof ArrayBuffer && new Uint8Array(event.data)[1] === 2) {
              state.replayFrame = () => handler.call(this, event)
              state.replayedFrames++
              handler.call(this, event)
            }
          })
        },
      })
    }
    const add = WebSocket.prototype.addEventListener
    WebSocket.prototype.addEventListener = function (
      type: string,
      listener: EventListenerOrEventListenerObject | null,
      options?: boolean | AddEventListenerOptions,
    ) {
      if (type !== 'message' || typeof listener !== 'function')
        return Reflect.apply(add, this, [type, listener, options])
      return add.call(
        this,
        type,
        function (this: WebSocket, event: Event) {
          const message = event as MessageEvent
          let liveClipboardWrite: ClipboardWriteObservation | null = null
          if (message.data instanceof ArrayBuffer && new Uint8Array(message.data)[1] === 2) {
            state.replayFrame = () => listener.call(this, event)
          }
          if (typeof message.data === 'string') {
            try {
              const frame = JSON.parse(message.data) as {
                method?: string
                params?: {
                  kind?: string
                  title?: string
                  body?: string
                  generation?: string
                  effectId?: string
                }
              }
              if (frame.method === 'session.effect') {
                state.effects++
                if (frame.params && typeof frame.params === 'object') {
                  const params = frame.params
                  state.effectPayloads.push({
                    kind: params.kind,
                    title: params.title,
                    body: params.body,
                  })
                  if (
                    params.kind === 'clipboard' &&
                    typeof params.generation === 'string' &&
                    typeof params.effectId === 'string'
                  ) {
                    const { generation, effectId } = params
                    liveClipboardWrite = { generation, effectId, source: 'live-effect' }
                    if (!state.replayClipboard) {
                      state.replayClipboard = () => {
                        const previous = state.activeClipboardWrite
                        state.activeClipboardWrite = {
                          generation,
                          effectId,
                          source: 'explicit-replay',
                        }
                        try {
                          listener.call(this, event)
                        } finally {
                          state.activeClipboardWrite = previous
                        }
                      }
                    }
                  }
                  if (params.kind === 'notification' && !state.replayNotification)
                    state.replayNotification = () => listener.call(this, event)
                }
              }
            } catch {
              /* non-JSON control data is not an effect */
            }
          }
          const hadClipboardBanner = document.querySelector('.clipboard-banner') !== null
          const previous = state.activeClipboardWrite
          if (liveClipboardWrite) state.activeClipboardWrite = liveClipboardWrite
          try {
            listener.call(this, event)
          } finally {
            state.activeClipboardWrite = previous
          }
          if (
            liveClipboardWrite &&
            !hadClipboardBanner &&
            document.querySelector('.clipboard-banner') !== null
          ) {
            state.pendingClipboardWrite = liveClipboardWrite
          }
        },
        options,
      )
    }

    const send = WebSocket.prototype.send
    WebSocket.prototype.send = function (
      this: WebSocket,
      data: string | ArrayBufferLike | Blob | ArrayBufferView,
    ) {
      if (typeof data === 'string') {
        try {
          const msg = JSON.parse(data) as {
            method?: string
            params?: { title?: string; body?: string }
          }
          if (msg.method === 'notify.raise' && msg.params)
            state.raises.push({ title: msg.params.title ?? '', body: msg.params.body ?? '' })
        } catch {
          /* binary and unrelated control traffic */
        }
      }
      return Reflect.apply(send, this, [data])
    }
  })
}

async function captureClipboardWritesForFailure(
  page: import('./harness').Page,
  testId: string,
  recorded: { count: number },
): Promise<{ clipboardWrites: number; notifications: number }> {
  const snapshot = await page.evaluate(() => ({
    clipboardWrites: window.__intentAcceptance?.clipboardWrites.length ?? 0,
    notifications: window.__intentAcceptance?.raises.length ?? 0,
    observations: window.__intentAcceptance?.clipboardWriteEffects ?? [],
  }))
  for (const observation of snapshot.observations.slice(recorded.count))
    recordClipboardWriteForTest(testId, observation)
  recorded.count = snapshot.observations.length
  return { clipboardWrites: snapshot.clipboardWrites, notifications: snapshot.notifications }
}

test('browser input crosses the session transport once and replayed effects stay once', async ({
  page,
  browserName,
}, testInfo) => {
  // Playwright exposes native clipboard permissions only in Chromium. The
  // runtime effect API remains stubbed below in both browsers.
  if (browserName === 'chromium')
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await recordAndReplayEffects(page, testInfo.testId)
  await page.goto('/')
  await promptReady(page)

  // The same physical Up key is encoded by the session runtime according to
  // the mode set by the running program. The browser sends only a key intent.
  const appMode = '1b 4f 41'
  await command(
    page,
    "printf '\\033[?1h'; stty -echo -icanon; dd bs=1 count=3 2>/dev/null | od -An -tx1; stty sane",
  )
  await focusLiveGrid(page)
  await page.keyboard.press('ArrowUp')
  await outputContains(page, appMode)
  await promptReady(page)

  const normalMode = '1b 5b 41'
  await command(
    page,
    "printf '\\033[?1l'; stty -echo -icanon; dd bs=1 count=3 2>/dev/null | od -An -tx1; stty sane",
  )
  await focusLiveGrid(page)
  await page.keyboard.press('ArrowUp')
  await outputContains(page, normalMode)
  await promptReady(page)

  if (browserName === 'chromium') {
    // Paste through the focused terminal input while the running program has
    // enabled bracketed-paste mode. The program reads and prints the bytes it
    // receives, so this checks the browser-to-helper route and the framing.
    const pasteText = 'BROWSER-PASTE'
    const pasteBytes = '1b 5b 32 30 30 7e 42 52 4f 57 53 45 52 2d 50 41 53 54 45 1b 5b 32 30 31 7e'
    await command(
      page,
      "printf '\\033[?2004h'; stty -echo -icanon; dd bs=1 count=25 2>/dev/null | od -An -tx1; stty sane",
    )
    await focusLiveGrid(page)
    await page.evaluate(async (text) => {
      const seed = window.__intentAcceptance?.seedClipboard
      if (!seed) throw new Error('test clipboard seeding hook is unavailable')
      await seed(text)
    }, pasteText)
    await page.keyboard.press('Control+V')
    await outputContains(page, pasteBytes)
    await promptReady(page)
  }

  const committed = 'IME-COMMIT-世界'
  await command(page, 'IFS= read -r value; printf \'IME-RESULT=<%s>\\n\' "$value"')
  await focusLiveGrid(page)
  await compose(page, committed)
  await focusLiveGrid(page)
  await page.keyboard.press('Enter')
  await outputContains(page, `IME-RESULT=<${committed}>`)
  await expect(page.locator(GRID)).toContainText(`IME-RESULT=<${committed}>`)
  await promptReady(page)

  const cancelled = 'IME-CANCEL-MUST-NOT-ARRIVE'
  await command(page, 'IFS= read -r value; printf \'CANCEL-RESULT=<%s>\\n\' "$value"')
  await focusLiveGrid(page)
  await cancelComposition(page, cancelled)
  await focusLiveGrid(page)
  await page.keyboard.type('survivor')
  await page.keyboard.press('Enter')
  await outputContains(page, 'CANCEL-RESULT=<survivor>')
  await expect(page.locator(GRID)).not.toContainText(`CANCEL-RESULT=<${cancelled}>`)
  await promptReady(page)

  // A delivered OSC 777 notification takes its ordinary runtime-effect
  // route with the exact ADR-0047 title/body pair. The init hook repeats the
  // identical full frame/effect at the WebSocket seam; it must still raise
  // exactly once.
  const title = `replay-${Date.now().toString(36)}`
  const body = `message-${title}`
  await command(page, `printf '\\033]777;notify;${title};${body}\\007'`)
  await expect
    .poll(() => page.evaluate(() => window.__intentAcceptance?.raises.length ?? 0))
    .toBe(1)
  await expect
    .poll(() => page.evaluate(() => window.__intentAcceptance?.effects ?? 0))
    .toBeGreaterThan(0)
  expect(await page.evaluate(() => window.__intentAcceptance?.raises)).toEqual([{ title, body }])
  expect(
    await page.evaluate(() =>
      window.__intentAcceptance?.effectPayloads.filter((effect) => effect.kind === 'notification'),
    ),
  ).toEqual([{ kind: 'notification', title, body }])

  // OSC 52 is also an identity-bearing effect. Granting browser permission
  // does not bypass nocx's own one-time gate; accept its visible banner, then
  // observe that replay of the same WebSocket event writes the text only once.
  const clipboardText = `clipboard-${title}`
  const encodedClipboard = Buffer.from(clipboardText).toString('base64')
  await command(page, `printf '\\033]52;c;${encodedClipboard}\\007'`)
  const clipboardBanner = page.locator('.clipboard-banner')
  await expect(clipboardBanner).toBeVisible()
  await clipboardBanner.getByRole('button', { name: 'Allow clipboard writes' }).click()
  const recordedClipboardWrites = { count: 0 }
  await expect
    .poll(
      async () =>
        (await captureClipboardWritesForFailure(page, testInfo.testId, recordedClipboardWrites))
          .clipboardWrites,
    )
    .toBe(1)
  expect(await page.evaluate(() => window.__intentAcceptance?.clipboardWrites)).toEqual([
    clipboardText,
  ])
  const clipboardWriteEffects = await page.evaluate(
    () => window.__intentAcceptance?.clipboardWriteEffects ?? [],
  )
  expect(clipboardWriteEffects).toHaveLength(1)
  expect(clipboardWriteEffects[0]?.source).toBe('live-effect')
  expect(clipboardWriteEffects[0]?.generation).toMatch(/^[1-9][0-9]*$/)
  expect(clipboardWriteEffects[0]?.effectId).toMatch(/^[1-9][0-9]*$/)

  // Replay the captured real identity-bearing notifications only after the
  // clipboard permission was granted and the first write was observed. Also
  // replay a captured runtime frame through the same WebSocket listener.
  const countsBeforeReplay = await page.evaluate(() => ({
    clipboardWrites: window.__intentAcceptance?.clipboardWrites.length ?? 0,
    notifications: window.__intentAcceptance?.raises.length ?? 0,
  }))
  await page.evaluate(() => {
    const state = window.__intentAcceptance
    if (!state?.replayClipboard || !state.replayNotification || !state.replayFrame)
      throw new Error(
        'real clipboard, notification, and runtime-frame deliveries were not captured',
      )
    state.replayClipboard()
    state.replayNotification()
    state.replayFrame()
  })
  await expect
    .poll(() => captureClipboardWritesForFailure(page, testInfo.testId, recordedClipboardWrites))
    .toEqual(countsBeforeReplay)

  await expect
    .poll(() => page.evaluate(() => window.__intentAcceptance?.replayedFrames ?? 0))
    .toBeGreaterThan(0)
})
