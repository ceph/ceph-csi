//go:build ceph_preview
// +build ceph_preview

package rados

// #cgo LDFLAGS: -lrados
// #include <rados/librados.h>
// #include <stdlib.h>
//
import "C"

import (
	"unsafe"
)

// ReadOpOmapGetValsByKeysStep holds the result of the
// GetOmapValuesByKeys read operation.
// Result is valid only after Operate() was called.
type ReadOpOmapGetValsByKeysStep struct {
	// C arguments

	iter  C.rados_omap_iter_t
	prval *C.int

	// Internal state

	// canIterate is only set after the operation is performed and is
	// intended to prevent premature fetching of data.
	canIterate bool
}

func newReadOpOmapGetValsByKeysStep() *ReadOpOmapGetValsByKeysStep {
	s := &ReadOpOmapGetValsByKeysStep{
		prval: (*C.int)(C.malloc(C.sizeof_int)),
	}

	return s
}

func (s *ReadOpOmapGetValsByKeysStep) free() {
	s.canIterate = false
	C.rados_omap_get_end(s.iter)

	C.free(unsafe.Pointer(s.prval))
	s.prval = nil
}

func (s *ReadOpOmapGetValsByKeysStep) update() error {
	err := getError(*s.prval)
	s.canIterate = (err == nil)

	return err
}

// Next gets the next omap key/value pair referenced by
// ReadOpOmapGetValsByKeysStep's internal iterator.
// If there are no more elements to retrieve, (nil, nil) is returned.
// May be called only after Operate() finished.
//  PREVIEW
func (s *ReadOpOmapGetValsByKeysStep) Next() (*OmapKeyValue, error) {
	if !s.canIterate {
		return nil, ErrOperationIncomplete
	}

	var (
		cKey    *C.char
		cVal    *C.char
		cValLen C.size_t
	)

	ret := C.rados_omap_get_next(s.iter, &cKey, &cVal, &cValLen)
	if ret != 0 {
		return nil, getError(ret)
	}

	if cKey == nil {
		// Iterator has reached the end of the list.
		return nil, nil
	}

	return &OmapKeyValue{
		Key:   C.GoString(cKey),
		Value: C.GoBytes(unsafe.Pointer(cVal), C.int(cValLen)),
	}, nil
}

// GetOmapValuesByKeys starts iterating over specific key/value pairs.
//  PREVIEW
//
// Implements:
//  void rados_read_op_omap_get_vals_by_keys(rados_read_op_t read_op,
//                                           char const * const * keys,
//                                           size_t keys_len,
//                                           rados_omap_iter_t * iter,
//                                           int * prval)
func (r *ReadOp) GetOmapValuesByKeys(keys []string) *ReadOpOmapGetValsByKeysStep {
	s := newReadOpOmapGetValsByKeysStep()
	r.steps = append(r.steps, s)

	cKeys := make([]*C.char, len(keys))
	defer func() {
		for _, cKeyPtr := range cKeys {
			C.free(unsafe.Pointer(cKeyPtr))
		}
	}()

	for i, key := range keys {
		cKeys[i] = C.CString(key)
	}

	C.rados_read_op_omap_get_vals_by_keys(
		r.op,
		&cKeys[0],
		C.size_t(len(keys)),
		&s.iter,
		s.prval,
	)

	return s
}
