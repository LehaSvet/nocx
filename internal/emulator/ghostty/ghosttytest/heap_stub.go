//go:build !linux || !cgo || vtmusl

package ghosttytest

import "unsafe"

// HeapSupported reports whether the Linux glibc mallinfo2 measurement exists.
func HeapSupported() bool { return false }

// HeapInUseBytes is unavailable outside the pinned Linux glibc test build.
func HeapInUseBytes() uint64 { return 0 }

// HeapProbeAlloc is unavailable outside the pinned Linux glibc test build.
func HeapProbeAlloc(uintptr) unsafe.Pointer { return nil }

// HeapProbeTouch is unavailable outside the pinned Linux glibc test build.
func HeapProbeTouch(unsafe.Pointer, uintptr) {}

// HeapProbeFree is unavailable outside the pinned Linux glibc test build.
func HeapProbeFree(unsafe.Pointer) {}
