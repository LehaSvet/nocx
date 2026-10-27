//go:build linux

package pty

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/log/logtest"
)

func TestNewLocal_SpawnFailureReportsLinuxResources(t *testing.T) {
	_, logger := logtest.New(t)
	_, err := NewLocal(logger, Config{
		Command: filepath.Join(t.TempDir(), "missing-shell"),
		Cols:    80, Rows: 24,
		Env: []string{"NOCX_DIAGNOSTIC_SECRET=must-not-be-reported"},
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("NewLocal = %v, want preserved missing-executable error", err)
	}
	for _, want := range []string{"spawn diagnostics", "pid=", "Threads=", "NoNewPrivs=", "Seccomp=", "Seccomp_filters=", "Max processes=", "pids.current=", "pids.max=", "pids.events="} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("spawn failure lacks %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "must-not-be-reported") {
		t.Fatalf("spawn diagnostics exposed the child environment: %v", err)
	}
}

func TestLinuxSpawnDiagnostics_CollectsOnlySelectedProcessFields(t *testing.T) {
	files := map[string]string{
		"/proc/self/status":    "Name:\tsecret-process-name\nThreads:\t12\nNoNewPrivs:\t0\nSeccomp:\t2\nSeccomp_filters:\t1\n",
		"/proc/self/limits":    "Limit                     Soft Limit           Hard Limit           Units\nMax processes             200                  300                  processes\nMax open files            1024                 1024                 files\n",
		"/proc/self/cgroup":    "0::/workers/job\n",
		"/proc/self/mountinfo": "29 23 0:26 /workers /sys/fs/cgroup rw - cgroup2 cgroup rw\n",
	}
	for _, dir := range []string{"/sys/fs/cgroup/job", "/sys/fs/cgroup"} {
		files[dir+"/pids.current"] = "17\n"
		files[dir+"/pids.max"] = "100\n"
		files[dir+"/pids.events"] = "max 3\n"
	}
	got := linuxSpawnDiagnostics(fixtureReader(files), 42)
	for _, want := range []string{"pid=42", "Threads=12", "NoNewPrivs=0", "Seccomp=2", "Seccomp_filters=1", "Max processes=200 300 processes", "/sys/fs/cgroup/job", "pids.current=17", "pids.max=100", "pids.events=max 3"} {
		if !strings.Contains(got, want) {
			t.Errorf("diagnostics lack %q: %s", want, got)
		}
	}
	if strings.Contains(got, "secret-process-name") || strings.Contains(got, "Max open files") {
		t.Fatalf("diagnostics included unselected proc data: %s", got)
	}
}

func TestLinuxSpawnDiagnostics_ReadFailuresRemainExplicit(t *testing.T) {
	got := linuxSpawnDiagnostics(func(string) ([]byte, error) { return nil, os.ErrPermission }, 42)
	for _, want := range []string{"Threads=unavailable", "NoNewPrivs=unavailable", "Seccomp=unavailable", "Seccomp_filters=unavailable", "Max processes=unavailable", "cgroup=unavailable", "pids.current=unavailable", "pids.max=unavailable", "pids.events=unavailable", "permission denied"} {
		if !strings.Contains(got, want) {
			t.Errorf("diagnostics lack %q: %s", want, got)
		}
	}
}

func TestLinuxSpawnDiagnostics_MissingFieldsRemainExplicit(t *testing.T) {
	got := linuxSpawnDiagnostics(fixtureReader(map[string]string{
		"/proc/self/status": "Name:\tignored\n", "/proc/self/limits": "Limit\n",
		"/proc/self/cgroup": "0::/\n", "/proc/self/mountinfo": "29 23 0:26 / /sys/fs/cgroup rw - cgroup2 cgroup rw\n",
	}), 42)
	for _, want := range []string{"Threads=unavailable", "field missing", "Max processes=unavailable", "pids.current=unavailable", "pids.max=unavailable", "pids.events=unavailable"} {
		if !strings.Contains(got, want) {
			t.Errorf("diagnostics lack %q: %s", want, got)
		}
	}
}

func TestSpawnCgroupDirectories_ResolvesVisibleAncestors(t *testing.T) {
	tests := []struct {
		name, membership, mounts string
		want                     []string
	}{
		{"unified hierarchy", "0::/users/job/child\n", "29 23 0:26 / /sys/fs/cgroup rw - cgroup2 cgroup rw\n", []string{"/sys/fs/cgroup/users/job/child", "/sys/fs/cgroup/users/job", "/sys/fs/cgroup/users", "/sys/fs/cgroup"}},
		{"subtree mount root", "0::/users/job/child\n", "29 23 0:26 /users/job /sys/fs/cgroup rw - cgroup2 cgroup rw\n", []string{"/sys/fs/cgroup/child", "/sys/fs/cgroup"}},
		{"nested namespace membership", "0::/child\n", "29 23 0:26 / /sys/fs/cgroup rw - cgroup2 cgroup rw\n", []string{"/sys/fs/cgroup/child", "/sys/fs/cgroup"}},
		{"legacy pids controller", "3:memory:/ignored\n4:pids:/jobs/one\n", "29 23 0:26 / /sys/fs/cgroup/pids rw - cgroup cgroup rw,pids\n", []string{"/sys/fs/cgroup/pids/jobs/one", "/sys/fs/cgroup/pids/jobs", "/sys/fs/cgroup/pids"}},
		{"escaped mountpoint", "0::/job\n", "29 23 0:26 / /sys/fs/cgroup\\040space rw - cgroup2 cgroup rw\n", []string{"/sys/fs/cgroup space/job", "/sys/fs/cgroup space"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := spawnCgroupDirectories(tt.membership, tt.mounts)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("directories = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpawnCgroupDirectories_RejectsUnresolvablePaths(t *testing.T) {
	for _, membership := range []string{"garbage", "0::/other/job\n", "0::/jobs/../../outside\n", "0::/\n", "0::/child\n"} {
		_, err := spawnCgroupDirectories(membership, "29 23 0:26 /jobs /sys/fs/cgroup rw - cgroup2 cgroup rw\n")
		if err == nil {
			t.Errorf("membership %q unexpectedly resolved", membership)
		}
	}
}

func fixtureReader(files map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		value, ok := files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return []byte(value), nil
	}
}

func TestLinuxSpawnDiagnostics_CgroupReadFailuresRemainExplicit(t *testing.T) {
	for _, files := range []map[string]string{
		{"/proc/self/cgroup": "0::/\n"},
		{"/proc/self/cgroup": "0::/\n", "/proc/self/mountinfo": "not mountinfo"},
	} {
		got := linuxSpawnDiagnostics(fixtureReader(files), 42)
		for _, want := range []string{"cgroup=unavailable", "pids.current=unavailable", "pids.max=unavailable", "pids.events=unavailable"} {
			if !strings.Contains(got, want) {
				t.Errorf("diagnostics lack %q: %s", want, got)
			}
		}
	}
}

func TestSpawnCgroupDirectories_CollectsDuplicateMountsIndependentlyOfOrder(t *testing.T) {
	restricted := "29 23 0:26 /jobs/one /restricted rw - cgroup2 cgroup rw\n"
	wide := "30 23 0:26 / /wide rw - cgroup2 cgroup rw\n"
	duplicate := "31 23 0:26 / /wide rw - cgroup2 cgroup rw\n"
	want := []string{"/restricted", "/wide", "/wide/jobs", "/wide/jobs/one"}
	for _, mounts := range []string{restricted + wide + duplicate, duplicate + wide + restricted} {
		got, err := spawnCgroupDirectories("0::/jobs/one\n", mounts)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("directories = %q, want %q", got, want)
		}
	}
}

func TestLinuxSpawnDiagnostics_UnreadableMountDoesNotHideReadableMount(t *testing.T) {
	files := map[string]string{
		"/proc/self/cgroup":      "0::/\n",
		"/proc/self/mountinfo":   "29 23 0:26 / /unreadable rw - cgroup2 cgroup rw\n30 23 0:26 / /readable rw - cgroup2 cgroup rw\n",
		"/readable/pids.current": "17\n", "/readable/pids.max": "100\n", "/readable/pids.events": "max 3\n",
	}
	got := linuxSpawnDiagnostics(fixtureReader(files), 42)
	for _, want := range []string{`cgroup="/unreadable" pids.current=unavailable`, `cgroup="/readable" pids.current=17 pids.max=100 pids.events=max 3`} {
		if !strings.Contains(got, want) {
			t.Errorf("diagnostics lack %q: %s", want, got)
		}
	}
}

func TestSpawnProcField_RequiresLimitNameBoundary(t *testing.T) {
	data := []byte("Max processesExtra 999 999 processes\nMax processes\t200 300 processes\n")
	if got := spawnProcField(data, nil, "Max processes"); got != "200 300 processes" {
		t.Fatalf("Max processes = %q, want exact field value", got)
	}
}

func TestSpawnCgroupDirectories_RejectsInheritedNamespaceMountAboveRoot(t *testing.T) {
	for _, mountRoot := range []string{"/..", "/../outside", "/jobs/../../outside"} {
		mounts := "29 23 0:26 " + mountRoot + " /sys/fs/cgroup rw - cgroup2 cgroup rw\n"
		if got, err := spawnCgroupDirectories("0::/child\n", mounts); err == nil {
			t.Errorf("mount root %q resolved to %q; want explicit unavailable", mountRoot, got)
		}
	}
}
