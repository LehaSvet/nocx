package shellintegration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// nocx-1ebrs.23: a shell nocx starts must not print anything its own startup
// would not print.
//
// What the person saw, on the same run as nocx-1ebrs.22, on the far side of a
// typed `ssh <host>`:
//
//	bash: <prefix>/share/bashdb/bashdb-main.inc: No such file or directory
//	bash: warning: cannot start debugger; debugging mode disabled
//
// Neither line is bash's idea. bash holds ONE flag behind `shopt -s extdebug`,
// and when an invocation with `-c` finishes its startup files with that flag
// set it calls start_debugger(): it looks for bashdb, prints those two lines
// when it is not installed, and then CLEARS the flag — so the option nocx uses
// to skip the original command was silently off as well.
//
// The shell that met it was the remote LOGIN shell: sshd runs the user's shell
// as `<shell> -c '<the command>'`, that is a login shell, and an installer-era
// gate line in the profile sources nocx's integration — which used to arm
// extdebug at source time, i.e. during startup. The fix is the boundary: the
// flag is armed at the FIRST PROMPT, which is after every startup file and
// before anything the person types, and a `-c` shell — one that shows no
// prompt and needs no preexec hook — never arms it at all.

// TestTheLoginBashThatRunsOurLoaderPrintsNoDebuggerLines is the defect.
//
// It reproduces the exact arrangement: a login shell invoked the way sshd
// invokes it (`-l -c`), whose profile sources nocx's integration. Today that
// prints bash's two debugger lines into the transcript; the assertion is that
// it prints neither, and that the command itself still ran.
func TestTheLoginBashThatRunsOurLoaderPrintsNoDebuggerLines(t *testing.T) {
	bash := requireShell(t, "bash")
	home := t.TempDir()
	script := writeScriptFile(t, "nocx.bash", bashScript)
	gate := "NOCX_SHELL_INTEGRATION=1\n. " + ShellQuote(script) + "\n"
	if err := os.WriteFile(filepath.Join(home, ".bash_profile"), []byte(gate), 0o600); err != nil {
		t.Fatalf("write .bash_profile: %v", err)
	}

	cmd := exec.Command(bash, "-l", "-c", "echo NOCX_LOGIN_COMMAND_RAN") //nolint:gosec // bash is requireShell's resolved path
	cmd.Env = cleanEnv("HOME="+home, "TERM=dumb", "HISTFILE=/dev/null")

	tr := startTranscript(t, cmd, "the login bash sshd would start")
	if err := tr.waitExit(); err != nil {
		t.Fatalf("the login shell failed: %v", err)
	}
	tr.noDebuggerLines()
	if !strings.Contains(tr.text(), "NOCX_LOGIN_COMMAND_RAN") {
		t.Fatalf("the command the login shell was asked to run did not run:\n%s", tr.visible())
	}
}

// TestBashArmsExtdebugAtTheFirstPrompt is the other end of the interval: the
// deferral must not cost the feature it was moved for. The skip-the-original-
// command mechanism is extdebug's, and an interactive session must have it
// armed by the time its first prompt is drawn — and must still print no
// debugger line, because the flag is now set after bash has finished starting.
func TestBashArmsExtdebugAtTheFirstPrompt(t *testing.T) {
	bash := requireShell(t, "bash")
	home := t.TempDir()
	script := writeScriptFile(t, "nocx.bash", bashScript)
	rc := filepath.Join(t.TempDir(), "nocx-bash.XXXXXX")
	body := "PS1='" + "NOCX_PROMPT> " + "'\n. " + ShellQuote(script) + "\n"
	if err := os.WriteFile(rc, []byte(body), 0o600); err != nil {
		t.Fatalf("write rcfile: %v", err)
	}

	cmd := exec.Command(bash, "--rcfile", rc, "-i") //nolint:gosec // bash is requireShell's resolved path
	cmd.Env = cleanEnv(
		"HOME="+home,
		"TERM=xterm",
		"HISTFILE=/dev/null",
		"NOCX_SHELL_INTEGRATION=1",
		"NOCX_PROMPT_MODE=marker-only",
		"NOCX_SESSION_ID="+strings.Repeat("a", 32),
	)
	tr := startTranscript(t, cmd, "an interactive bash with nocx installed")

	// The B marker rides PS1, so its arrival is the first byte that means the
	// first prompt was drawn — the event, not a duration.
	tr.waitFor("the first prompt", func(out string) bool { return strings.Contains(out, "\x1b]133;B") })
	tr.write("echo ARMED=$(shopt -p extdebug)\nexit\n")
	if err := tr.waitExit(); err != nil {
		t.Logf("the interactive shell exited non-zero: %v", err)
	}
	tr.noDebuggerLines()
	if !strings.Contains(tr.text(), "ARMED=shopt -s extdebug") {
		t.Fatalf("extdebug was not armed by the first prompt, so nested sudo/ssh interception is off:\n%s", tr.visible())
	}
}
