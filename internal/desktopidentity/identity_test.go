//go:build linux

package desktopidentity_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/desktopidentity"
)

func TestDefaultApplicationIdentity(t *testing.T) {
	for _, scenario := range []string{"missing", "unregistered", "registered"} {
		t.Run(scenario, func(t *testing.T) {
			args := []string{"run"}
			if desktopidentity.ID == "dev.nocx" {
				args = append(args, "-tags=release")
			}
			args = append(args, "./testdata/probe", scenario, desktopidentity.ID)
			cmd := exec.Command("go", args...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("identity probe: %v\n%s", err, output)
			}
			if !strings.Contains(string(output), "PASS "+scenario) {
				t.Fatalf("probe did not confirm %s: %s", scenario, output)
			}
		})
	}
}
