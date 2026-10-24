//go:build linux

package main

/*
#cgo pkg-config: gio-2.0
#include <gio/gio.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"os"
	"strings"
	"unsafe"

	"github.com/shady2k/nocx/internal/desktopidentity"
)

func main() {
	scenario, expected := os.Args[1], os.Args[2]
	if scenario == "missing" {
		err := desktopidentity.SetDefault()
		requireMessage(err, "no default", "missing application was not refused")
	} else {
		name := C.CString("org.wails.nocx")
		defer C.free(unsafe.Pointer(name))
		app := C.g_application_new(name, C.G_APPLICATION_NON_UNIQUE)
		defer C.g_object_unref(C.gpointer(app))
		require(C.g_application_get_default() == app, "new application is not default")
		if scenario == "registered" {
			require(C.g_application_register(app, nil, nil) != 0, "registration failed")
			err := desktopidentity.SetDefault()
			requireMessage(err, "registered", "registered application was not refused")
			require(C.GoString(C.g_application_get_application_id(app)) == "org.wails.nocx", "registered ID changed")
		} else {
			require(desktopidentity.SetDefault() == nil, "identity was not applied")
			require(C.GoString(C.g_application_get_application_id(app)) == expected, "default application ID differs from build ID")
			require(C.g_application_register(app, nil, nil) != 0, "registration failed")
			require(C.GoString(C.g_application_get_application_id(app)) == expected, "registration changed ID")
		}
	}
	fmt.Println("PASS", scenario)
}

func require(ok bool, message string) {
	if !ok {
		fmt.Fprintln(os.Stderr, message)
		os.Exit(1)
	}
}

func requireMessage(err error, part, message string) {
	require(err != nil && strings.Contains(fmt.Sprint(err), part), message)
}
