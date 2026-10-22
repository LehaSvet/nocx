//go:build nocx_e2e_shell_override

package artifacts

// The e2e-only local helper is separate from the local helper shipped by
// default builds. Its dedicated build tag enables NOCX_HELPER_SHELL solely
// for the first-command shell regression tests.
import (
	"embed"
	"errors"

	"github.com/shady2k/nocx/internal/helper/deploy"
)

var ErrE2ELocalArtifactsNotBuilt = errors.New("deploy: e2e local helper artifact not built (run make helper-local-e2e)")

//go:embed all:bin/local-e2e
var e2eLocalArtifactsFS embed.FS

var e2eLocalArtifactsByPlatform map[deploy.Platform]artifact

func init() {
	e2eLocalArtifactsByPlatform = artifactsInDir(e2eLocalArtifactsFS, "bin/local-e2e")
}

type e2eLocalSource struct{}

func (e2eLocalSource) Artifact(p deploy.Platform) ([]byte, string, error) {
	if a, ok := e2eLocalArtifactsByPlatform[p]; ok {
		return a.compressed, a.contentHash, nil
	}
	return nil, "", ErrE2ELocalArtifactsNotBuilt
}
