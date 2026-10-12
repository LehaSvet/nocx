//go:build !linux

package transport

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func processExited(pid int) bool {
	return errors.Is(unix.Kill(pid, 0), unix.ESRCH)
}

func processExitDetail(pid int) string {
	if err := unix.Kill(pid, 0); errors.Is(err, unix.ESRCH) {
		return fmt.Sprintf("pid=%d is absent", pid)
	} else if err != nil {
		return fmt.Sprintf("pid=%d kill(pid, 0) error: %v", pid, err)
	}
	return fmt.Sprintf("pid=%d still exists (state unavailable)", pid)
}
