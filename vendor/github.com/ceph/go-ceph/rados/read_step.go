package rados

// #include <stdint.h>
import "C"

import (
	"runtime"
	"unsafe"
)

type readStep struct {
	withoutUpdate
	// the c pointer utilizes the Go byteslice data, pinned while librados holds it

	// inputs:
	b []byte

	// arguments:
	cBuffer  *C.char
	cReadLen C.size_t
	cOffset  C.uint64_t

	pinner runtime.Pinner
}

func newReadStep(b []byte, offset uint64) *readStep {
	s := &readStep{
		b:        b,
		cReadLen: C.size_t(len(b)),
		cOffset:  C.uint64_t(offset),
	}
	if len(b) > 0 {
		s.cBuffer = (*C.char)(unsafe.Pointer(&b[0]))
		s.pinner.Pin(s.cBuffer)
	}
	// the runtime panics if it collects a Pinner that still pins memory
	runtime.SetFinalizer(s, opStepFinalizer)
	return s
}

func (s *readStep) free() {
	s.pinner.Unpin()
	runtime.SetFinalizer(s, nil)
}
