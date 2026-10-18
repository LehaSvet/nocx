package storagetest

// THE REMOVAL'S FAILURE HAS TO SAY WHAT IT TRIPPED ON (nocx-xn63t.6.54).
//
// The check this file holds is the one the CI run's own message could not
// answer: `unlinkat /private/tmp/nocx-home-783600994: directory not empty`
// names the home and nothing else. A reader cannot tell from it whether a
// file appeared in the walk's window (a shell writing on its way out), whether
// a process re-created the endpoint directory, or whether the walk itself was
// wrong — which is why the occurrence cost a diagnosis and a rerun.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// removalChildEnv marks the re-executed child of the check below.
const removalChildEnv = "NOCX_STORAGETEST_REMOVAL_CHILD"

// TestTheRemovalNamesWhatAStragglerLeftInTheHome fails on the tree this bead
// was filed against: its message named the directory alone.
//
// IT IS RUN IN A CHILD OF THIS TEST BINARY. The text it is about is handed to
// t.Error, and the only place that text can be read from is the test binary's
// own output; so the child plants the straggler and fails on purpose, and this
// one reads the words it printed. (Go's standard re-exec pattern, the same one
// internal/transport, internal/content and cmd/nocx-helper use.)
//
// THE HOME IS REACHED THROUGH A SYMLINK, which is what macOS does to every
// temporary directory (/tmp is a link to /private/tmp): the removal resolves
// both sides before it compares them, and a check that only ever saw one
// spelling would exercise half of that. On Linux the two spellings are the
// same string, so the link is the test's own.
func TestTheRemovalNamesWhatAStragglerLeftInTheHome(t *testing.T) {
	if os.Getenv(removalChildEnv) == "1" {
		stragglerInTheWayOfTheRemoval(t)
		return
	}
	out, err := theChildTestBinaryPrinted(t, "TestTheRemovalNamesWhatAStragglerLeftInTheHome")
	if err == nil {
		t.Fatalf("the child test passed, and its whole purpose is to fail; it printed:\n%s", out)
	}
	if !strings.Contains(out, "remove the disposable home") {
		t.Fatalf("the child failed for some other reason than the removal; it printed:\n%s", out)
	}
	for _, want := range []string{
		"what was left under it:",
		".bash_history",
		"the processes still holding it:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the removal's failure does not say %q, so the next one still costs a diagnosis. It said:\n%s", want, out)
		}
	}
	if runtime.GOOS == "linux" {
		// The holder is a shell whose HOME is the home and whose working
		// directory is there: /proc answers both, and the message has to name
		// the process rather than the paths alone.
		for _, want := range []string{"pid ", "env HOME="} {
			if !strings.Contains(out, want) {
				t.Errorf("the removal's failure does not name the process in the way (%q). It said:\n%s", want, out)
			}
		}
		return
	}
	// Every other platform lists command lines only, and that is less than the
	// question: the process this check plants holds the home through its
	// environment and its working directory too. A message that answered
	// "none" without saying what it could not see would be a lie of omission,
	// so the listing is qualified on the platforms that cannot see it all.
	if !strings.Contains(out, "this platform lists command lines only") {
		t.Errorf("the removal's failure does not say what this platform could not see. It said:\n%s", out)
	}
}

// stragglerInTheWayOfTheRemoval is the child: it builds the state the CI
// failure left behind and lets the removal report it.
func stragglerInTheWayOfTheRemoval(t *testing.T) {
	_, _, home := aHomeReachedThroughASymlink(t)
	// The file a pane's shell leaves on its way out — the one the storagetest
	// doc names for bash, and the shape a zsh pane writes as well.
	if err := os.WriteFile(filepath.Join(home, ".bash_history"), []byte("exit\n"), 0o600); err != nil {
		t.Fatalf("plant the straggler: %v", err)
	}
	aProcessHoldingTheHome(t, home)

	// THE WALK'S OWN FAILURE, MADE WHERE IT IS MADE IN REALITY: os.RemoveAll
	// reads a directory's entries, deletes them, and only then removes the
	// directory — so "directory not empty" means one more entry existed at
	// that last step. Reproducing it by racing a real walk is a race about a
	// millisecond wide (measured 2026-10-18: 0.88-1.21 ms to remove a home
	// holding a 14 MB installed helper), and a check that aimed at the race
	// would be a flake instead of a check.
	removeAll = func(target string) error {
		return &os.PathError{Op: "unlinkat", Path: target, Err: syscall.ENOTEMPTY}
	}
	removeUnderTempDir(t, home) // fails on purpose: this is the message under test
}

