//go:build nocx_e2e_shell_override

package artifacts

import (
	"os"
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/helper/deploy"
)

func TestE2EHelperArtifactIsEmbedded(t *testing.T) {
	want := os.Getenv("NOCX_REQUIRE_E2E_LOCAL_ARTIFACT")
	if want == "" {
		t.Skip("NOCX_REQUIRE_E2E_LOCAL_ARTIFACT is unset")
	}
	goos, goarch, ok := strings.Cut(want, "/")
	if !ok || goos == "" || goarch == "" {
		t.Fatalf("NOCX_REQUIRE_E2E_LOCAL_ARTIFACT=%q must be <goos>/<goarch>", want)
	}
	p := deploy.Platform{GOOS: goos, GOARCH: goarch}
	_, e2eHash, err := e2eLocalSource{}.Artifact(p)
	if err != nil {
		t.Fatalf("e2e local artifact %s is not embedded: %v", want, err)
	}
	_, deployableHash, err := DefaultSource.Artifact(p)
	if err != nil {
		t.Fatalf("deployable artifact %s is not embedded: %v", want, err)
	}
	if e2eHash == deployableHash {
		t.Fatalf("e2e local artifact %s is the deployable artifact; expected separate local shell-override build", want)
	}
}
