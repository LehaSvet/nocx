/** Backend refusals are safe sentences; match only their stable text so a
 * wrapped path or platform errno never reaches the person. Detailed operation
 * failures use a remedy rather than leaking filesystem or verifier details. */
const REFUSALS = [
  'cannot install the update; allow writes to Applications and try again',
  'cannot find your home folder — check your account settings and try again',
  'the Applications folder is not writable — allow writes to Applications and try again',
  'the AppImage directory is not writable — move the AppImage to a directory you can write to, launch it there, and try again',
  'this is a development build; self-update is not available',
  'this is not a distributed AppImage build',
  'rollback: this was a first installation, with no previous target to restore',
  'the update install target is unavailable',
  'this window cannot quit the application',
  'state directory is not configured',
] as const

const FAILURES: readonly { pattern: RegExp; message: string }[] = [
  {
    pattern: /download|fetch/i,
    message: 'The update could not be downloaded. Check your connection and try again.',
  },
  {
    pattern: /verif|checksum|signature|digest/i,
    message: 'The update could not be verified. Check for updates and download it again.',
  },
  {
    pattern: /extract|unpack|archive/i,
    message: 'The update could not be unpacked. Check free space and try again.',
  },
  {
    pattern: /exchange|rename|permission|not writable|read.only/i,
    message:
      'The update could not replace the app. Allow writes to the app’s folder and try again.',
  },
  {
    pattern: /not installed|install target|app bundle/i,
    message:
      'This copy of nocx cannot be updated here. Install the latest release in a folder you can write to, then try again.',
  },
]

export function updateFailureMessage(error: unknown): string {
  const text = error instanceof Error ? error.message : String(error)
  const refusal = REFUSALS.find((message) => text.includes(message))
  if (refusal) return refusal
  return (
    FAILURES.find((row) => row.pattern.test(text))?.message ??
    'The update could not finish. Try again, or install the latest release manually.'
  )
}
