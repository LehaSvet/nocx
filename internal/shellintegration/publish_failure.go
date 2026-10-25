package shellintegration

import "errors"

// Why the integration bundle could not be written to a host — in a form the
// product renders (nocx-1kejz.4).
//
// # The defect this vocabulary exists for
//
// A publish that fails is, by design, not a refusal: the far side keeps any
// generation it already had, byte-identical, and the pane opens as an ordinary
// terminal (design §6.1 step 5, ADR-0004). What it was NOT was visible. The
// failure reached one slog line and stopped there, so a person got a pane that
// behaved as though delivery had been configured — while nocx knew, at that
// moment and in its own words, that the copy had not happened.
//
// # Why this is not a RefusalReason, and not an Outcome
//
// shellintegration.RefusalReason is the LAUNCHER's answer to "may this shell be
// integrated at all", decided locally before anything crosses a wire.
// ssh.RefusalReason is the product's answer to "did integration happen, and
// why not", and every one of its members is a FACT ABOUT THE SESSION — a fact
// this failure has not settled, and may never settle in this direction: a host
// that already carries a generation integrates from it happily after a failed
// publish, and a host that carries none goes conventional a hello-timeout
// later. Reporting this failure through either vocabulary would be the session
// axis answering a question it was not asked, which is what nocx-dvql removed
// from the open ack.
//
// So it is its own closed set, and it is small on purpose: nocx-viil settled
// the granularity — a reason a person can act on, never the raw error. A
// coordinator-side publish failure is an SFTP status code and a path, which is
// meaningful in a log and meaningless in a pane, so the raw chain stays in the
// log and this set is what the pane may say.
type PublishFailureReason string

const (
	// PublishFailureNone is "nothing to report": no publish was attempted (a
	// raw pane, a mode that delivers no scripts, an injected installer that
	// answered nothing) or the copy succeeded. It is never a wire value; the
	// field is absent instead, the same rule ReasonNone carries.
	PublishFailureNone PublishFailureReason = ""
	// PublishFailureConnection: the connection the copy needed did not come up
	// or ended while the bundle was being written. Nothing arrived that says
	// the far side refused anything — a transient as far as this side can tell.
	PublishFailureConnection PublishFailureReason = "connection-failed"
	// PublishFailureHome: the far side did not report a home directory for the
	// account, or would not answer the question at all, so nocx had nowhere to
	// write. A bundle put into a directory nobody named lands where no session
	// will ever read it, which is why the route refuses instead of guessing.
	PublishFailureHome PublishFailureReason = "home-unavailable"
	// PublishFailureFileTransfer: the far side would not open (or would not
	// speak) the file-transfer channel the copy rides. A server with the sftp
	// subsystem switched off, or an intermediary that permits interactive
	// channels only, is this row — and the row a person can act on is theirs,
	// not nocx's.
	PublishFailureFileTransfer PublishFailureReason = "file-transfer-unavailable"
	// PublishFailureWrite: the copy was attempted and the far side would not
	// take it — a home directory that refuses writes, a path nocx will not
	// overwrite, a bundle that could not be committed.
	PublishFailureWrite PublishFailureReason = "write-refused"
	// PublishFailureUnknown: the copy failed and this build cannot say why.
	// A real, visible answer rather than a synonym for success — the same rule
	// ssh.ReasonUnknown carries, and the fallback for a cause added elsewhere
	// and not yet named here.
	PublishFailureUnknown PublishFailureReason = "unknown"
)

// PublishFailure is a publish cause, TYPED at the step that knows it.
//
// The step is the whole point: "the sftp channel was refused" and "the write
// was refused" are decided in different places by different code, and a
// classifier that read the finished error's text to tell them apart would
// break the first time somebody reworded a message.
type PublishFailure struct {
	// Reason is one of the members above. Never PublishFailureNone: a cause
	// with no reason is not a failure this type can describe.
	Reason PublishFailureReason
	// Cause is the underlying error, kept for the log. It is NOT what the
	// product shows (nocx-viil): an SFTP status code and a path are meaningful
	// in a log and meaningless in a pane.
	Cause error
}

func (e *PublishFailure) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return string(e.Reason)
	}
	return string(e.Reason) + ": " + e.Cause.Error()
}

func (e *PublishFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// PublishFailureReasonOf answers what a caller may say about this failure.
//
// NONE FOR NIL, and UNKNOWN for an error that carries no typed cause — which is
// the fail-open rule the refusal vocabulary already uses: a cause nobody has
// classified is still a degraded pane, and the product saying "nocx cannot say
// why" is worth more than a second silence.
func PublishFailureReasonOf(err error) PublishFailureReason {
	if err == nil {
		return PublishFailureNone
	}
	var failure *PublishFailure
	if errors.As(err, &failure) && failure.Reason != PublishFailureNone {
		return failure.Reason
	}
	return PublishFailureUnknown
}
