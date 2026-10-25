package shellintegration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the installer gate through the same native launch as a local pane.
// The kernel rejects sequence replays: a second hello cannot activate the shell.
func TestLocalZshPane_InstalledGateEstablishesOnceInEitherOrder(t *testing.T) {
	zsh := requireIntegrationShell(t, "zsh")
	for _, phase := range []string{".zshrc", ".zlogin"} {
		t.Run(phase, func(t *testing.T) {
			home := t.TempDir()
			installed := filepath.Join(home, ".nocx")
			if err := os.Mkdir(installed, 0o700); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(installed, "shell-integration.zsh")
			if err := os.WriteFile(script, []byte(zshScript), 0o600); err != nil {
				t.Fatal(err)
			}
			// .zshrc loads the installed script before the launcher; .zlogin
			// loads it after. The latter deliberately clears only the install
			// latch, just as the launcher does, to exercise state preservation.
			gate := gateLineZsh + "\n"
			if phase == ".zlogin" {
				gate = "unset __nocx_loaded\n" + gate
			}
			if err := os.WriteFile(filepath.Join(home, phase), []byte(gate), 0o600); err != nil {
				t.Fatal(err)
			}
			unsetZDOTDIR(t)
			s := startLocalPane(t, zsh, ShellZsh, home, localTestOpts())
			s.run("ls; print -r -- \"ACTIVE=\"$__nocx_lc_active")
			s.run("print -r -- SECOND_COMMAND_DONE")
			if !strings.Contains(s.output(), "ACTIVE=1") {
				t.Fatalf("channel inactive: %q", s.output())
			}
			if got := s.kernel.count("hello"); got != 1 {
				t.Errorf("hello count=%d, want 1", got)
			}
			if got := s.kernel.rejectedCount(); got != 0 {
				t.Errorf("replayed frames=%d", got)
			}
		})
	}
}

// Re-sourcing while an app command is running must retain its shell attempt
// and sequence, not just keep a ready shell looking active.
func TestShellChannel_ResourcePreservesTheActiveAttempt(t *testing.T) {
	for _, tc := range []struct{ name, script string }{{"zsh", zshScript}, {"bash", bashScript}} {
		t.Run(tc.name, func(t *testing.T) {
			shell := requireIntegrationShell(t, tc.name)
			s := startChannelShell(t, shell, "nocx."+tc.name, tc.script)
			file := filepath.Join(t.TempDir(), "integration")
			if err := os.WriteFile(file, []byte(tc.script), 0o600); err != nil {
				t.Fatal(err)
			}
			s.run("unset __nocx_loaded; . " + ShellQuote(file) + "; unset __nocx_loaded; . " + ShellQuote(file) + "; unset __nocx_loaded; . " + ShellQuote(file) + "; echo RESOURCE_DONE")
			s.run("(unset __nocx_loaded __nocx_cap; . " + ShellQuote(file) + "; echo SUBSHELL_ACTIVE=$__nocx_lc_active)")
			if !strings.Contains(s.output(), "SUBSHELL_ACTIVE=0") {
				t.Fatalf("fork reused the parent's established channel: %q", s.output())
			}
			s.run("echo NEXT_COMMAND_DONE")
			if s.kernel.count("hello") != 1 || s.kernel.rejectedCount() != 0 {
				t.Fatalf("channel restarted: events=%v rejected=%d", s.kernel.events(), s.kernel.rejectedCount())
			}
			if got := s.kernel.count("complete"); got != 3 {
				t.Errorf("completed %d commands, want 3", got)
			}
		})
	}
}

