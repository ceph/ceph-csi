package cutil

import "C"

import (
	"unsafe"
)

// Basic types from C that we can make "public" without too much fuss.

// SizeT wraps size_t from C.
type SizeT C.size_t

// This section contains a bunch of types that are basically just
// unsafe.Pointer but have specific types to help "self document" what the
// underlying pointer is really meant to represent.

// CharPtrPtr is an unsafe pointer wrapping C's `char**`.
type CharPtrPtr unsafe.Pointer

// CharPtr is an unsafe pointer wrapping C's `char*`.
type CharPtr unsafe.Pointer

// SizeTPtr is an unsafe pointer wrapping C's `size_t*`.
type SizeTPtr unsafe.Pointer

// FreeFunc is a wrapper around calls to, or act like, C's free function.
type FreeFunc func(unsafe.Pointer)
