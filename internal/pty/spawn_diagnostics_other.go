//go:build !linux

package pty

// Linux proc/cgroup resource counters have no portable equivalent.
func spawnDiagnostics() string {
	return "unavailable (Linux proc/cgroup diagnostics are not supported on this platform)"
}
