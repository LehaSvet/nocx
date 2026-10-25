package shellintegration

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestFishLaunchResolveKeepsPathAsOneField(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish is unavailable")
	}
	for _, path := range []string{"/agent bin:/usr/bin", "/agent\nline\n", ""} {
		t.Run(path, func(t *testing.T) {
			program := fishLaunchSupport + `
function __nocx_agent_clear
end
function __nocx_launch_request_id
    set -g __nocx_launch_rid test-request
end
function __nocx_agent_resolve_answer
end
function __nocx_lc_send
    test (count $argv) = 13; or exit 21
    test "$argv[8]" = /pane_path; or exit 22
    test "$argv[10]" = "$WANT_PATH"; or exit 23
    test "$argv[11]" = /pane_cwd; or exit 24
    test "$argv[13]" = "$PWD"; or exit 25
    builtin printf CONTEXT-OK
end
set -gx PATH "$WANT_PATH"
if test -z "$WANT_PATH"
    set -e PATH
end
__nocx_agent_resolve myagent
`
			cmd := exec.Command(fish, "--no-config", "-c", program) //nolint:gosec // fixture shell and fixed program
			cmd.Env = append(os.Environ(), "WANT_PATH="+path)
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "CONTEXT-OK") {
				t.Fatalf("pane PATH changed the field framing: %q, %v", output, err)
			}
		})
	}
}