// TestTheRemovalTakesTheHomeReachedThroughASymlink is the other half of the
// symlink lesson, and it is the half a Linux-only spelling hides: the removal
// must delete the directory the home RESOLVES to, not the link that reaches
// it, or a home under macOS's /tmp would be reported removed while it still
// held everything the suite wrote.
func TestTheRemovalTakesTheHomeReachedThroughASymlink(t *testing.T) {
	_, _, home := aHomeReachedThroughASymlink(t)
	if err := os.WriteFile(filepath.Join(home, ".bash_history"), []byte("exit\n"), 0o600); err != nil {
		t.Fatalf("write into the home: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("resolve the home: %v", err)
	}
	if resolved == home {
		// The check would pass on a machine where both spellings are the same
		// and say nothing about the platform we ship to (AGENTS.md).
		t.Fatalf("the home %q is not reached through a symlink, so this check proves nothing about the resolved spelling", home)
	}

	removeUnderTempDir(t, home)

	if _, err := os.Stat(resolved); !os.IsNotExist(err) {
		t.Fatalf("the home resolved to %q and it is still there after the removal: %v", resolved, err)
	}
}

// aHomeReachedThroughASymlink makes a disposable home whose path goes through
// a link, inside the same root the removal will only delete under, and returns
// the real directory, the link and the home.
func aHomeReachedThroughASymlink(t *testing.T) (realDir, link, home string) {
	t.Helper()
	realDir, err := os.MkdirTemp(DisposableRoot(), "nocx-real-")
	if err != nil {
		t.Fatalf("create the directory the link will reach: %v", err)
	}
	link = realDir + "-link"
	if linkErr := os.Symlink(realDir, link); linkErr != nil {
		t.Fatalf("link %q to %q: %v", link, realDir, linkErr)
	}
	home, err = os.MkdirTemp(link, "nocx-home-")
	if err != nil {
		t.Fatalf("create a home under %q: %v", link, err)
	}
	t.Cleanup(func() {
		// os.RemoveAll rather than the removal under test: this cleans up
		// after a check that has just proven it fails, and a cleanup that
		// uses the code being tested reports its own seam back as a failure.
		_ = os.RemoveAll(home)
		_ = os.Remove(link)
		_ = os.RemoveAll(realDir)
	})
	return realDir, link, home
}

// aProcessHoldingTheHome starts the process shape the failure message is
// usually about: a shell whose HOME is the disposable home and whose working
// directory is there as well, which is exactly what a pane's shell is while
// the removal runs.
func aProcessHoldingTheHome(t *testing.T, home string) {
	t.Helper()
	// The command names the home as well as having it as its HOME and its
	// working directory, so the process is one a listing that can only read
	// command lines still finds — this check is about the message, and every
	// platform has to be able to reach its holder half.
	cmd := exec.Command("/bin/sh", "-c", "cd "+home+" && while :; do sleep 1; done") //nolint:gosec // this test's own fixed command, built from its own home
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "HOME="+home)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the process that holds %q: %v", home, err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
}

// theChildTestBinaryPrinted re-runs this test binary for one check and returns
// everything it printed, with its failure expected by the caller.
func theChildTestBinaryPrinted(t *testing.T, name string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+name+"$", "-test.v=false") // #nosec G204 — os.Args[0] is this test binary and the argument is this check's own name
	cmd.Env = append(os.Environ(), removalChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestThePlatformsWithoutProcStillNameAHolderPinTheParse is the half of the
// failure message this machine cannot run: holdersOf on a platform with no
// /proc reads ps instead, and ps is not asked here. Its parse is, against the
// shape ps actually prints — padded columns, a header, and a command line that
// may itself contain runs of blanks.
func TestThePlatformsWithoutProcStillNameAHolderPinTheParse(t *testing.T) {
	const home = "/tmp/nocx-home-783600994"
	out := []byte(`  PID  PPID COMMAND
    1     0 /sbin/launchd
 4321     1 /bin/zsh -l
 4322  4321 /usr/bin/nocx-helper mcp --socket ` + home + `/x.sock
 5000     1
`)
	lines := parsePSHolders(out, home, "/private/tmp/nocx-home-783600994")
	want := []string{"pid 4322 (ppid 4321): /usr/bin/nocx-helper mcp --socket " + home + "/x.sock"}
	if len(lines) != len(want) || lines[0] != want[0] {
		t.Fatalf("the parse found %q, want %q", lines, want)
	}
	if lines := parsePSHolders(out, "/private/tmp/nocx-home-783600994"); len(lines) != 0 {
		t.Fatalf("the parse matched the /private/tmp spelling against a command line that does not have it: %q", lines)
	}
}
