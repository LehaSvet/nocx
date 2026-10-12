/** The active pane closed before an accepted frame could be captured. */
export class CaptureAbortedError extends Error {
  constructor() {
    super('frame capture aborted: no accepted screen frame is available')
    this.name = 'CaptureAbortedError'
  }
}
