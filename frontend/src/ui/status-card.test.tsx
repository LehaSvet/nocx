// @vitest-environment jsdom
import { describe, expect, it, afterEach } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { render, screen, cleanup, fireEvent } from '@solidjs/testing-library'
import { StatusCard, type StatusCardProps } from './status-card'
import { Button } from './button'

afterEach(() => cleanup())

function subject(overrides?: Partial<StatusCardProps>) {
  const props: StatusCardProps = {
    title: 'Vault is locked',
    ...overrides,
  }
  return render(() => <StatusCard {...props} />)
}

describe('StatusCard', () => {
  it('carries its identity class', () => {
    subject()
    expect(document.querySelector('.ui-status-card')).not.toBeNull()
  })

  it('states the title', () => {
    subject()
    expect(screen.getByText('Vault is locked')).toBeTruthy()
  })

  it('defaults to the neutral tone', () => {
    subject()
    expect(document.querySelector('.ui-status-card')!.getAttribute('data-tone')).toBe('neutral')
  })

  it('carries the tone it is given', () => {
    subject({ tone: 'warning' })
    expect(document.querySelector('.ui-status-card')!.getAttribute('data-tone')).toBe('warning')
  })

  it('omits the description element when there is no description', () => {
    subject()
    expect(document.querySelector('.ui-status-card__desc')).toBeNull()
  })

  it('states the description when given one', () => {
    subject({ description: 'Unlock to use saved passwords.' })
    expect(screen.getByText('Unlock to use saved passwords.')).toBeTruthy()
  })

  it('omits the icon slot entirely when no icon is given, so nothing indents around empty space', () => {
    subject()
    expect(document.querySelector('.ui-status-card__icon')).toBeNull()
  })

  it('renders the icon it is given', () => {
    subject({ icon: <svg data-testid="glyph" /> })
    expect(document.querySelector('.ui-status-card__icon svg')).not.toBeNull()
  })

  // VERBATIM TEXT KEEPS ITS SHAPE. `prose` exists for a description that is
  // somebody else's multi-paragraph writing rather than one authored
  // sentence — the skill audit's model-written report — and the defect it
  // prevents is the component quietly reformatting the thing it was asked to
  // present as written. The attribute is asserted rather than the computed
  // style, because jsdom does not apply the stylesheet and the variance IS
  // the attribute.
  it('marks a description as verbatim prose so its blank lines survive', () => {
    subject({ description: 'first\n\nsecond', prose: true })
    const card = document.querySelector('.ui-status-card')!
    expect(card.getAttribute('data-prose')).toBe('')
    expect(document.querySelector('.ui-status-card__desc')?.textContent).toBe('first\n\nsecond')
  })

  it('carries no prose attribute by default, so an authored sentence is set as one', () => {
    subject({ description: 'one sentence' })
    expect(document.querySelector('.ui-status-card')!.hasAttribute('data-prose')).toBe(false)
  })

  // The card exists to pair a state with the one thing to do about it. A card
  // whose action does not reach its handler announces a problem and offers a
  // remedy that does nothing — worse than announcing nothing.
  it('the action it renders is clickable and reaches its handler', () => {
    let clicked = 0
    subject({
      action: (
        <Button variant="primary" onClick={() => (clicked += 1)}>
          Unlock
        </Button>
      ),
    })
    const button = screen.getByRole('button', { name: 'Unlock' })
    expect((button as HTMLButtonElement).disabled).toBe(false)
    fireEvent.click(button)
    expect(clicked).toBe(1)
  })

  // A BAND, NOT A CARD (nocx-xn63t.6.36). The third state's notice is a strip
  // of the pane it belongs to, and the card's own air below it — which exists
  // because the next thing after a card is unknown — is exactly the empty band
  // the owner photographed above the pane's content. The variance is ONE
  // attribute and the stylesheet answers it: a band is full-bleed and owns no
  // air on the side it is attached by. Asserted on the rule itself because
  // jsdom does not apply the stylesheet, and the property under test IS the
  // rule (the measured geometry is e2e/unreconciled-notice-band.spec.ts's).
  it('is a card by default, so a surface that places one keeps the card', () => {
    subject()
    expect(document.querySelector('.ui-status-card')!.hasAttribute('data-band')).toBe(false)
  })

  it('carries the band variance when it is asked for', () => {
    subject({ band: true })
    expect(document.querySelector('.ui-status-card')!.getAttribute('data-band')).toBe('')
  })

  it('the band rule gives up the rounded box and the air below it', () => {
    const css = readFileSync(
      resolve(import.meta.dirname ?? '.', '../styles/components/status-card.css'),
      'utf8',
    ).replace(/\/\*[\s\S]*?\*\//g, '')
    const rule = /\.ui-status-card\[data-band\]\s*\{([^}]*)\}/.exec(css)
    expect(rule, 'the band variance has no rule of its own').not.toBeNull()
    const body = rule![1]
    expect(body).toMatch(/margin-bottom:\s*0/)
    expect(body).toMatch(/border-radius:\s*0/)
    // The tone's leading edge is the one part of the frame that carries a
    // fact, so the band gives up the other three sides and never the left: a
    // `border:` shorthand here would drop the colour the tone rules set.
    expect(body).not.toMatch(/(^|[^-])border\s*:/)
  })
})
