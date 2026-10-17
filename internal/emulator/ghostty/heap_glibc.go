package ghostty

/*
#include "bridge.h"
*/
import "C"

import "unsafe"

func cHeapSupported() bool                             { return C.nocxCHeapInUse() != 0 }
func cHeapInUseBytes() uint64                          { return uint64(C.nocxCHeapInUse()) }
func cHeapProbeAlloc(size uintptr) unsafe.Pointer      { return C.nocxCHeapProbeAlloc(C.size_t(size)) }
func cHeapProbeFree(ptr unsafe.Pointer)                { C.nocxCHeapProbeFree(ptr) }
func cHeapProbeTouch(ptr unsafe.Pointer, size uintptr) { C.nocxCHeapProbeTouch(ptr, C.size_t(size)) }
