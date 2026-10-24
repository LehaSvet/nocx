//go:build release

package desktopidentity

import "testing"

func TestReleaseID(t *testing.T) {
	if ID != "dev.nocx" {
		t.Fatalf("release desktop ID = %q", ID)
	}
}
