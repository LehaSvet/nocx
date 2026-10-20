package shellintegration

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// nocx-1ebrs.22: the handshake's own markers must never be printable.
//
// What the owner saw on the shipped build, on the INLINE path — `ssh <host>`
// typed in an integrated local tab, so the remote shell is driven in-band
// through the pty he is looking at:
//
//	NOCX1 LOADER_READY
//	NOCX1 STAGE_READY
//	NOCX1 OUTCOME accepted
//
// in a staircase, each line one column right of the last (a bare LF with no
// CR). They are the handshake's frames printed as text, and internal
// protocol vocabulary in somebody's scrollback is the same class of defect as
// the typed `. /dev/fd/3` that nocx-1ebrs.8 removed.
//
// Why it happened is not a bug in the reader. On the saved path the
// bootstrap's reader is the ONLY consumer, so taking a marker line out of the
// stream takes it off the screen by construction. The typed path cannot do
// that: its window is a COPY of the parent session's output (internal/session/
// bootstrap_window.go — every byte still reaches the renderer, unchanged,
// because it is the person's own screen), so nothing the reader does can
// remove a marker, and a reader that is slow, absent or past its deadline
// leaves the protocol in the transcript.
//
// So the markers stopped being text: every one of them travels inside OSC
// 1337, nocx's own private control string, which a terminal consumes whole and
// paints none of. These tests hold BOTH halves of that: the bytes the scripts
// emit, and the fact that no reader is needed for the screen to be clean.

// stripPrivateOSC removes every OSC string from s — what a terminal consumes
// instead of painting. An unterminated one is removed to the end, because a
// terminal in an OSC state paints nothing after it either.
func stripPrivateOSC(s string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "\x1b]")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		j := strings.IndexByte(s[i:], 0x07)
		if j < 0 {
			return b.String()
		}
		s = s[i+j+1:]
	}
}

// markersAndOutcomes is every token this protocol puts on the wire, in the
// spelling the scripts carry.
func markersAndOutcomes() []string {
	out := []string{LoaderReadyToken, StageReadyToken, OutcomePrefix}
	for _, o := range AllOutcomes() {
		out = append(out, OutcomeToken(o))
	}
	return out
}

// TestEveryMarkerIsEmittedInsideThePrivateOSC is the shape assertion: the
// three scripts that emit a marker must carry the wrapper in the printf
// FORMAT, so that printf turns the escapes into ESC and BEL at the terminal.
//
// It is stated as exact text rather than as "contains an OSC" on purpose. A
// marker emitted anywhere else — a bare line, a comment, a debug echo — is
// the defect, and only the exact production says which is which.
func TestEveryMarkerIsEmittedInsideThePrivateOSC(t *testing.T) {
	loader := carrierLoader()
	for _, want := range []string{
		`printf "\033]1337;` + LoaderReadyToken + `\a"`,
		`printf "\033]1337;` + OutcomePrefix + `%s\a" "$1"`,
	} {
		if !strings.Contains(loader, want) {
			t.Errorf("the loader does not emit its marker as %q; a bare line is printed in the person's transcript (nocx-1ebrs.22). loader:\n%s", want, loader)
		}
	}

	stage1, err := Stage1Frame(ShellBash, LaunchOptions{SessionID: strings.Repeat("a", 32), StageDigest: strings.Repeat("0", 64)})
	if err != nil {
		t.Fatalf("render stage-1: %v", err)
	}
	if want := `printf "\033]1337;` + StageReadyToken + `\a"`; !strings.Contains(string(stage1), want) {
		t.Errorf("stage-1 does not emit STAGE_READY as %q", want)
	}

	carrier := launchCarrier()
	if want := `printf '\033]1337;` + OutcomePrefix + `%s\a' "$1"`; !strings.Contains(carrier, want) {
		t.Errorf("the launch carrier does not emit its outcome as %q", want)
	}

	// And nothing else in the three scripts names a token at the start of a
	// printf format, which is what would print it.
	for name, text := range map[string]string{"loader": loader, "stage-1": string(stage1), "carrier": carrier} {
		for _, tok := range markersAndOutcomes() {
			for _, bare := range []string{`printf "` + tok, "printf '" + tok} {
				if strings.Contains(text, bare) {
					t.Errorf("%s still prints %q outside the private OSC", name, tok)
				}
			}
		}
	}
}

// TestMarkerOSCSpellingsAgree holds the two spellings of one wrapper together:
// the bytes a Go reader matches, and the printf escapes the scripts carry.
// They are written twice because two languages need them, and a drift between
// them is a bootstrap that times out on every session.
func TestMarkerOSCSpellingsAgree(t *testing.T) {
	unescaped := strings.NewReplacer(`\033`, "\x1b", `\a`, "\x07").Replace(markerOSCPrintfOpen + "TOKEN" + markerOSCPrintfClose)
	if unescaped != MarkerOSC("TOKEN") {
		t.Fatalf("the printf spelling unescapes to %q, the Go wrapper is %q", unescaped, MarkerOSC("TOKEN"))
	}
}

