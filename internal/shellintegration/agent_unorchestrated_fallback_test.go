package shellintegration

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A missing lifecycle channel disables orchestration, not the command the
// person typed. Exercise the delivered script and its generated wrapper in
// both shells: checking a rendered string would not prove the agent ran.
func TestAgentWrapperWithoutLifecycleChannelRunsTheCommand(t *testing.T) {
	for _, shell := range []struct {
		name string
		raw  string
	}{
		{"bash", bashScriptRaw},
		{"zsh", zshScriptRaw},
	} {
		t.Run(shell.name, func(t *testing.T) {
			binary := requireShell(t, shell.name)
			for _, delivery := range []struct {
				name string
				kind Delivery
			}{
				{"local", DeliveryLocal},
				{"published", DeliveryPublished},
			} {
				t.Run(delivery.name, func(t *testing.T) {
					for _, status := range []int{0, 37} {
						t.Run(strconv.Itoa(status), func(t *testing.T) {
							home := t.TempDir()
							binDir := t.TempDir()
							argvPath := filepath.Join(home, "agent-argv")
							callsPath := filepath.Join(home, "unexpected-orchestration")
							const agent = `#!/bin/sh
printf '%s\000' "$@" > "$NOCX_FALLBACK_ARGV"
printf 'AGENT-RAN\n'
exit "$NOCX_FALLBACK_STATUS"
`
							// #nosec G306 -- a fake executable agent in the test's own directory.
							if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte(agent), 0o755); err != nil {
								t.Fatal(err)
							}
							script := writeScriptFile(t, "nocx."+shell.name, renderScript(shell.raw, []string{"claude"}, delivery.kind))
							const program = `script=$1
shift
source "$script"
if [[ "${__nocx_lc_active:-0}" != 0 ]]; then
    builtin printf 'unexpected active lifecycle channel\n' >&2
    exit 92
fi
__nocx_agent_enrol() { builtin printf 'enrol\n' >> "$NOCX_FALLBACK_CALLS"; return 91; }
__nocx_agent_launch_resolve() { builtin printf 'resolve\n' >> "$NOCX_FALLBACK_CALLS"; return 91; }
__nocx_lc_send() { builtin printf 'send:%s\n' "$1" >> "$NOCX_FALLBACK_CALLS"; return 91; }
claude "$@"
exit $?
`
							args := []string{"--model", "two words", "", "*?[x]", "quote'\"\\tail", "line\nbreak"}
							ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
							defer cancel()
							// #nosec G204 -- requireShell resolves the real shell under test.
							cmd := exec.CommandContext(ctx, binary, append([]string{"-f", "-c", program, shell.name, script}, args...)...)
							cmd.Env = cleanEnv(
								"HOME="+home, "ZDOTDIR="+home, "TMPDIR="+t.TempDir(), "HISTFILE=/dev/null",
								"PATH="+binDir+":"+os.Getenv("PATH"), "NOCX_SHELL_INTEGRATION=1",
								"NOCX_FALLBACK_ARGV="+argvPath, "NOCX_FALLBACK_CALLS="+callsPath,
								"NOCX_FALLBACK_STATUS="+strconv.Itoa(status),
							)
							out, err := cmd.CombinedOutput()
							if ctx.Err() != nil {
								t.Fatalf("agent wrapper did not finish: %v; output=%q", ctx.Err(), out)
							}
							gotStatus := 0
							if err != nil {
								var exitErr *exec.ExitError
								if !errors.As(err, &exitErr) {
									t.Fatalf("run shell: %v; output=%q", err, out)
								}
								gotStatus = exitErr.ExitCode()
							}
							if gotStatus != status {
								t.Errorf("wrapper exit status = %d, want agent's %d; output=%q", gotStatus, status, out)
							}
							if strings.Count(string(out), "AGENT-RAN\n") != 1 {
								t.Errorf("agent must run exactly once; output=%q", out)
							}
							if !strings.Contains(string(out), "nocx: not orchestrated — this pane has no lifecycle channel\n") {
								t.Errorf("missing visible no-channel warning; output=%q", out)
							}
							// #nosec G304 -- this test owns the argument capture file.
							gotArgs, err := os.ReadFile(argvPath)
							if err != nil {
								t.Fatalf("agent did not record its arguments: %v; output=%q", err, out)
							}
							wantArgs := []byte(strings.Join(args, "\x00") + "\x00")
							if !bytes.Equal(gotArgs, wantArgs) {
								t.Errorf("agent argv = %q, want %q", gotArgs, wantArgs)
							}
							// #nosec G304 -- this test owns the orchestration spy file.
							if calls, err := os.ReadFile(callsPath); !os.IsNotExist(err) {
								t.Errorf("no-channel fallback attempted orchestration: calls=%q, err=%v", calls, err)
							}
						})
					}
				})
			}
		})
	}
}
