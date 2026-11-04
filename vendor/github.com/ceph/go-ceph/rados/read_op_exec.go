package rados

/*
#cgo LDFLAGS: -lrados
#include <stdlib.h>
#include <rados/librados.h>

typedef struct {
	char *out_buf;
	size_t out_len;
	int prval;
} read_op_exec_out;
*/
import "C"

import (
	"runtime"
	"unsafe"
)

// ReadOpExecStep - step for exec operation code.
type ReadOpExecStep struct {
	inBuffPtr *C.char
	inBuffLen C.size_t

	// written by librados after Exec returns, so not Go memory
	cOut *C.read_op_exec_out

	// copied from cOut by update, as Release frees cOut
	outBuffPtr    *C.char
	outBuffLen    C.size_t
	canReadOutput bool
}

// newExecStepOp - init new *execStepOp.
func newReadOpExecStep(in []byte) *ReadOpExecStep {
	es := &ReadOpExecStep{
		cOut: (*C.read_op_exec_out)(C.malloc(C.sizeof_read_op_exec_out)),
	}
	*es.cOut = C.read_op_exec_out{}

	if len(in) > 0 {
		es.inBuffPtr = (*C.char)(unsafe.Pointer(&in[0]))
		es.inBuffLen = C.size_t(len(in))
	}

	runtime.SetFinalizer(es, func(es *ReadOpExecStep) {
		if es != nil {
			es.free()
			es.freeBuffer()
			es = nil
		}
	})
	return es
}

func (es *ReadOpExecStep) free() {
	C.free(unsafe.Pointer(es.cOut))
	es.cOut = nil
}

// freeBuffer - releases C allocated buffer. It is separated from es.free() because lifespan of C allocated buffer is
// longer than lifespan of read operation.
func (es *ReadOpExecStep) freeBuffer() {
	if es.outBuffPtr != nil {
		C.free(unsafe.Pointer(es.outBuffPtr))
		es.outBuffPtr = nil
		es.canReadOutput = false
	}
}

// update - update state operation.
func (es *ReadOpExecStep) update() error {
	es.outBuffPtr = es.cOut.out_buf
	es.outBuffLen = es.cOut.out_len
	err := getErrorIfNegative(es.cOut.prval)
	es.canReadOutput = err == nil
	return err
}

// Bytes returns the result of the executed command as a byte slice.
func (es *ReadOpExecStep) Bytes() ([]byte, error) {
	if !es.canReadOutput {
		return nil, ErrOperationIncomplete
	}

	return C.GoBytes(unsafe.Pointer(es.outBuffPtr), C.int(es.outBuffLen)), nil
}

// Exec executes an OSD class method on an object.
// See rados_exec() in the RADOS C api documentation for a general description.
//
// Implements:
//
//	void rados_read_op_exec(rados_read_op_t read_op,
//	                        const char *cls,
//	                        const char *method,
//	                        const char *in_buf,
//	                        size_t in_len,
//	                        char **out_buf,
//	                        size_t *out_len,
//	                        int *prval);
func (r *ReadOp) Exec(clsName, method string, in []byte) *ReadOpExecStep {
	cClsName := C.CString(clsName)
	defer C.free(unsafe.Pointer(cClsName))

	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))

	es := newReadOpExecStep(in)
	r.steps = append(r.steps, es)
	C.rados_read_op_exec(
		r.op,
		cClsName,
		cMethod,
		es.inBuffPtr,
		es.inBuffLen,
		&es.cOut.out_buf,
		&es.cOut.out_len,
		&es.cOut.prval,
	)

	return es
}
