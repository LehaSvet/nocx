// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render } from '@solidjs/testing-library'
import { createSignal, type Component } from 'solid-js'
import { mountSidebar, type SidebarAction, type SidebarViewDescriptor } from './sidebar'
import { createUpdateToast } from './update-toast'
import { ToastHost } from './ui/toast'

const TestIcon: Component = () => <svg data-test-icon="1" />
const TestView: Component = () => <div>Updates test view</div>
const settings: SidebarAction = {
  id: 'settings',
  title: 'Settings',
  icon: TestIcon,
  onActivate: vi.fn(),
}
const views: SidebarViewDescriptor[] = [
  { id: 'test', title: 'Test', icon: TestIcon, view: TestView, order: 0 },
]

afterEach(() => {
  cleanup()
  document.body.replaceChildren()
})

describe('activity bar update indicator', () => {
  it('shows an available update beside Settings and exposes one Install control when activated', () => {
    const bar = document.createElement('div')
    const panel = document.createElement('div')
    document.body.append(bar, panel)
    const [available] = createSignal(true)
    const toast = createUpdateToast({ apply: vi.fn() })
    render(() => <ToastHost />)
    mountSidebar(bar, panel, views, [settings], null, undefined, undefined, undefined, undefined, {
      title: 'Update available',
      icon: TestIcon,
      available,
      onActivate: () => toast.showAvailable('2.0.0', 'https://example.com/releases/2.0.0'),
    })

    const bottomZone = bar.querySelector('[aria-label="Actions"]')
    const indicator = bottomZone?.querySelector<HTMLButtonElement>('[data-update-indicator]')
    const settingsButton = bottomZone?.querySelector<HTMLButtonElement>('[data-action="settings"]')
    expect(indicator).not.toBeNull()
    expect(settingsButton).not.toBeNull()
    expect(
      settingsButton!.compareDocumentPosition(indicator!) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()

    fireEvent.click(indicator!)
    expect(document.body.querySelectorAll('button').length).toBeGreaterThan(2)
    expect(
      [...document.body.querySelectorAll('button')].filter(
        (button) => button.textContent?.trim() === 'Install now',
      ),
    ).toHaveLength(1)
  })
})
