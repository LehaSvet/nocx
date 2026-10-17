//go:build linux && cgo && !vtmusl

package ghostty

/*
#include <malloc.h>
#include <stdlib.h>

static size_t nocx_heap_in_use(void) {
    struct mallinfo2 info = mallinfo2();
    return info.uordblks + info.hblkhd;
}
*/
import "C"

import "unsafe"

// cHeapInUseBytes reads glibc's in-use allocation total. Unlike RSS, this
// value does not move when the kernel reclaims or faults resident pages.
func cHeapSupported() bool { return true }

func cHeapInUseBytes() uint64 { return uint64(C.nocx_heap_in_use()) }

func cHeapProbeAlloc(size uintptr) unsafe.Pointer { return C.malloc(C.size_t(size)) }
func cHeapProbeFree(ptr unsafe.Pointer)           { C.free(ptr) }
func cHeapProbeTouch(ptr unsafe.Pointer, size uintptr) {
	bytes := unsafe.Slice((*byte)(ptr), size)
	for i := range bytes {
		bytes[i] = byte(i)
	}
}
