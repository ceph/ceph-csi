package rados

// #cgo LDFLAGS: -lrados
// #include <rados/librados.h>
// #include <stdlib.h>
//
import "C"

import (
	"unsafe"
)

// SetXattr sets an xattr.
//
// Implements:
//
//	void rados_write_op_setxattr(rados_write_op_t write_op,
//	                             const char * name,
//	                             const char * value,
//	                             size_t value_len)
func (w *WriteOp) SetXattr(name string, value []byte) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	var cValue *C.char
	if len(value) > 0 {
		cValue = (*C.char)(unsafe.Pointer(&value[0]))
	}

	C.rados_write_op_setxattr(
		w.op,
		cName,
		cValue,
		C.size_t(len(value)),
	)
}
