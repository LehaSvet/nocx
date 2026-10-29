/**
 * Is a failed update-state read worth telling a person about? (review,
 * nocx-1ebrs.66.1.1)
 *
 * The rule knows two things that a rejection alone does not say.
 *
 * AN EMBEDDING WITHOUT AN UPDATER HAS NOTHING TO READ. The web build, the e2e
 * stand and `make dev-web` run the renderer against a backend that carries no
 * updater at all (`cmd/nocx-server`), so `GetUpdateState` rejects on every
 * launch there. That is not a failure of anything — it is the shape of the
 * embedding — and a warning about it is noise the person can neither act on nor
 * dismiss for good. It also has a measured cost: it collided with every spec
 * that asserts on `.ui-toast__message`, which is how this rule came to exist.
 *
 * A DESKTOP BUILD THAT CANNOT READ ITS STATE HAS FAILED. The installed app has
 * an updater; if its state cannot be read, the offer will silently never
 * appear, and a soft degrade the UI contradicts is exactly what AGENTS.md says
 * must be visible in the product. There the person is told once, in words.
 *
 * `platform` is the repository's own fact for "is this the packaged app"
 * (`currentPlatform()` in platform.ts — the GOOS inside Wails, 'web' without
 * it), never a second derivation.
 */
import type { Platform } from './platform'

/**
 * Errors that mean "this build has no updater", not "the read failed".
 *
 * The backend says it in two ways: the window's own guard answers "the updater
 * is not available in this window", and a control plane with no such method
 * rejects with a method-not-found in a few spellings. Matching text is a
 * boundary compromise, so the list is short, explicit and paired with the
 * platform check — a desktop build whose read fails for any OTHER reason still
 * warns.
 */
const NO_UPDATER_REASONS = [
  'updater is not available',
  'not available in this window',
  'method not found',
  'unknown method',
  'no such method',
  'not supported',
] as const

/** Whether a failed read should be said out loud, once, to the person. */
export function shouldAnnounceUpdateStateFailure(reason: string, platform: Platform): boolean {
  // No Wails runtime: no updater exists in this embedding, so nothing failed.
  if (platform === 'web') return false
  const text = reason.toLowerCase()
  if (NO_UPDATER_REASONS.some((phrase) => text.includes(phrase))) return false
  return true
}
