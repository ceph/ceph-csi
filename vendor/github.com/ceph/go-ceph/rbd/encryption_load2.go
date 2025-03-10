//go:build !octopus && !pacific && !quincy && ceph_preview

package rbd

// #cgo LDFLAGS: -lrbd
// /* force XSI-complaint strerror_r() */
// #define _POSIX_C_SOURCE 200112L
// #undef _GNU_SOURCE
// #include <stdlib.h>
// #include <errno.h>
// #include <rbd/librbd.h>
import "C"

import (
	"unsafe"
)

type encryptionOptions2 interface {
	EncryptionOptions
	writeEncryptionSpec(spec *C.rbd_encryption_spec_t) func()
}

func (opts EncryptionOptionsLUKS1) writeEncryptionSpec(spec *C.rbd_encryption_spec_t) func() {
	/* only C memory should be attached to spec */
	cPassphrase := (*C.char)(C.CBytes(opts.Passphrase))
	cOptsSize := C.size_t(C.sizeof_rbd_encryption_luks1_format_options_t)
	cOpts := (*C.rbd_encryption_luks1_format_options_t)(C.malloc(cOptsSize))
	cOpts.alg = C.rbd_encryption_algorithm_t(opts.Alg)
	cOpts.passphrase = cPassphrase
	cOpts.passphrase_size = C.size_t(len(opts.Passphrase))

	spec.format = C.RBD_ENCRYPTION_FORMAT_LUKS1
	spec.opts = C.rbd_encryption_options_t(cOpts)
	spec.opts_size = cOptsSize
	return func() {
		C.free(unsafe.Pointer(cOpts.passphrase))
		C.free(unsafe.Pointer(cOpts))
	}
}

func (opts EncryptionOptionsLUKS2) writeEncryptionSpec(spec *C.rbd_encryption_spec_t) func() {
	/* only C memory should be attached to spec */
	cPassphrase := (*C.char)(C.CBytes(opts.Passphrase))
	cOptsSize := C.size_t(C.sizeof_rbd_encryption_luks2_format_options_t)
	cOpts := (*C.rbd_encryption_luks2_format_options_t)(C.malloc(cOptsSize))
	cOpts.alg = C.rbd_encryption_algorithm_t(opts.Alg)
	cOpts.passphrase = cPassphrase
	cOpts.passphrase_size = C.size_t(len(opts.Passphrase))

	spec.format = C.RBD_ENCRYPTION_FORMAT_LUKS2
	spec.opts = C.rbd_encryption_options_t(cOpts)
	spec.opts_size = cOptsSize
	return func() {
		C.free(unsafe.Pointer(cOpts.passphrase))
		C.free(unsafe.Pointer(cOpts))
	}
}

// EncryptionLoad2 enables IO on an open encrypted image. Multiple encryption
// option values can be passed to this call in a slice. For more information
// about how items in the slice are applied to images, and possibly ancestor
// images refer to the documentation in the C api for rbd_encryption_load2.
//
// Implements:
//
//	int rbd_encryption_load2(rbd_image_t image,
//	                         const rbd_encryption_spec_t *specs,
//	                         size_t spec_count);
func (image *Image) EncryptionLoad2(opts []EncryptionOptions) error {
	if image.image == nil {
		return ErrImageNotOpen
	}
	for _, o := range opts {
		if _, ok := o.(encryptionOptions2); !ok {
			// this should not happen unless someone adds a new type
			// implementing EncryptionOptions but fails to add a
			// writeEncryptionSpec such that the type is not also implementing
			// encryptionOptions2.
			return getError(C.EINVAL)
		}
	}

	length := len(opts)
	cspecs := make([]C.rbd_encryption_spec_t, length)
	freeFuncs := make([]func(), length)

	for idx, option := range opts {
		f := option.(encryptionOptions2).writeEncryptionSpec(&cspecs[idx])
		freeFuncs[idx] = f
	}
	defer func() {
		for _, f := range freeFuncs {
			f()
		}
	}()

	ret := C.rbd_encryption_load2(
		image.image,
		(*C.rbd_encryption_spec_t)(unsafe.Pointer(&cspecs[0])),
		C.size_t(length))
	return getError(ret)
}
