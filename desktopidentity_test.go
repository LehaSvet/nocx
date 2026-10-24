package main

import (
	"os"
	"strings"
	"testing"
)

// The startup service must remain first: Wails constructs its GtkApplication
// before services and registers it after them. Guard both the lifecycle seam
// and the GTK Wayland/X11 program-name seam, which are independent identities.
func TestLinuxDesktopIdentityWiring(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(source)
	identity := strings.Index(s, "application.NewService(&desktopIdentityService{})")
	window := strings.Index(s, "application.NewService(wailsApp)")
	if identity < 0 || window < identity {
		t.Fatal("desktop identity must be the first startup service")
	}
	if !strings.Contains(s, "ProgramName: desktopidentity.ID") {
		t.Fatal("GTK Wayland program name must match desktop identity")
	}
}

func TestAppImageDesktopIdentity(t *testing.T) {
	source, err := os.ReadFile("scripts/appimage/package-appimage.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(source)
	if !strings.Contains(s, "cat > AppDir/dev.nocx.desktop") || !strings.Contains(s, "--desktop-file AppDir/dev.nocx.desktop") {
		t.Fatal("AppImage must install the dev.nocx desktop launcher")
	}
}
