//go:build nocx_e2e_shell_override

package artifacts

import "github.com/shady2k/nocx/internal/helper/deploy"

func localArtifactSource() deploy.ArtifactSource { return e2eLocalSource{} }
