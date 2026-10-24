//go:build !release

package desktopidentity

import "testing"

func TestDevelopmentID(t *testing.T) {
	if ID != "dev.nocx-dev" {
		t.Fatalf("development desktop ID = %q", ID)
	}
}
