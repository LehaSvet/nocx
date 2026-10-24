//go:build linux

// Package desktopidentity owns the Linux desktop application's identity.
package desktopidentity

/*
#cgo pkg-config: gio-2.0
#include <gio/gio.h>
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// SetDefault sets the GtkApplication's ID before registration. Wails beta.9
// constructs the default application before ServiceStartup and registers it
// afterwards, in linuxApp.run. Its ProgramName option only sets g_set_prgname;
// it cannot replace the hardcoded org.wails prefix.
//
// This uses GLib's public pre-registration API, not Wails internals or a fork:
// https://docs.gtk.org/gio/method.Application.set_application_id.html
// Refuse a missing or already registered application so lifecycle drift fails
// startup instead of silently shipping a different Wayland identity.
func SetDefault() error {
	app := C.g_application_get_default()
	if app == nil {
		return errors.New("desktop identity: no default application")
	}
	if C.g_application_get_is_registered(app) != 0 {
		return errors.New("desktop identity: application is already registered")
	}
	id := C.CString(ID)
	defer C.free(unsafe.Pointer(id))
	C.g_application_set_application_id(app, id)
	if got := C.GoString(C.g_application_get_application_id(app)); got != ID {
		return fmt.Errorf("desktop identity: application ID is %q, want %q", got, ID)
	}
	return nil
}
