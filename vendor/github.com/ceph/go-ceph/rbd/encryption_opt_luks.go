//go:build !octopus && !pacific && !quincy && ceph_preview

package rbd

// #cgo LDFLAGS: -lrbd
// /* force XSI-complaint strerror_r() */
// #define _POSIX_C_SOURCE 200112L
// #undef _GNU_SOURCE
// #include <stdlib.h>
// #include <rbd/librbd.h>
import "C"

import (
	"unsafe"
)

// EncryptionOptionsLUKS generic options for either LUKS v1 or v2. May only be
// used for opening existing images - not valid for the EncryptionFormat call.
type EncryptionOptionsLUKS struct {
	Passphrase []byte
}

func (opts EncryptionOptionsLUKS) allocateEncryptionOptions() cEncryptionData {
	var cOpts C.rbd_encryption_luks_format_options_t
	var retData cEncryptionData
	// CBytes allocates memory. it will be freed when cEncryptionData.free is called
	cOpts.passphrase = (*C.char)(C.CBytes(opts.Passphrase))
	cOpts.passphrase_size = C.size_t(len(opts.Passphrase))
	retData.opts = C.rbd_encryption_options_t(&cOpts)
	retData.optsSize = C.size_t(C.sizeof_rbd_encryption_luks_format_options_t)
	retData.free = func() { C.free(unsafe.Pointer(cOpts.passphrase)) }
	retData.format = C.RBD_ENCRYPTION_FORMAT_LUKS
	return retData
}

func (opts EncryptionOptionsLUKS) writeEncryptionSpec(spec *C.rbd_encryption_spec_t) func() {
	/* only C memory should be attached to spec */
	cPassphrase := (*C.char)(C.CBytes(opts.Passphrase))
	cOptsSize := C.size_t(C.sizeof_rbd_encryption_luks_format_options_t)
	cOpts := (*C.rbd_encryption_luks_format_options_t)(C.malloc(cOptsSize))
	cOpts.passphrase = cPassphrase
	cOpts.passphrase_size = C.size_t(len(opts.Passphrase))

	spec.format = C.RBD_ENCRYPTION_FORMAT_LUKS
	spec.opts = C.rbd_encryption_options_t(cOpts)
	spec.opts_size = cOptsSize
	return func() {
		C.free(unsafe.Pointer(cOpts.passphrase))
		C.free(unsafe.Pointer(cOpts))
	}
}
