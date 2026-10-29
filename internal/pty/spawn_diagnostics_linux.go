//go:build linux

package pty

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
)

// spawnDiagnostics samples the parent's state AFTER Start failed. These readings
// are evidence about that instant, not proof of which child-setup syscall failed.
// Only named resource/security fields are retained; no command or environment is read.
func spawnDiagnostics() string {
	return linuxSpawnDiagnostics(os.ReadFile, os.Getpid())
}

func linuxSpawnDiagnostics(read func(string) ([]byte, error), pid int) string {
	fields := []string{"pid=" + strconv.Itoa(pid)}
	status, statusErr := read("/proc/self/status")
	for _, key := range []string{"Threads", "NoNewPrivs", "Seccomp", "Seccomp_filters"} {
		fields = append(fields, key+"="+spawnProcField(status, statusErr, key+":"))
	}
	limits, limitsErr := read("/proc/self/limits")
	fields = append(fields, "Max processes="+spawnProcField(limits, limitsErr, "Max processes"))
	membership, err := read("/proc/self/cgroup")
	if err != nil {
		return strings.Join(append(fields, spawnCgroupUnavailable(err)), "; ")
	}
	mounts, err := read("/proc/self/mountinfo")
	if err != nil {
		return strings.Join(append(fields, spawnCgroupUnavailable(err)), "; ")
	}
	dirs, err := spawnCgroupDirectories(string(membership), string(mounts))
	if err != nil {
		return strings.Join(append(fields, spawnCgroupUnavailable(err)), "; ")
	}
	for _, dir := range dirs {
		counters := []string{"cgroup=" + strconv.Quote(dir)}
		for _, name := range []string{"pids.current", "pids.max", "pids.events"} {
			value, readErr := read(path.Join(dir, name))
			text := strings.Join(strings.Fields(string(value)), " ")
			if readErr != nil {
				text = spawnUnavailable(readErr)
			} else if text == "" {
				text = spawnUnavailable(fmt.Errorf("empty file"))
			}
			counters = append(counters, name+"="+text)
		}
		fields = append(fields, strings.Join(counters, " "))
	}
	return strings.Join(fields, "; ")
}

func spawnCgroupUnavailable(err error) string {
	unavailable := spawnUnavailable(err)
	return "cgroup=" + unavailable + " pids.current=" + unavailable + " pids.max=" + unavailable + " pids.events=" + unavailable
}

func spawnUnavailable(err error) string {
	return fmt.Sprintf("unavailable (%v)", err)
}

func spawnProcField(data []byte, err error, key string) string {
	if err != nil {
		return spawnUnavailable(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, key) {
			continue
		}
		rest := strings.TrimPrefix(line, key)
		if !strings.HasSuffix(key, ":") && len(rest) > 0 && rest[0] != ' ' && rest[0] != '\t' {
			continue
		}
		if value := strings.Join(strings.Fields(rest), " "); value != "" {
			return value
		}
	}
	return spawnUnavailable(fmt.Errorf("field missing"))
}

// spawnCgroupDirectories resolves the pids controller through mountinfo rather
// than assuming /sys/fs/cgroup is the process's cgroup. Only visible ancestors
// are readable: stop at the mount root, never claim to observe the host above it.
func spawnCgroupDirectories(membership, mounts string) ([]string, error) {
	var unified, legacy string
	for _, line := range strings.Split(membership, "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		if parts[0] == "0" && parts[1] == "" {
			unified = parts[2]
		}
		if spawnHasOption(parts[1], "pids") {
			legacy = parts[2]
		}
	}
	var dirs []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(mounts, "\n") {
		halves := strings.SplitN(line, " - ", 2)
		if len(halves) != 2 {
			continue
		}
		left, right := strings.Fields(halves[0]), strings.Fields(halves[1])
		if len(left) < 6 || len(right) < 3 {
			continue
		}
		member := ""
		switch {
		case right[0] == "cgroup" && legacy != "" && spawnHasOption(right[2], "pids"):
			member = legacy
		case right[0] == "cgroup2" && legacy == "":
			member = unified
		}
		if member == "" {
			continue
		}
		mountRoot, mountpoint := spawnMountPath(left[3]), spawnMountPath(left[4])
		if !path.IsAbs(member) || path.Clean(member) != member ||
			!path.IsAbs(mountRoot) || path.Clean(mountRoot) != mountRoot ||
			!path.IsAbs(mountpoint) || path.Clean(mountpoint) != mountpoint {
			continue
		}
		relative := ""
		switch {
		case member == mountRoot:
			// Both paths are cgroup-namespace-relative. Do not guess a
			// namespace mapping when they describe different subtrees.
		case mountRoot == "/":
			relative = strings.TrimPrefix(member, "/")
		case strings.HasPrefix(member, mountRoot+"/"):
			relative = strings.TrimPrefix(member, mountRoot+"/")
		default:
			continue
		}
		current := path.Join(mountpoint, relative)
		for {
			if !seen[current] {
				seen[current] = true
				dirs = append(dirs, current)
			}
			if current == mountpoint {
				break
			}
			current = path.Dir(current)
		}
	}
	if len(dirs) > 0 {
		return dirs, nil
	}
	return nil, fmt.Errorf("pids cgroup mount or membership not resolvable")
}

func spawnHasOption(options, want string) bool {
	for _, option := range strings.Split(options, ",") {
		if option == want {
			return true
		}
	}
	return false
}

func spawnMountPath(value string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(value)
}
