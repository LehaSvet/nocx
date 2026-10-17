//go:build linux && cgo && !vtmusl

package ghosttytest

/*
#include <malloc.h>
#include <stdlib.h>

static size_t nocxHeapInUse(void) {
    struct mallinfo2 info = mallinfo2();
    return info.uordblks + info.hblkhd;
}
static void nocxHeapProbeTouch(void *ptr, size_t size) {
    unsigned char *bytes = (unsigned char *)ptr;
    for (size_t i = 0; i < size; i++) bytes[i] = (unsigned char)i;
}
*/
import "C"

import "unsafe"

// HeapSupported reports whether the Linux glibc mallinfo2 measurement exists.
func HeapSupported() bool { return true }

// HeapInUseBytes reports glibc bytes currently allocated, including separately
// mmap'd allocation blocks, independent of kernel resident-page reclaim.
func HeapInUseBytes() uint64 { return uint64(C.nocxHeapInUse()) }

// HeapProbeAlloc returns a retained libc allocation for instrument controls.
func HeapProbeAlloc(size uintptr) unsafe.Pointer { return C.malloc(C.size_t(size)) }

// HeapProbeTouch touches each byte so the allocation is fully committed.
func HeapProbeTouch(ptr unsafe.Pointer, size uintptr) {
	C.nocxHeapProbeTouch(ptr, C.size_t(size))
}

// HeapProbeFree releases a HeapProbeAlloc result.
func HeapProbeFree(ptr unsafe.Pointer) { C.free(ptr) }
