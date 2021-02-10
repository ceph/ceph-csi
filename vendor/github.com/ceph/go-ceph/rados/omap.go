package rados

/*
#cgo LDFLAGS: -lrados
#include <stdlib.h>
#include <rados/librados.h>

typedef void* voidptr;

*/
import "C"

import (
	"runtime"
	"unsafe"
)

const (
	ptrSize   = C.sizeof_voidptr
	sizeTSize = C.sizeof_size_t
)

// setOmapStep is a write op step. It holds C memory used in the operation.
type setOmapStep struct {
	withRefs
	withoutUpdate

	// C arguments
	cKeys    **C.char
	cValues  **C.char
	cLengths *C.size_t
	cNum     C.size_t
}

func newSetOmapStep(pairs map[string][]byte) *setOmapStep {

	maplen := C.size_t(len(pairs))
	cKeys := C.malloc(maplen * ptrSize)
	cValues := C.malloc(maplen * ptrSize)
	cLengths := C.malloc(maplen * sizeTSize)

	sos := &setOmapStep{
		cKeys:    (**C.char)(cKeys),
		cValues:  (**C.char)(cValues),
		cLengths: (*C.size_t)(cLengths),
		cNum:     C.size_t(len(pairs)),
	}
	sos.add(cKeys)
	sos.add(cValues)
	sos.add(cLengths)

	var i uintptr
	for key, value := range pairs {
		// key
		ck := C.CString(key)
		sos.add(unsafe.Pointer(ck))
		ckp := (**C.char)(unsafe.Pointer(uintptr(cKeys) + i*ptrSize))
		*ckp = ck

		// value and its length
		cvp := (**C.char)(unsafe.Pointer(uintptr(cValues) + i*ptrSize))
		vlen := C.size_t(len(value))
		if vlen > 0 {
			cv := C.CBytes(value)
			sos.add(cv)
			*cvp = (*C.char)(cv)
		} else {
			*cvp = nil
		}

		clp := (*C.size_t)(unsafe.Pointer(uintptr(cLengths) + i*ptrSize))
		*clp = vlen

		i++
	}

	runtime.SetFinalizer(sos, opStepFinalizer)
	return sos
}

func (sos *setOmapStep) free() {
	sos.cKeys = nil
	sos.cValues = nil
	sos.cLengths = nil
	sos.withRefs.free()
}

// OmapKeyValue items are returned by the GetOmapStep's Next call.
type OmapKeyValue struct {
	Key   string
	Value []byte
}

// GetOmapStep values are used to get the results of an GetOmapValues call
// on a WriteOp. Until the Operate method of the WriteOp is called the Next
// call will return an error. After Operate is called, the Next call will
// return valid results.
//
// The life cycle of the GetOmapStep is bound to the ReadOp, if the ReadOp
// Release method is called the public methods of the step must no longer be
// used and may return errors.
type GetOmapStep struct {
	// inputs:
	startAfter   string
	filterPrefix string
	maxReturn    uint64

	// arguments:
	cStartAfter   *C.char
	cFilterPrefix *C.char

	// C returned data:
	iter C.rados_omap_iter_t
	more C.uchar
	rval C.int

	// internal state:

	// canIterate is only set after the operation is performed and is
	// intended to prevent premature fetching of data
	canIterate bool
}

func newGetOmapStep(startAfter, filterPrefix string, maxReturn uint64) *GetOmapStep {
	gos := &GetOmapStep{
		startAfter:    startAfter,
		filterPrefix:  filterPrefix,
		maxReturn:     maxReturn,
		cStartAfter:   C.CString(startAfter),
		cFilterPrefix: C.CString(filterPrefix),
	}
	runtime.SetFinalizer(gos, opStepFinalizer)
	return gos
}

func (gos *GetOmapStep) free() {
	gos.canIterate = false
	if gos.iter != nil {
		C.rados_omap_get_end(gos.iter)
	}
	gos.iter = nil
	gos.more = 0
	gos.rval = 0
	C.free(unsafe.Pointer(gos.cStartAfter))
	gos.cStartAfter = nil
	C.free(unsafe.Pointer(gos.cFilterPrefix))
	gos.cFilterPrefix = nil
}

func (gos *GetOmapStep) update() error {
	err := getError(gos.rval)
	gos.canIterate = (err == nil)
	return err
}

// Next returns the next key value pair or nil if iteration is exhausted.
func (gos *GetOmapStep) Next() (*OmapKeyValue, error) {
	if !gos.canIterate {
		return nil, ErrOperationIncomplete
	}
	var (
		cKey *C.char
		cVal *C.char
		cLen C.size_t
	)
	ret := C.rados_omap_get_next(gos.iter, &cKey, &cVal, &cLen)
	if ret != 0 {
		return nil, getError(ret)
	}
	if cKey == nil {
		return nil, nil
	}
	return &OmapKeyValue{
		Key:   C.GoString(cKey),
		Value: C.GoBytes(unsafe.Pointer(cVal), C.int(cLen)),
	}, nil
}