func TestLocalBashPane_InstalledGateEstablishesOnce(t *testing.T) {
	bash := requireIntegrationShell(t, "bash")
	home := t.TempDir()
	installed := filepath.Join(home, ".nocx")
	if err := os.Mkdir(installed, 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(installed, "shell-integration.bash")
	if err := os.WriteFile(script, []byte(bashScript), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(gateLineBash+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := startLocalPane(t, bash, ShellBash, home, localTestOpts())
	s.run("ls; printf 'ACTIVE=%s\\n' \"$__nocx_lc_active\"")
	s.run("echo SECOND_COMMAND_DONE")
	if !strings.Contains(s.output(), "ACTIVE=1") {
		t.Fatalf("channel inactive: %q", s.output())
	}
	if s.kernel.count("hello") != 1 || s.kernel.rejectedCount() != 0 {
		t.Fatalf("channel restarted: events=%v rejected=%d", s.kernel.events(), s.kernel.rejectedCount())
	}
}

func TestShellChannel_ResourceDoesNotReuseAnotherOwner(t *testing.T) {
	for _, tc := range []struct{ name, script string }{{"zsh", zshScript}, {"bash", bashScript}} {
		t.Run(tc.name, func(t *testing.T) {
			shell := requireIntegrationShell(t, tc.name)
			script := writeScriptFile(t, "nocx."+tc.name, tc.script)
			for _, axis := range []string{"shell", "owner", "lane", "dom", "epoch", "subshell"} {
				t.Run(axis, func(t *testing.T) {
					program := `__nocx_lc_shell=$3
__nocx_lc_owner=$$
__nocx_lc_subshell=${ZSH_SUBSHELL:-${BASH_SUBSHELL:-0}}
__nocx_lc_active=1
__nocx_lc_lane=lane
__nocx_lc_dom=domain
__nocx_lc_epoch=1
NOCX_LIFECYCLE_LANE=lane
NOCX_LIFECYCLE_DOMAIN=domain
NOCX_LIFECYCLE_EPOCH=1
case "$2" in
    shell) __nocx_lc_shell=another ;;
    owner) __nocx_lc_owner=not-this-shell ;;
    lane) NOCX_LIFECYCLE_LANE=another ;;
    dom) NOCX_LIFECYCLE_DOMAIN=another ;;
    epoch) NOCX_LIFECYCLE_EPOCH=2 ;;
    subshell) ( . "$1"; [[ "$__nocx_lc_active" == 0 && "$__nocx_lc_seq" == 0 ]] ); exit $? ;;
esac
. "$1"
[[ "$__nocx_lc_active" == 0 && "$__nocx_lc_seq" == 0 ]]
`
					// #nosec G204 -- requireIntegrationShell resolves the real shell under test.
					cmd := exec.Command(shell, "-f", "-c", program, tc.name, script, axis, tc.name)
					cmd.Env = cleanEnv("HOME="+t.TempDir(), "NOCX_SHELL_INTEGRATION=1", "TMPDIR="+t.TempDir())
					if out, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("reused a different %s: %v output=%q", axis, err, out)
					}
				})
			}
		})
	}
}

func TestBashResourceRetainsOneWrapperAndOriginalPredecessors(t *testing.T) {
	bash := requireIntegrationShell(t, "bash")
	script := writeScriptFile(t, "nocx.bash", bashScript)
	program := `PROMPT_COMMAND='printf original-prompt'
trap 'printf original-debug >/dev/null' DEBUG
trap 'printf original-exit >/dev/null' EXIT
. "$1"
__nocx_expected_debug=$__nocx_old_debug
__nocx_expected_exit=$__nocx_old_exit
for __nocx_pass in 1 2 3; do
    unset __nocx_loaded
    . "$1"
    [[ "$PROMPT_COMMAND" == __nocx_prompt_command ]] || exit 81
    [[ "$__nocx_old_pc" == 'printf original-prompt' ]] || exit 82
    [[ "$__nocx_old_debug" == "$__nocx_expected_debug" ]] || exit 83
    [[ "$__nocx_old_exit" == "$__nocx_expected_exit" ]] || exit 84
done
[[ "$(trap -p DEBUG)" == "trap -- '__nocx_preexec_wrapper' DEBUG" ]] || exit 85
[[ "$(trap -p EXIT)" == "trap -- '__nocx_exit_cleanup' EXIT" ]] || exit 86
`
	// #nosec G204 -- requireIntegrationShell resolves the real shell under test.
	cmd := exec.Command(bash, "--noprofile", "--norc", "-c", program, "bash", script)
	cmd.Env = cleanEnv("HOME="+t.TempDir(), "TMPDIR="+t.TempDir(), "NOCX_SHELL_INTEGRATION=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("wrapper capture stacked on re-source: %v; output=%q", err, out)
	}
}
