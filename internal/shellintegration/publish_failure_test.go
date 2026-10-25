package shellintegration

import (
	"errors"
	"fmt"
	"testing"
)

// The classifier is the seam the pane reads through, and it has two edges that
// must not be the same: nil is "nothing to report" and a landing copy must stay
// silent, while an error nobody typed is still a copy that did not land
// (nocx-1kejz.4). Both are asserted here because the interesting failure is the
// second one — a blank card is worse than a card that says "nocx cannot say
// why", and a card on every healthy pane is worse than both.
func TestPublishFailureReasonOf(t *testing.T) {
	cause := errors.New("the far side named no home directory")
	typed := &PublishFailure{Reason: PublishFailureHome, Cause: cause}

	cases := []struct {
		name string
		err  error
		want PublishFailureReason
	}{
		{"nothing to report", nil, PublishFailureNone},
		{"typed at the step that knows it", typed, PublishFailureHome},
		// The real route wraps the typed cause twice — the publish protocol's
		// own sentence and the host — and the reason has to survive both.
		{
			"wrapped twice by the route",
			fmt.Errorf("publish the shell integration bundle on host.example: %w",
				fmt.Errorf("sftp: %w", typed)),
			PublishFailureHome,
		},
		{"a cause nobody classified", cause, PublishFailureUnknown},
		// A typed failure with no reason is not a failure this type can
		// describe; guessing one would be inventing the cause.
		{"typed with no reason", &PublishFailure{Cause: cause}, PublishFailureUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PublishFailureReasonOf(tc.err); got != tc.want {
				t.Fatalf("PublishFailureReasonOf(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// The raw chain is what the LOG keeps (nocx-viil keeps it out of the pane), so
// the wrapper must not swallow it: a typed failure still answers errors.Is for
// the cause beneath it, and its message names both halves.
func TestPublishFailureKeepsItsCause(t *testing.T) {
	cause := errors.New("permission denied")
	failure := &PublishFailure{Reason: PublishFailureWrite, Cause: cause}
	wrapped := fmt.Errorf("publish the shell integration bundle on host.example: %w", failure)

	if !errors.Is(wrapped, cause) {
		t.Fatalf("the cause is unreachable through the wrapper: %v", wrapped)
	}
	var typed *PublishFailure
	if !errors.As(wrapped, &typed) || typed.Reason != PublishFailureWrite {
		t.Fatalf("the typed failure is unreachable through the wrapper: %v", wrapped)
	}
	// The reason is spelled into the message rather than implied by it: a log
	// line is read by somebody who cannot see the type.
	if msg := wrapped.Error(); msg != "publish the shell integration bundle on host.example: "+
		"write-refused: permission denied" {
		t.Fatalf("message = %q, want the route's sentence, the reason and the cause", msg)
	}
}
