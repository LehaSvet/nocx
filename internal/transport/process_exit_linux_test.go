//go:build linux

package transport

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/waittest"
	"golang.org/x/sys/unix"
)

// TestProcessExitHelper is an executable child-process fixture. The parent
// deliberately does not call Wait until cleanup, leaving the exited child in
// the process table so the test can distinguish exit from reaping.
func TestProcessExitHelper(t *testing.T) {
	if os.Getenv("NOCX_TEST_PROCESS_EXIT_HELPER_MODE") != "child" {
		t.Skip("subprocess fixture")
	}
}

func processExitChildEnvironment() []string {
	env := os.Environ()
	childEnv := make([]string, 0, len(env)+1)
	for _, item := range env {
		if strings.HasPrefix(item, "NOCX_TEST_PROCESS_EXIT_HELPER_MODE=") {
			continue
		}
		childEnv = append(childEnv, item)
	}
	return append(childEnv, "NOCX_TEST_PROCESS_EXIT_HELPER_MODE=child")
}

func TestProcessExitedRecognizesZombieAsExited(t *testing.T) {
	child := exec.Command(os.Args[0], "-test.run=^TestProcessExitHelper$") // #nosec G204 — this is the current test binary with fixed arguments.
	child.Env = processExitChildEnvironment()
	if err := child.Start(); err != nil {
		t.Fatalf("start exited child: %v", err)
	}
	t.Cleanup(func() {
		_ = child.Wait()
	})
	pid := child.Process.Pid
	waittest.WaitFor(t, "child process to become an unreaped zombie", func() bool {
		state, stateErr := linuxProcessState(pid)
		return stateErr == nil && state == 'Z'
	})
	if err := unix.Kill(pid, 0); err != nil {
		t.Fatalf("zombie kill(pid, 0) = %v, want success for the unreaped process", err)
	}
	if !processExited(pid) {
		t.Fatalf("processExited(%d) = false for a process in zombie state", pid)
	}
}

func processExited(pid int) bool {
	state, err := linuxProcessState(pid)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if err == nil {
		return state == 'Z' || state == 'X'
	}
	return errors.Is(unix.Kill(pid, 0), unix.ESRCH)
}

func processExitDetail(pid int) string {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return fmt.Sprintf("pid=%d stat unavailable: %v", pid, err)
	}
	stat := string(b)
	open := strings.IndexByte(stat, '(')
	close := strings.LastIndexByte(stat, ')')
	if open < 0 || close <= open {
		return fmt.Sprintf("pid=%d malformed /proc stat: %q", pid, b)
	}
	fields := strings.Fields(stat[close+2:])
	if len(fields) < 4 {
		return fmt.Sprintf("pid=%d incomplete /proc stat: %q", pid, b)
	}
	comm := stat[open+1 : close]
	return fmt.Sprintf("pid=%d state=%s ppid=%s pgrp=%s session=%s comm=%s", pid, fields[0], fields[1], fields[2], fields[3], comm)
}

func linuxProcessState(pid int) (byte, error) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	// comm is parenthesized and may contain spaces or ')', so split only
	// after its final close paren; state is the first field that follows.
	close := strings.LastIndexByte(string(b), ')')
	if close < 0 || close+2 >= len(b) {
		return 0, errors.New("malformed /proc stat")
	}
	return b[close+2], nil
}
