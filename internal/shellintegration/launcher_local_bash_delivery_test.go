package shellintegration

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shady2k/nocx/internal/waittest"
)

// Bash 3.2 sizes its rcfile with fstat and reads that many bytes once. A
// macOS CI pane reached a plain prompt without OSC 133;A despite an enhanced
// pipe launch (nocx-ukf54). Require complete delivery before starting Bash,
// rather than relying on the scheduling of a writer goroutine.
func TestLocalBashLaunchHasTheWholeRcfileBeforeSpawn(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	opts := LaunchOptions{
		SessionID: "local-rcfile", Enhanced: true,
		Capability: testCap, Recovery: strings.Repeat("ef", 32),
		AgentHelperPath:     "/" + strings.Repeat("p", 96*1024),
		AgentToolSocketPath: "/test/coordinator.sock",
	}
	want, err := LocalBashRcfile(opts)
	if err != nil {
		t.Fatal(err)
	}
	want = "exec 3<&-\n" + want
	if len(want) <= 64*1024 {
		t.Fatal("fixture no longer exceeds a pipe buffer")
	}
	launch, err := LocalEnhancedLaunchInMemory("bash", ShellBash, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer launch.Abort()
	if !reflect.DeepEqual(launch.Args, []string{"--rcfile", "/dev/fd/3", "-i"}) {
		t.Fatalf("local Bash changed its inherited script descriptor: %v", launch.Args)
	}
	for _, secret := range []string{opts.Capability, opts.Recovery} {
		if strings.Contains(strings.Join(launch.Args, "\n")+strings.Join(launch.Env, "\n"), secret) {
			t.Fatal("a lifecycle bearer reached launch argv or environment")
		}
	}
	if len(launch.ExtraFiles) != 1 {
		t.Fatalf("script descriptors = %d, want 1", len(launch.ExtraFiles))
	}
	f := launch.ExtraFiles[0]
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Size() != int64(len(want)) {
		t.Fatalf("rcfile before spawn: mode=%s size=%d, want a regular file with all %d bytes", info.Mode(), info.Size(), len(want))
	}
	if _, statErr := os.Stat(f.Name()); !os.IsNotExist(statErr) {
		t.Fatalf("the bearer-bearing rcfile still has a filesystem name: %v", statErr)
	}
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil || offset != 0 {
		t.Fatalf("initial rcfile offset=%d: %v", offset, err)
	}
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatal("the inherited descriptor does not carry the complete rendered rcfile")
	}
	launch.Cleanup()
	launch.Cleanup()
	if _, err := f.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("cleanup did not close the parent's script descriptor: %v", err)
	}
}

// Exercise the public launch on both the oldest supported Bash and the PATH
// Bash. The oversized rcfile must install its final prompt hooks, then the
// shell itself must print the coordinator endpoint. No early terminal input.
func TestLocalBashLaunchIntegratesBeyondAPipeBuffer(t *testing.T) {
	forEachBash(t, func(t *testing.T, shell string) {
		bash := requireShell(t, shell)
		home := t.TempDir()
		// User startup must not inherit a rewindable copy of the script's
		// lifecycle bearers after Bash has read its rcfile into memory.
		rc := `if { : <&3; } 2>/dev/null; then
    printf 'RC_DESCRIPTOR_LEAKED\n'
else
    printf 'RC_DESCRIPTOR_CLOSED\n'
fi
`
		if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(rc), 0o600); err != nil {
			t.Fatal(err)
		}
		opts := LaunchOptions{
			SessionID: "local-large-rcfile", Enhanced: true,
			AgentHelperPath:     "/" + strings.Repeat("p", 96*1024),
			AgentToolSocketPath: "/test/own-coordinator.sock",
		}
		launch, err := LocalEnhancedLaunchInMemory(bash, ShellBash, opts)
		if err != nil {
			t.Fatal(err)
		}
		defer launch.Abort()
		// #nosec G204 -- resolved test shell and product-owned launch arguments.
		cmd := exec.Command(launch.Command, launch.Args...)
		cmd.ExtraFiles = launch.ExtraFiles
		cmd.Env = append(cleanEnv("HOME="+home, "TERM=xterm", "HISTFILE=/dev/null"), launch.Env...)
		capture := newPTYCapture(t, cmd)
		launch.Cleanup()
		waittest.WaitForTimeoutDetail(t, "the integrated prompt", 15*time.Second,
			func() string { return capture.output() },
			func() bool { return strings.Contains(capture.output(), "\x1b]133;A") })
		if _, err := capture.ptmx.Write([]byte(`printf 'OWN-ENDPOINT[%s]\n' "$NOCX_TOOL_SOCKET"` + "\nexit\n")); err != nil {
			t.Fatal(err)
		}
		waittest.WaitForTimeout(t, "the shell to exit", 15*time.Second, func() bool {
			select {
			case <-capture.done:
				return true
			default:
				return false
			}
		})
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(capture.output(), "RC_DESCRIPTOR_LEAKED") || !strings.Contains(capture.output(), "RC_DESCRIPTOR_CLOSED") {
			t.Fatalf("user startup inherited the script descriptor: %q", capture.output())
		}
		if !strings.Contains(capture.output(), "OWN-ENDPOINT["+opts.AgentToolSocketPath+"]\r\n") {
			t.Fatalf("the pane did not print its own tool endpoint: %q", capture.output())
		}
	})
}

func TestLocalBashLaunchReportsAnUnusableScriptDirectory(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	launch, err := LocalEnhancedLaunchInMemory("bash", ShellBash, LaunchOptions{SessionID: "no-temp", Enhanced: true})
	if err == nil {
		launch.Abort()
		t.Fatal("a missing script directory must refuse the launch before spawning")
	}
	if len(launch.ExtraFiles) != 0 {
		t.Fatal("failed launch retained a script descriptor")
	}
}