// More returns true if there are more matching keys available.
func (gos *GetOmapStep) More() bool {
	// tad bit hacky, but go can't automatically convert from
	// unsigned char to bool
	return gos.more != 0
}

// removeOmapKeysStep is a write operation step used to track state, especially
// C memory, across the setup and use of a WriteOp.
type removeOmapKeysStep struct {
	withRefs
	withoutUpdate

	// arguments:
	cKeys **C.char
	cNum  C.size_t
}

func newRemoveOmapKeysStep(keys []string) *removeOmapKeysStep {
	cKeys := C.malloc(C.size_t(len(keys)) * ptrSize)
	roks := &removeOmapKeysStep{
		cKeys: (**C.char)(cKeys),
		cNum:  C.size_t(len(keys)),
	}
	roks.add(cKeys)

	i := 0
	for _, key := range keys {
		ckp := (**C.char)(unsafe.Pointer(uintptr(cKeys) + uintptr(i)*ptrSize))
		*ckp = C.CString(key)
		roks.add(unsafe.Pointer(*ckp))
		i++
	}

	runtime.SetFinalizer(roks, opStepFinalizer)
	return roks
}

func (roks *removeOmapKeysStep) free() {
	roks.cKeys = nil
	roks.withRefs.free()
}

// SetOmap appends the map `pairs` to the omap `oid`
func (ioctx *IOContext) SetOmap(oid string, pairs map[string][]byte) error {
	op := CreateWriteOp()
	defer op.Release()
	op.SetOmap(pairs)
	return op.operateCompat(ioctx, oid)
}

// OmapListFunc is the type of the function called for each omap key
// visited by ListOmapValues
type OmapListFunc func(key string, value []byte)

// ListOmapValues iterates over the keys and values in an omap by way of
// a callback function.
//
// `startAfter`: iterate only on the keys after this specified one
// `filterPrefix`: iterate only on the keys beginning with this prefix
// `maxReturn`: iterate no more than `maxReturn` key/value pairs
// `listFn`: the function called at each iteration
func (ioctx *IOContext) ListOmapValues(oid string, startAfter string, filterPrefix string, maxReturn int64, listFn OmapListFunc) error {

	op := CreateReadOp()
	defer op.Release()
	gos := op.GetOmapValues(startAfter, filterPrefix, uint64(maxReturn))
	err := op.operateCompat(ioctx, oid)
	if err != nil {
		return err
	}

	for {
		kv, err := gos.Next()
		if err != nil {
			return err
		}
		if kv == nil {
			break
		}
		listFn(kv.Key, kv.Value)
	}
	return nil
}

// GetOmapValues fetches a set of keys and their values from an omap and returns then as a map
// `startAfter`: retrieve only the keys after this specified one
// `filterPrefix`: retrieve only the keys beginning with this prefix
// `maxReturn`: retrieve no more than `maxReturn` key/value pairs
func (ioctx *IOContext) GetOmapValues(oid string, startAfter string, filterPrefix string, maxReturn int64) (map[string][]byte, error) {
	omap := map[string][]byte{}

	err := ioctx.ListOmapValues(
		oid, startAfter, filterPrefix, maxReturn,
		func(key string, value []byte) {
			omap[key] = value
		},
	)

	return omap, err
}

// GetAllOmapValues fetches all the keys and their values from an omap and returns then as a map
// `startAfter`: retrieve only the keys after this specified one
// `filterPrefix`: retrieve only the keys beginning with this prefix
// `iteratorSize`: internal number of keys to fetch during a read operation
func (ioctx *IOContext) GetAllOmapValues(oid string, startAfter string, filterPrefix string, iteratorSize int64) (map[string][]byte, error) {
	omap := map[string][]byte{}
	omapSize := 0

	for {
		err := ioctx.ListOmapValues(
			oid, startAfter, filterPrefix, iteratorSize,
			func(key string, value []byte) {
				omap[key] = value
				startAfter = key
			},
		)

		if err != nil {
			return omap, err
		}

		// End of omap
		if len(omap) == omapSize {
			break
		}

		omapSize = len(omap)
	}

	return omap, nil
}

// RmOmapKeys removes the specified `keys` from the omap `oid`
func (ioctx *IOContext) RmOmapKeys(oid string, keys []string) error {
	op := CreateWriteOp()
	defer op.Release()
	op.RmOmapKeys(keys)
	return op.operateCompat(ioctx, oid)
}

// CleanOmap clears the omap `oid`
func (ioctx *IOContext) CleanOmap(oid string) error {
	op := CreateWriteOp()
	defer op.Release()
	op.CleanOmap()
	return op.operateCompat(ioctx, oid)
}
