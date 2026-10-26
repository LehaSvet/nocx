// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@solidjs/testing-library'
import { createUpdateController } from './update-controller'
import { ToastHost } from './ui/toast'
import { mountSidebar } from './sidebar'

const NOTES = 'https://example.com/releases/2.0.0'
afterEach(() => {
  cleanup()
  document.body.replaceChildren()
})

function mountIndicator(controller: ReturnType<typeof createUpdateController>) {
  const bar = document.createElement('div')
  const panel = document.createElement('div')
  document.body.append(bar, panel)
  const handle = mountSidebar(
    bar,
    panel,
    [],
    [{ id: 'settings', title: 'Settings', icon: () => <svg />, onActivate: vi.fn() }],
    null,
    undefined,
    undefined,
    undefined,
    undefined,
    {
      title: 'Update available',
      icon: () => <svg />,
      available: () => controller.state().kind !== 'hidden',
      onActivate: controller.showOffer,
    },
  )
  return { bar, panel, handle }
}

describe('shared update offer', () => {
  it('opens one offer beside Settings without taking tab space and applies once across sidebar rebuilds', async () => {
    let resolve!: () => void
    const apply = vi.fn(
      () =>
        new Promise<void>((r) => {
          resolve = r
        }),
    )
    const controller = createUpdateController({ apply })
    render(() => (
      <>
        <div id="tabbar" />
        <ToastHost />
      </>
    ))
    controller.showAvailable('2.0.0', NOTES)
    const first = mountIndicator(controller)
    fireEvent.click(screen.getByRole('button', { name: 'Update available' }))
    expect(screen.getByRole('status').textContent).toContain('2.0.0')
    expect(screen.getAllByRole('button', { name: /^Install$/ })).toHaveLength(1)
    expect(document.querySelector('#tabbar')?.textContent).toBe('')
    fireEvent.click(screen.getByRole('button', { name: /^Install$/ }))
    expect(apply).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('status').textContent).toContain('Downloading')
    first.handle.destroy()
    first.bar.remove()
    first.panel.remove()
    mountIndicator(controller)
    fireEvent.click(screen.getByRole('button', { name: 'Update available' }))
    controller.showAvailable('2.0.1', NOTES)
    await controller.apply()
    expect(apply).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('button', { name: /^Install$/ })).toBeNull()
    resolve()
    await vi.waitFor(() =>
      expect(screen.getByRole('status').textContent).toContain('2.0.0 installed'),
    )
  })
})
