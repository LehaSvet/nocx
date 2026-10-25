package shellintegration

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func plantLegacyScripts(t *testing.T, home string) map[string][]byte {
	t.Helper()
	// Use the production installer for the gates, never a second spelling.
	if err := New(testLogger()).EnsureInstalled(home); err != nil {
		t.Fatal(err)
	}
	before := map[string][]byte{}
	for rc := range rcGate {
		// #nosec G304 -- disposable home plus production-owned rc filename.
		data, err := os.ReadFile(filepath.Join(home, rc))
		if err != nil {
			t.Fatal(err)
		}
		before[rc] = data
	}
	for name := range scripts {
		if err := os.WriteFile(filepath.Join(home, dirName, name), []byte("# previous integration generation\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return before
}

func TestRemotePublishNeutralisesLegacyScriptsWithoutEditingStartup(t *testing.T) {
	for _, overPipe := range []bool{false, true} {
		t.Run(map[bool]string{false: "sftp", true: "pipe"}[overPipe], func(t *testing.T) {
			home := t.TempDir()
			before := plantLegacyScripts(t, home)
			s := New(testLogger())
			publish := s.EnsureInstalledRemote
			if overPipe {
				publish = s.EnsureInstalledOverPipe
			}
			for attempt := 0; attempt < 2; attempt++ {
				// Replant after the first publish: the current manifest must not
				// cause retirement to be skipped on the next connection.
				if attempt != 0 {
					plantLegacyScripts(t, home)
				}
				if err := publish(context.Background(), NewOSFS(), home); err != nil {
					t.Fatal(err)
				}
				manifest := readManifestT(t, filepath.Join(home, dirName))
				for name := range scripts {
					data := readFileT(t, filepath.Join(home, dirName, name))
					if string(data) != legacyRetirementStub(manifest.Generation) {
						t.Fatalf("legacy target is not the provenance-bearing inert stub: %s", name)
					}
				}
				for rc, want := range before {
					if string(readFileT(t, filepath.Join(home, rc))) != string(want) {
						t.Fatalf("startup changed: %s", rc)
					}
				}
			}
		})
	}
}

func TestRemoteRetirementNeverExposesPartialScript(t *testing.T) {
	for _, operation := range []string{"lstat", "readfile", "create", "write", "sync", "close", "rename", "syncdir"} {
		t.Run(operation, func(t *testing.T) {
			home := t.TempDir()
			plantLegacyScripts(t, home)
			root := filepath.Join(home, dirName)
			if _, err := NewPublisher(testLogger(), NewOSFS(), root).Publish(shippedBundle()); err != nil {
				t.Fatal(err)
			}
			faults := newFaultFS(NewOSFS())
			fsys := &retirementFaultFS{
				FS: faults, faults: faults, operation: operation,
				target: filepath.Join(root, "shell-integration.bash"),
			}
			pub := NewPublisher(testLogger(), fsys, root).attempt()
			if err := pub.retireLegacyScripts("replacement"); !errors.Is(err, errInjected) {
				t.Fatalf("fault not surfaced: %v", err)
			}
			for name := range scripts {
				data := string(readFileT(t, filepath.Join(root, name)))
				if data != "# previous integration generation\n" && data != legacyRetirementStub("replacement") {
					t.Fatalf("partial legacy target after %s failure", operation)
				}
			}
			// Any failed retirement can be retried, with no broken live file.
			if err := NewPublisher(testLogger(), NewOSFS(), root).attempt().retireLegacyScripts("replacement"); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(filepath.Join(root, tmpName))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("retirement left staging residue: %d entries", len(entries))
			}
		})
	}
}

func TestRemoteRetirementRefusesSymlinksAndNonRegularTargets(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "directory", true: "symlink"}[symlink], func(t *testing.T) {
			home := t.TempDir()
			plantLegacyScripts(t, home)
			root := filepath.Join(home, dirName)
			target := filepath.Join(root, "shell-integration.zsh")
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), "untouched")
			if err := os.WriteFile(outside, []byte("untouched"), 0o600); err != nil {
				t.Fatal(err)
			}
			if symlink {
				if err := os.Symlink(outside, target); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(target, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := New(testLogger()).EnsureInstalledRemote(context.Background(), NewOSFS(), home); err == nil {
				t.Fatal("unsafe legacy target accepted")
			}
			if string(readFileT(t, outside)) != "untouched" {
				t.Fatal("followed the legacy symlink")
			}
		})
	}
}