// TestBootstrapReadsAMarkerInEitherSpelling is the version-skew half. A
// generation already installed on a far host emits bare lines; the reader
// must still understand them, or every session on that host becomes a
// bootstrap timeout. The wrapped form is the one this tree emits.
func TestBootstrapReadsAMarkerInEitherSpelling(t *testing.T) {
	for _, tc := range []struct {
		name string
		mark func(string) string
	}{
		{"wrapped in the private OSC", MarkerOSC},
		{"a bare line, as an older generation emits", func(s string) string { return s }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			far := &scriptedFarSide{steps: []farSideStep{
				{line: tc.mark(LoaderReadyToken)},
				{line: tc.mark(StageReadyToken)},
				{line: tc.mark(OutcomePrefix + OutcomeToken(OutcomeBootstrapAccepted))},
			}}
			opts := stageOpts()
			minted := 0
			lg, _ := captureLog()
			out := DeliverBootstrap(context.Background(), lg, far, testPlan(t, opts, &minted))
			if out != OutcomeBootstrapAccepted {
				t.Fatalf("outcome = %q, want %q (the marker was not recognised)", out, OutcomeBootstrapAccepted)
			}
		})
	}
}

// TestLoaderMarkersNeverReachTheTranscriptWithNoReader is the defect itself,
// stated as an interval with both ends: the loader runs on a real pty, NOBODY
// reads its markers — no window, no ReadLine, no DeliverBootstrap — and the
// transcript the pty carries contains no protocol text at all. The markers
// ARE emitted (the last assertion), so "nothing was printed" cannot be
// satisfied by "nothing was sent".
//
// This is the typed path's situation exactly: the person's pty is the far
// side's stdout, and the bootstrap window is a copy that can be late, or never
// arrive at all.
func TestLoaderMarkersNeverReachTheTranscriptWithNoReader(t *testing.T) {
	sh := requireShell(t, "sh")

	// The loader as the carrier renders it for one session, with the
	// addressing arguments it reads as positional parameters and a
	// well-formed digest it will never be asked to verify here.
	args := []string{"nocx-loader", "", "", "", "0", "9", strings.Repeat("0", 64)}
	cmd := exec.Command(sh, append([]string{"-c", carrierLoader()}, args...)...) //nolint:gosec // the loader is rendered from constants in this package
	// SHELL names the native login shell the loader execs when it refuses;
	// /bin/false is one that ends immediately, so the test observes an exit
	// instead of killing a login shell later.
	cmd.Env = cleanEnv("SHELL=/bin/false", "TERM=xterm")

	tr := startTranscript(t, cmd, "the loader")

	// The loader announces READY once it owns the terminal and then blocks on
	// a header. Nothing consumes that announcement — this is the stalled
	// reader — and the terminal is the only thing watching.
	tr.waitFor("the loader's READY marker on the pty",
		func(out string) bool { return strings.Contains(out, MarkerOSC(LoaderReadyToken)) })

	// Unblock it, with the bytes a deadline sends, so it names its outcome and
	// reaches its prompt: the outcome is the third marker the owner saw
	// printed, and the one furthest from the loader's own code.
	tr.write(string(AbortFrame()))
	if err := tr.waitExit(); err != nil {
		t.Logf("the loader exited non-zero (expected: the native shell it execs is /bin/false): %v", err)
	}

	got := tr.text()
	visible := stripPrivateOSC(got)
	for _, tok := range markersAndOutcomes() {
		if strings.Contains(visible, tok) {
			t.Errorf("the transcript carries %q as printable text:\n%s", tok, visible)
		}
	}
	if !strings.Contains(got, MarkerOSC(OutcomePrefix+OutcomeToken(OutcomeBootstrapProtocol))) {
		t.Errorf("the loader never named its protocol outcome, so this test proves nothing; transcript: %q", got)
	}
}

// TestStageAndCarrierMarkersAreNotPrintable is the same assertion for the two
// markers the SCRIPTED stages emit, driven as the loader drives them: a real
// /bin/sh runs the text, and the transcript is read the way a terminal reads
// it.
func TestStageAndCarrierMarkersAreNotPrintable(t *testing.T) {
	sh := requireShell(t, "sh")
	for name, text := range map[string]string{
		"stage-1": `printf "\033]1337;` + StageReadyToken + `\a"`,
		"carrier": `printf '\033]1337;` + OutcomePrefix + `%s\a' accepted`,
	} {
		out, err := exec.Command(sh, "-c", text).Output() //nolint:gosec // the text is this package's own format
		if err != nil {
			t.Fatalf("%s: run the printf: %v", name, err)
		}
		if visible := stripPrivateOSC(string(out)); visible != "" {
			t.Errorf("%s prints %q, and a terminal would paint it", name, visible)
		}
		if len(out) == 0 {
			t.Errorf("%s emitted nothing at all", name)
		}
	}
}
