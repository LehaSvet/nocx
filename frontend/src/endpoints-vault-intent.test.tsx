// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render } from '@solidjs/testing-library'
import { Show } from 'solid-js'
import { Dispatcher } from './dispatcher'
import { fixedEndpoint } from './endpoint'
import { EndpointClient } from './endpoints'
import { EndpointsSection } from './endpoints-section'
import { VaultClient } from './vault-client'
import { createVaultState, UnlockDialog } from './vault'

const UNSEALED = {
  state: 'unsealed',
  hasPassphrase: true,
  autoSealMinutes: 0,
  providers: [],
  defaultProvider: null,
}
const STORED = {
  id: 'endpoint:custom:provider:1',
  name: 'provider',
  baseUrl: 'https://api.example.com/v1',
  schema: 'openai-compatible',
  noKey: false,
  credential: 'secrow:0123456789abcdef',
  models: [],
  headers: [],
}

/** Only the socket is doubled. Replies are delivered explicitly, so the
 * seal can win after the cached unsealed status without relying on load. */
class Socket {
  static readonly OPEN = 1
  static current: Socket
  readyState = Socket.OPEN
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  requests: { id: number; method: string; params: unknown }[] = []
  constructor() {
    Socket.current = this
  }
  send(data: string) {
    this.requests.push(JSON.parse(data) as { id: number; method: string; params: unknown })
  }
  close() {
    this.readyState = 3
    this.onclose?.()
  }
  reply(method: string, result: unknown) {
    const request = [...this.requests].reverse().find((r) => r.method === method)
    if (!request) throw new Error(`No ${method} request`)
    this.deliver({ jsonrpc: '2.0', id: request.id, result })
  }
  sealedInventory() {
    const request = [...this.requests].reverse().find((r) => r.method === 'vault.inventory')
    if (!request) throw new Error('No inventory request')
    this.deliver({
      jsonrpc: '2.0',
      id: request.id,
      error: {
        code: -32001,
        message: 'vault.inventory: vault is sealed',
        data: { reason: 'vault-sealed' },
      },
    })
  }
  deliver(message: unknown) {
    this.onmessage?.({ data: JSON.stringify(message) })
  }
}

const flush = async () => {
  for (let i = 0; i < 8; i++) await Promise.resolve()
}
let dispatcher: Dispatcher | undefined

afterEach(() => {
  cleanup()
  dispatcher?.close()
  vi.unstubAllGlobals()
})

async function mountList() {
  vi.stubGlobal('WebSocket', Socket)
  const d = new Dispatcher(fixedEndpoint(9876))
  dispatcher = d
  const vault = new VaultClient(d)
  const ctrl = createVaultState(vault)
  d.start()
  await flush()
  const socket = Socket.current
  socket.onopen?.()
  const refresh = ctrl.refresh()
  socket.reply('vault.status', UNSEALED)
  await refresh
  // The same sequence as the failure frames: status says unsealed; seal
  // is sent; the endpoints list mounts before the sealed status arrives.
  const seal = vault.seal()
  const view = render(() => (
    <>
      <EndpointsSection client={new EndpointClient(d)} vaultClient={vault} vaultController={ctrl} />
      <Show when={ctrl.showUnlock()}>
        <UnlockDialog
          open
          onClose={() => ctrl.closeUnlock()}
          onUnsealed={() => ctrl.onUnsealDone()}
          vaultClient={vault}
          vaultStatus={ctrl.status()}
        />
      </Show>
    </>
  ))
  socket.reply('endpoints.list', [STORED])
  socket.reply('vault.seal', {})
  await seal
  await flush()
  expect(socket.requests.filter((r) => r.method === 'vault.inventory')).toHaveLength(1)
  return { socket, ctrl, ...view }
}

describe('endpoint inventory intent when a seal wins the cached-status race', () => {
  it('keeps the passive list interactive with no unlock sheet', async () => {
    const { socket, ctrl, container } = await mountList()
    socket.sealedInventory()
    await flush()
    expect(ctrl.showUnlock()).toBe(false)
    expect(document.querySelector('.ui-prompt-overlay')).toBeNull()
    // The seal notification can arrive later. The list then reports the
    // locked state, not a deleted credential or an unavailable store.
    const refresh = ctrl.refresh()
    socket.reply('vault.status', { ...UNSEALED, state: 'sealed' })
    await refresh
    await flush()
    expect(container.textContent).toContain('The vault is locked')
    expect(document.querySelector('.ui-prompt-overlay')).toBeNull()
    expect(container.querySelector('[aria-label="Edit provider"]')).not.toBeNull()
  })

  it('still asks to unlock when the person opens a bound-key editor', async () => {
    const { socket, ctrl, container } = await mountList()
    socket.reply('vault.inventory', { entries: [] })
    await flush()
    const refresh = ctrl.refresh()
    socket.reply('vault.status', { ...UNSEALED, state: 'sealed' })
    await refresh
    await flush()
    fireEvent.click(container.querySelector('[aria-label="Edit provider"]')!)
    socket.sealedInventory()
    await flush()
    expect(ctrl.showUnlock()).toBe(true)
    expect(document.querySelector('.ui-prompt-overlay')?.getAttribute('data-prompt')).toBe(
      'Unlock the vault',
    )
    fireEvent.click(
      Array.from(document.querySelectorAll('button')).find(
        (button) => button.textContent === 'Cancel',
      )!,
    )
    await flush()
    expect(document.querySelector('.ui-prompt-overlay')).toBeNull()
  })
})