func TestInjectedStartupReservesLegacyLatch(t *testing.T) {
	for _, kind := range []ShellKind{ShellBash, ShellZsh} {
		t.Run(string(kind), func(t *testing.T) {
			shell := requireShell(t, string(kind))
			home := t.TempDir()
			if err := New(testLogger()).EnsureInstalled(home); err != nil {
				t.Fatal(err)
			}
			name := "shell-integration." + string(kind)
			// A hostile old body after the historical latch proves it never
			// gets past that guard, even if publish could not neutralise it.
			old := "if [[ -n \"${__nocx_loaded:-}\" ]]; then return; fi\nprintf 'STALE_LOADER_ACTIVE\\n'\n"
			if err := os.WriteFile(filepath.Join(home, dirName, name), []byte(old), 0o600); err != nil {
				t.Fatal(err)
			}
			var arg string
			outer := "/bin/sh"
			opts := LaunchOptions{SessionID: "reserved-loader", Enhanced: true}
			if kind == ShellBash {
				rc, err := LocalBashRcfile(opts)
				if err != nil {
					t.Fatal(err)
				}
				arg = bashArgFor(rc)
				outer = shell
			} else {
				arg = zshArgFor(zshRcfile(launcherEnvBlock(opts), zshScript, ""))
			}
			out := runLauncherOnPTY(t, outer, arg, []string{"HOME=" + home, "ZDOTDIR=" + home, "SHELL=" + shell, "NOCX_SHELL_INTEGRATION=1"}, `printf 'CURRENT_LOADER[%s]\n' "$__nocx_script_version"`, "exit")
			if strings.Contains(out, "STALE_LOADER_ACTIVE") {
				t.Fatal("installer loader ran during injected startup")
			}
			if !strings.Contains(out, "CURRENT_LOADER["+version+"]") {
				t.Fatal("injected shell never reached its first command")
			}
		})
	}
}

// Keep the absence contract explicit: publishing a fresh remote home must
// not create installer targets for a gate that never existed.
func TestRemotePublishDoesNotInventLegacyTargets(t *testing.T) {
	home := t.TempDir()
	if err := New(testLogger()).EnsureInstalledRemote(context.Background(), NewOSFS(), home); err != nil {
		t.Fatal(err)
	}
	for name := range scripts {
		if _, err := os.Lstat(filepath.Join(home, dirName, name)); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("legacy target invented: %s (%v)", name, err)
		}
	}
}

func TestInjectedBashStartupPreservesUserExitTrap(t *testing.T) {
	bash := requireShell(t, "bash")
	home := t.TempDir()
	trapped := filepath.Join(home, "exit-trap-ran")
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte("trap "+ShellQuote("printf preserved > "+ShellQuote(trapped))+" EXIT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rc, err := LocalBashRcfile(LaunchOptions{Enhanced: true, SessionID: "preserved-trap"})
	if err != nil {
		t.Fatal(err)
	}
	runLauncherOnPTY(t, bash, bashArgFor(rc), []string{"HOME=" + home, "TMPDIR=" + t.TempDir()}, "exit")
	if string(readFileT(t, trapped)) != "preserved" {
		t.Fatal("reserved legacy latch erased user exit trap")
	}
}

// A retirement temp belongs to the publisher's staging directory. Hold its
// lock across the write/rename interval so a concurrent publish cannot sweep
// the temp while it is still live.
func TestRemoteRetirementHoldsPublisherLock(t *testing.T) {
	home := t.TempDir()
	plantLegacyScripts(t, home)
	root := filepath.Join(home, dirName)
	if _, err := NewPublisher(testLogger(), NewOSFS(), root).Publish(shippedBundle()); err != nil {
		t.Fatal(err)
	}
	fsys := &retirementLockFS{FS: NewOSFS(), t: t, root: root}
	if err := NewPublisher(testLogger(), fsys, root).attempt().retireLegacyScripts("replacement"); err != nil {
		t.Fatal(err)
	}
	if fsys.staged != len(scripts) {
		t.Fatalf("checked %d replacements, want %d", fsys.staged, len(scripts))
	}
}

type retirementLockFS struct {
	FS
	t      *testing.T
	root   string
	staged int
}

func (f *retirementLockFS) Rename(src, dst string) error {
	if strings.HasPrefix(filepath.Base(src), "retired-") {
		if _, err := f.FS.Lstat(filepath.Join(f.root, lockName)); err != nil {
			f.t.Fatal("retirement staging is exposed to concurrent publish cleanup")
		}
		f.staged++
	}
	return f.FS.Rename(src, dst)
}

// Arm only when retirement reaches its first legacy target. Lock acquisition
// has its own nonce write and must not consume a replacement fault.
type retirementFaultFS struct {
	FS
	faults    *faultFS
	operation string
	target    string
	armed     bool
}

func (f *retirementFaultFS) Lstat(path string) (fs.FileInfo, error) {
	if path == f.target && !f.armed {
		f.armed = true
		f.faults.setFault(f.operation, f.faults.counts[f.operation]+1, errInjected)
	}
	return f.FS.Lstat(path)
}
