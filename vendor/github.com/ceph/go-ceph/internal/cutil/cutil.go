package cutil

import "unsafe"

// VoidPtr casts a uintptr value to an unsafe.Pointer value in order to use it
// directly as a void* argument in a C function call.
// CAUTION: NEVER store the result in a variable, or the Go GC could panic.
func VoidPtr(i uintptr) unsafe.Pointer {
	var nullPtr unsafe.Pointer
	// It's not possible to cast uintptr directly to unsafe.Pointer. Therefore we
	// cast a null pointer to uintptr and apply pointer arithmetic on it, which
	// allows us to cast it back to unsafe.Pointer.
	return unsafe.Pointer(uintptr(nullPtr) + i)
}
