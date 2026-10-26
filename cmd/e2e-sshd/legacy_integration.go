package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shady2k/nocx/internal/log"
	"github.com/shady2k/nocx/internal/shellintegration"
)

// plantLegacyIntegration derives a synthetic pre-fix install from the real
// installer. No private rc file or account data is copied into the fixture.
// The production writer owns the gate spelling. Only the retired accept
// semantics are simulated: generation 43 required an echoed bearer, which the
// current kernel must never send. Keeping the load guard tests the startup
// suppression independently of the remote retirement.
func plantLegacyIntegration(home string) error {
	if err := shellintegration.New(log.From(context.Background())).EnsureInstalled(home); err != nil {
		return err
	}
	for _, kind := range []string{"bash", "zsh"} {
		target := filepath.Join(home, ".nocx", "shell-integration."+kind)
		// #nosec G304 -- the disposable fixture home and installer-owned name.
		current, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		stamp := "__nocx_script_version=" + shellintegration.ScriptVersion()
		if strings.Count(string(current), stamp) != 1 {
			return fmt.Errorf("fixture integration version marker missing")
		}
		old := strings.Replace(string(current), stamp, "__nocx_script_version=43", 1)
		// This declaration is deliberately inert unless the historical load
		// guard failed. The generated script keeps all other current hooks.
		accept := `*'"evt":"accept"'*) : ;;`
		retired := `*'"evt":"accept"'*'"cap":"'"$__nocx_cap"'"'*) : ;;`
		if strings.Count(old, accept) != 1 {
			return fmt.Errorf("fixture accept shape changed")
		}
		old = strings.Replace(old, accept, retired, 1)
		old = "# Synthetic installer generation 43 with retired accept semantics.\n" + old
		if !strings.Contains(old, "__nocx_script_version=43\n") || !strings.Contains(old, retired) {
			return fmt.Errorf("fixture did not plant the previous generation")
		}
		// #nosec G306 -- fixture integration files have installer data mode.
		if err := os.WriteFile(target, []byte(old), 0o600); err != nil {
			return err
		}
	}
	// Keep the install stamp consistent with the script generation.
	if err := os.WriteFile(filepath.Join(home, ".nocx", "VERSION"), []byte("43\n"), 0o600); err != nil {
		return err
	}
	printLine("LEGACY_INTEGRATION=43")
	printLine("SCRIPT_VERSION=" + shellintegration.ScriptVersion())
	return nil
}
