package storagetest

// WHO IS STILL HOLDING A DISPOSABLE HOME (nocx-xn63t.6.54).
//
// ONE FILE WITH A RUNTIME BRANCH, and the reason is the OS split rather than
// taste. The set of OS-specific packages is DERIVED from the build constraints
// in the tree (the Makefile's ci-os-split), and a `//go:build linux` pair here
// would move this package into that set — for a reason (the tags) that is not
// the reason the set exists. This package is a test helper both platforms run,
// and what differs between them is the ANSWER it can give, not what it does.
// DisposableRoot above branches on runtime.GOOS for the same reason.
//
// /proc is the Linux answer and the complete one for this account: a process's
// command line, its working directory, its executable and its environment are
// all readable there for every process of this account, and all four are
// places a connection to a directory shows up for somebody else to read. The
// environment is the one that matters most here — a pane's shell carries
// HOME=<the disposable home> and does NOT name it in its command line, so argv
// alone would report "none" about the very process that wrote.
//
// ps is the portable half and the INCOMPLETE one: it lists command lines, so a
// holder visible only through its environment or its working directory is not
// listed — and the caller's message says so, rather than reporting "none"
// about a machine that was never asked the whole question.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// holdersOf names every process that has the home — under any of the spellings
// it is reached by — in its command line, its working directory, its executable
// or its environment.
//
// complete says whether that is the whole question: it is true only where all
// four places could be read, so an empty answer with complete true really is
// "nothing holds it", and an empty one without it is "nothing that names it in
// the way this platform can see".
func holdersOf(spellings ...string) (lines []string, complete bool) {
	if runtime.GOOS == "linux" {
		return procHolders(spellings)
	}
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=,command=").Output() //nolint:gosec // fixed arguments, no caller input
	if err != nil {
		return []string{fmt.Sprintf("could not be listed: %v", err)}, false
	}
	return parsePSHolders(out, spellings...), false
}

// procHolders reads /proc: every process of this account, the four places a
// connection to a directory shows up in.
func procHolders(spellings []string) (lines []string, complete bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return []string{fmt.Sprintf("could not be listed: %v", err)}, true
	}
	for _, e := range entries {
		pid := e.Name()
		if pid == "" || pid[0] < '0' || pid[0] > '9' {
			continue
		}
		base := filepath.Join("/proc", pid)
		var where []string
		// #nosec G304 — /proc/<pid>/<file> for a pid read out of /proc itself;
		// no caller-supplied path reaches these reads.
		if raw, readErr := os.ReadFile(filepath.Join(base, "cmdline")); readErr == nil {
			if cmd := strings.ReplaceAll(strings.TrimRight(string(raw), "\x00"), "\x00", " "); namesAny(cmd, spellings...) {
				where = append(where, "cmd "+cmd)
			}
		}
		if cwd, linkErr := os.Readlink(filepath.Join(base, "cwd")); linkErr == nil && namesAny(cwd, spellings...) {
			where = append(where, "cwd "+cwd)
		}
		if exe, linkErr := os.Readlink(filepath.Join(base, "exe")); linkErr == nil && namesAny(exe, spellings...) {
			where = append(where, "exe "+exe)
		}
		if env := namedInEnvironment(filepath.Join(base, "environ"), spellings); env != "" {
			where = append(where, "env "+env)
		}
		if len(where) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("pid %s (ppid %s): %s", pid, parentOf(base), strings.Join(where, "; ")))
	}
	sort.Strings(lines)
	if len(lines) > holdersMax {
		lines = append(lines[:holdersMax], fmt.Sprintf("and %d more", len(lines)-holdersMax))
	}
	return lines, true
}

// namedInEnvironment returns the environment entries of one process that name
// the home, joined, or "" when none does.
//
// Each entry is bounded: an environment block belongs to the process, and a
// value longer than this has nothing to add to the answer.
func namedInEnvironment(path string, spellings []string) string {
	// #nosec G304 — /proc/<pid>/environ, built from the pids /proc listed.
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var hits []string
	for _, kv := range strings.Split(string(raw), "\x00") {
		if !namesAny(kv, spellings...) {
			continue
		}
		if len(kv) > 200 {
			kv = kv[:200] + "..."
		}
		hits = append(hits, kv)
	}
	return strings.Join(hits, " ")
}

// parentOf reads a process's parent out of its stat line, for the message's
// sake: ppid 1 says the process that started it is already gone, which is how
// a reader tells a straggler from a live tree.
func parentOf(base string) string {
	// #nosec G304 — /proc/<pid>/stat, built from the pids /proc listed.
	raw, err := os.ReadFile(filepath.Join(base, "stat"))
	if err != nil {
		return "?"
	}
	// comm is parenthesised and may itself contain spaces and brackets, so the
	// fields after the LAST ')' are the ones that are positional.
	i := strings.LastIndex(string(raw), ")")
	if i < 0 {
		return "?"
	}
	fields := strings.Fields(string(raw)[i+1:])
	if len(fields) < 2 {
		return "?"
	}
	return fields[1]
}
