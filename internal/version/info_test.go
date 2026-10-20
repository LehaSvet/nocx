package version

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// THE POINT OF THE WHOLE DESCRIPTOR: no field is ever empty. The About page
// exists so a person filing a bug has something to read and quote, and a blank
// row tells them nothing while looking exactly like a row that has not loaded
// (nocx-8bbp). Absence is spelled out here, once, so no surface has to decide
// what to draw for a missing value.
func TestInfoNamesEveryFieldEvenWhenNothingWasStamped(t *testing.T) {
	got := Info()
	fields := map[string]string{
		"Version":  got.Version,
		"Commit":   got.Commit,
		"Date":     got.Date,
		"Go":       got.Go,
		"Wails":    got.Wails,
		"Platform": got.Platform,
	}
	for name, value := range fields {
		if strings.TrimSpace(value) == "" {
			t.Errorf("%s is empty; every field must say something, including that it is unknown", name)
		}
	}
}

// A `go test` binary passes no -ldflags, so this asserts the un-stamped build
// exactly as TestLinkTimeDefaults does for the raw vars — and that the
// descriptor reports it as a development build rather than presenting "dev" as
// though it were a release number.
func TestInfoReportsAnUnstampedBuildAsDevelopment(t *testing.T) {
	got := Info()
	if got.Version != "dev" {
		t.Fatalf("Version = %q, want the unstamped default %q", got.Version, "dev")
	}
	if !got.Development {
		t.Fatal("an unstamped build must report Development, or the page shows 'dev' as a release")
	}
}

// The two the process knows about itself rather than being told: they must be
// read, never restated. A hand-maintained platform string is the constant this
// whole descriptor exists to avoid.
func TestInfoReadsTheRuntimeItIsRunningOn(t *testing.T) {
	got := Info()
	if got.Go != runtime.Version() {
		t.Fatalf("Go = %q, want %q", got.Go, runtime.Version())
	}
	if want := runtime.GOOS + "/" + runtime.GOARCH; got.Platform != want {
		t.Fatalf("Platform = %q, want %q", got.Platform, want)
	}
}

// A stamped build reports what it was stamped with. Asserted through the same
// vars the linker writes, because that is the only way in — and restored after,
// since they are package state a later test would otherwise inherit.
func TestInfoReportsAStampedBuildAsARelease(t *testing.T) {
	defer func(v, c, d string) { Version, Commit, Date = v, c, d }(Version, Commit, Date)
	Version, Commit, Date = "0.3.1", "abc1234", "2026-08-20T10:00:00Z"

	got := Info()
	if got.Version != "0.3.1" || got.Commit != "abc1234" || got.Date != "2026-08-20T10:00:00Z" {
		t.Fatalf("stamped build reported as %+v", got)
	}
	if got.Development {
		t.Fatal("a stamped build is not a development build")
	}
}

// THE SHIPPED APP'S SHELL VERSION IS STAMPED, NOT READ (nocx-1ebrs.11).
//
// app.about is answered by the coordinator, which is built without cgo and
// without a window, so the module list it was linked from has no wails entry —
// the fallback in wailsVersion cannot answer there, and the About page showed
// "Wails: unknown" in the shipped 0.6.0. The release build stamps the pinned
// version instead, and a stamped value has to win over a module list that will
// never have one.
func TestInfoPrefersTheStampedShellVersion(t *testing.T) {
	defer func(w string) { Wails = w }(Wails)
	Wails = "v3.0.0-beta.9"

	if got := Info().Wails; got != "v3.0.0-beta.9" {
		t.Fatalf("Wails = %q, want the stamped %q", got, "v3.0.0-beta.9")
	}

	// An empty stamp is "nothing stamped it", not a value: the module list is
	// then the answer, and a build that links the shell keeps working.
	Wails = ""
	if got := Info().Wails; strings.TrimSpace(got) == "" {
		t.Fatal("an unstamped build must still say something rather than nothing")
	}
}

// AND THE RELEASE BUILD HAS TO DO THE STAMPING. The value above is only ever
// set by a link flag, and a link flag the workflow does not pass is a field the
// shipped binary cannot know — which is the defect, not a missing test.
//
// Asserted against the workflow itself because that file is the only thing
// that runs the linker for a release: every LDFLAGS chain that stamps Version
// must stamp Wails in the same chain, so a second build step cannot be added
// later with half the stamps.
func TestReleaseBuildStampsTheShellVersion(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "release.yml")
	raw, err := os.ReadFile(path) //nolint:gosec // this repository's own tracked workflow file
	if err != nil {
		t.Fatalf("the release workflow is the only place a release is linked: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	chains := 0
	for i, line := range lines {
		if !strings.Contains(line, `LDFLAGS="`) || !strings.Contains(line, "internal/version.Version=") {
			continue
		}
		chains++
		stamped := false
		for j := i; j < len(lines) && j < i+5; j++ {
			if strings.Contains(lines[j], "internal/version.Wails=") {
				stamped = true
				break
			}
			// The chain ends where the build begins: a stamp written after
			// `go build` is a stamp that build never saw.
			if strings.Contains(lines[j], "go build") || strings.Contains(lines[j], "go list") {
				break
			}
		}
		if !stamped {
			t.Errorf("release.yml line %d builds a release without stamping version.Wails", i+1)
		}
	}
	if chains == 0 {
		t.Fatal("no LDFLAGS chain stamps the version; the check above proved nothing")
	}
}
