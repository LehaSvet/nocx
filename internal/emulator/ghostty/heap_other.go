//go:build !linux || !cgo || vtmusl

package ghostty

import "unsafe"

func cHeapSupported() bool { return false }

func cHeapInUseBytes() uint64                 { return 0 }
func cHeapProbeAlloc(uintptr) unsafe.Pointer  { return nil }
func cHeapProbeFree(unsafe.Pointer)           {}
func cHeapProbeTouch(unsafe.Pointer, uintptr) {}
