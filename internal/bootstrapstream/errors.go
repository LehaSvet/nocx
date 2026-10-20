// Package bootstrapstream owns the two errors a bootstrap line reader can
// return, and owns them for every implementation of that reader.
//
// It exists because the alternative was tried and failed silently. The reader
// is declared as an interface by its CONSUMER (shellintegration's
// BootstrapStream) and implemented by two producers that know nothing about
// frames — internal/ssh over a channel, internal/session over a local pty. Each
// of the three declared its own `ErrBootstrapDeadline`, with this rationale
// written above one of them:
//
//	"It is declared here as well as in internal/shellintegration because
//	 neither package may import the other; the values are compared by the
//	 CALLER's Is, so each side matches its own."
//
// Both halves were wrong. shellintegration DOES import internal/ssh (the
// publisher's SFTP carrier), and `errors.Is` compares the value it is GIVEN
// against the value it is asked about — so a deadline raised by either producer
// never matched the sentinel the consumer tested for. The consequence was not
// a lint-level tidiness problem: `bootstrap-timeout` became unreachable in
// production, every deadline reached the user as "the connection ended", and
// the branch that writes the abort header — the one that gets a far side off
// its blocked read and back to a prompt — never ran. Three packages of tests
// stayed green throughout, because each one's fake returned its own package's
// sentinel.
//
// That vocabulary is broader than the two errors: WHAT A LINE IS crosses the
// same seam, and it is LineEnd's — for the same reason, since a producer that
// decides a marker is a line and a consumer that waits for one have to agree on
// where a line ends. They disagreed once (nocx-1ebrs.22), and the symptom was
// a protocol token in the person's transcript.
//
// So the errors live in a leaf package that all three import, and there is one
// value per fact. The INTERFACE stays with its consumer, where Go puts it; only
// the vocabulary that crosses the seam is shared (AD-8: one owner per
// behaviour, and the owner is whoever the behaviour is about).
package bootstrapstream

import (
	"bytes"
	"errors"
)

// ErrDeadline is what a bootstrap ReadLine returns when its deadline passed
// before a line completed. It is a deadline and nothing else: the stream is
// still open, the far side is still there, and the bytes read so far are left
// where the user's terminal will get them.
var ErrDeadline = errors.New("bootstrap: the deadline passed before a line arrived")

// ErrLineTooLong is what a bootstrap ReadLine returns when the far side wrote
// more than the reader's bound with no line ending. It says nothing about the
// connection — which is precisely why it is not the same error as EOF: what
// arrived is output that is not this protocol, and the reader stops rather than
// accumulate an unbounded stream looking for a newline that is not coming.
//
// Wrap it with the counts so the log can say how much arrived without the
// bytes themselves ever being written anywhere (AD-6: the backend does not
// read the stream, and a "bounded prefix" in a log file is reading it).
var ErrLineTooLong = errors.New("bootstrap: a line exceeded the bound")

// LineEnd reports whether b holds a complete line: content is the index one
// past the line's own bytes (its line ending and any '\r' excluded), next is
// the index the caller consumes up to, and ok says the line is complete.
//
// A line ends at '\n', with ONE exception: a line that begins with ESC ']' is
// an OSC string, and it ends at its BEL terminator instead.
//
// The exception is nocx-1ebrs.22. The bootstrap's markers used to be ordinary
// lines, and on the typed-`ssh` path the far side's stdout is the pty the
// person is looking at — the window's tap is a copy, so nothing can "consume"
// a marker out of their transcript, and a slow reader or a timeout left
// NOCX1 LOADER_READY and its siblings printed in the middle of it. Every
// terminal consumes an OSC whole and paints none of it, so the markers travel
// in one now (shellintegration.MarkerOSC) and the screen cannot receive them at
// all, whatever the reader does. Such a string carries no '\n', and waiting
// for one would turn every marker into a bootstrap timeout, so BEL ends it
// here.
//
// The BEL is part of the line's CONTENT, not its ending: the wrapper is the
// caller's to strip, and the terminator belongs to it. That is the one reason
// content and next are two numbers — a '\n' is the line's ending and is not,
// a BEL is nocx's own byte and is.
//
// The concession costs nothing the newline rule had not already paid for. A far
// side that opens an OSC and never closes it is refused by the reader's line
// bound exactly as a stream with no newline in it is, and the bytes are left
// where the next reader will find them.
func LineEnd(b []byte) (content, next int, ok bool) {
	if len(b) > 1 && b[0] == 0x1b && b[1] == ']' {
		if i := bytes.IndexByte(b, 0x07); i >= 0 {
			return i + 1, i + 1, true
		}
		return 0, 0, false
	}
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return i, i + 1, true
	}
	return 0, 0, false
}
