package rbd

// #cgo LDFLAGS: -lrbd
// /* force XSI-complaint strerror_r() */
// #define _POSIX_C_SOURCE 200112L
// #undef _GNU_SOURCE
// #include <errno.h>
// #include <stdlib.h>
// #include <rados/librados.h>
// #include <rbd/librbd.h>
import "C"

import (
	"errors"
	"io"
	"time"
	"unsafe"

	"github.com/ceph/go-ceph/internal/cutil"
	"github.com/ceph/go-ceph/internal/retry"
	ts "github.com/ceph/go-ceph/internal/timespec"
	"github.com/ceph/go-ceph/rados"
)

const (
	// Image.Seek() constants:

	// SeekSet is used with Seek to absolutely position the file.
	SeekSet = int(C.SEEK_SET)
	// SeekCur is used with Seek to position the file relatively to the current
	// position.
	SeekCur = int(C.SEEK_CUR)
	// SeekEnd is used with Seek to position the file relatively to the end.
	SeekEnd = int(C.SEEK_END)
)

// bits for Image.validate() and Snapshot.validate()
const (
	imageNeedsName uint32 = 1 << iota
	imageNeedsIOContext
	imageIsOpen
	imageIsNotOpen
	snapshotNeedsName

	// NoSnapshot indicates that no snapshot name is in use (see OpenImage)
	NoSnapshot = ""
)

// Timespec is a public type for the internal C 'struct timespec'
type Timespec ts.Timespec

// revive:disable:var-naming old-yet-exported public api

// ImageInfo represents the status information for an image.
type ImageInfo struct {
	Size              uint64
	Obj_size          uint64
	Num_objs          uint64
	Order             int
	Block_name_prefix string
}

// revive:enable:var-naming

// SnapInfo represents the status information for a snapshot.
type SnapInfo struct {
	Id   uint64
	Size uint64
	Name string
}

// Locker provides info about a client that is locking an image.
type Locker struct {
	Client string
	Cookie string
	Addr   string
}

// Image is a handle for an RBD image.
type Image struct {
	io.Reader
	io.Writer
	io.Seeker
	io.ReaderAt
	io.WriterAt
	name   string
	offset int64
	ioctx  *rados.IOContext
	image  C.rbd_image_t
}

// TrashInfo contains information about trashed RBDs.
type TrashInfo struct {
	Id               string    // Id string, required to remove / restore trashed RBDs.
	Name             string    // Original name of trashed RBD.
	DeletionTime     time.Time // Date / time at which the RBD was moved to the trash.
	DefermentEndTime time.Time // Date / time after which the trashed RBD may be permanently deleted.
}

// cephIoctx returns a ceph rados_ioctx_t given a go-ceph rados IOContext.
func cephIoctx(radosIoctx *rados.IOContext) C.rados_ioctx_t {
	p := radosIoctx.Pointer()
	if p == nil {
		panic("invalid IOContext pointer")
	}
	return C.rados_ioctx_t(p)
}

// test if a bit is set in the given value
func hasBit(value, bit uint32) bool {
	return (value & bit) == bit
}

// validate the attributes listed in the req bitmask, and return an error in
// case the attribute is not set
func (image *Image) validate(req uint32) error {
	if hasBit(req, imageNeedsName) && image.name == "" {
		return ErrNoName
	} else if hasBit(req, imageNeedsIOContext) && image.ioctx == nil {
		return ErrNoIOContext
	} else if hasBit(req, imageIsOpen) && image.image == nil {
		return ErrImageNotOpen
	} else if hasBit(req, imageIsNotOpen) && image.image != nil {
		return ErrImageIsOpen
	}

	return nil
}

// Version returns the major, minor, and patch level of the librbd library.
func Version() (int, int, int) {
	var cMajor, cMinor, cPatch C.int
	C.rbd_version(&cMajor, &cMinor, &cPatch)
	return int(cMajor), int(cMinor), int(cPatch)
}

// GetImage gets a reference to a previously created rbd image.
func GetImage(ioctx *rados.IOContext, name string) *Image {
	return &Image{
		ioctx: ioctx,
		name:  name,
	}
}

// Create a new rbd image.
//
// Implements:
//
//	int rbd_create(rados_ioctx_t io, const char *name, uint64_t size, int *order);
//
// Also implements (for backward compatibility):
//
//	int rbd_create2(rados_ioctx_t io, const char *name, uint64_t size,
//	        uint64_t features, int *order);
//	int rbd_create3(rados_ioctx_t io, const char *name, uint64_t size,
//	      uint64_t features, int *order,
//	      uint64_t stripe_unit, uint64_t stripe_count);
func Create(ioctx *rados.IOContext, name string, size uint64, order int,
	args ...uint64) (image *Image, err error) {
	var ret C.int

	switch len(args) {
	case 3:
		return Create3(ioctx, name, size, args[0], order, args[1],
			args[2])
	case 1:
		return Create2(ioctx, name, size, args[0], order)
	case 0:
		cOrder := C.int(order)
		cName := C.CString(name)

		defer C.free(unsafe.Pointer(cName))

		ret = C.rbd_create(cephIoctx(ioctx),
			cName, C.uint64_t(size), &cOrder)
	default:
		return nil, errors.New("Wrong number of argument")
	}

	if ret < 0 {
		return nil, rbdError(ret)
	}

	return &Image{
		ioctx: ioctx,
		name:  name,
	}, nil
}

// Create2 creates a new rbd image using provided features.
//
// Implements:
//
//	int rbd_create2(rados_ioctx_t io, const char *name, uint64_t size,
//	        uint64_t features, int *order);
func Create2(ioctx *rados.IOContext, name string, size uint64, features uint64,
	order int) (image *Image, err error) {
	var ret C.int

	cOrder := C.int(order)
	cName := C.CString(name)

	defer C.free(unsafe.Pointer(cName))

	ret = C.rbd_create2(cephIoctx(ioctx), cName,
		C.uint64_t(size), C.uint64_t(features), &cOrder)
	if ret < 0 {
		return nil, rbdError(ret)
	}

	return &Image{
		ioctx: ioctx,
		name:  name,
	}, nil
}

// Create3 creates a new rbd image using provided features and stripe
// parameters.
//
// Implements:
//
//	int rbd_create3(rados_ioctx_t io, const char *name, uint64_t size,
//	      uint64_t features, int *order,
//	      uint64_t stripe_unit, uint64_t stripe_count);
func Create3(ioctx *rados.IOContext, name string, size uint64, features uint64,
	order int, stripeUnit uint64, stripeCount uint64) (image *Image, err error) {
	var ret C.int

	cOrder := C.int(order)
	cName := C.CString(name)

	defer C.free(unsafe.Pointer(cName))

	ret = C.rbd_create3(cephIoctx(ioctx), cName,
		C.uint64_t(size), C.uint64_t(features), &cOrder,
		C.uint64_t(stripeUnit), C.uint64_t(stripeCount))
	if ret < 0 {
		return nil, rbdError(ret)
	}

	return &Image{
		ioctx: ioctx,
		name:  name,
	}, nil
}

// Clone a new rbd image from a snapshot.
//
// Implements:
//
//	int rbd_clone(rados_ioctx_t p_ioctx, const char *p_name,
//	         const char *p_snapname, rados_ioctx_t c_ioctx,
//	         const char *c_name, uint64_t features, int *c_order);
func (image *Image) Clone(snapname string, cIoctx *rados.IOContext, cName string, features uint64, order int) (*Image, error) {
	if err := image.validate(imageNeedsIOContext); err != nil {
		return nil, err
	}

	cOrder := C.int(order)
	cParentName := C.CString(image.name)
	cParentSnapName := C.CString(snapname)
	cCloneName := C.CString(cName)

	defer C.free(unsafe.Pointer(cParentName))
	defer C.free(unsafe.Pointer(cParentSnapName))
	defer C.free(unsafe.Pointer(cCloneName))

	ret := C.rbd_clone(
		cephIoctx(image.ioctx),
		cParentName,
		cParentSnapName,
		cephIoctx(cIoctx),
		cCloneName,
		C.uint64_t(features),
		&cOrder)
	if ret < 0 {
		return nil, rbdError(ret)
	}

	return &Image{
		ioctx: cIoctx,
		name:  cName,
	}, nil
}

// Remove the specified rbd image.
//
// Implements:
//
//	int rbd_remove(rados_ioctx_t io, const char *name);
func (image *Image) Remove() error {
	if err := image.validate(imageNeedsIOContext | imageNeedsName | imageIsNotOpen); err != nil {
		return err
	}
	return RemoveImage(image.ioctx, image.name)
}

// Trash will move an image into the RBD trash, where it will be protected (i.e., salvageable) for
// at least the specified delay.
func (image *Image) Trash(delay time.Duration) error {
	if err := image.validate(imageNeedsIOContext | imageNeedsName); err != nil {
		return err
	}

	cName := C.CString(image.name)
	defer C.free(unsafe.Pointer(cName))

	return getError(C.rbd_trash_move(cephIoctx(image.ioctx), cName,
		C.uint64_t(delay.Seconds())))
}

// Rename an rbd image.
//
// Implements:
//
//	int rbd_rename(rados_ioctx_t src_io_ctx, const char *srcname, const char *destname);
func (image *Image) Rename(destname string) error {
	if err := image.validate(imageNeedsIOContext | imageNeedsName); err != nil {
		return err
	}

	cSrcName := C.CString(image.name)
	cDestName := C.CString(destname)

	defer C.free(unsafe.Pointer(cSrcName))
	defer C.free(unsafe.Pointer(cDestName))

	err := rbdError(C.rbd_rename(cephIoctx(image.ioctx),
		cSrcName, cDestName))
	if err == 0 {
		image.name = destname
		return nil
	}
	return err
}

// Open the rbd image.
//
// Deprecated: use OpenImage and OpenImageReadOnly instead
func (image *Image) Open(args ...interface{}) error {
	if err := image.validate(imageNeedsIOContext | imageNeedsName); err != nil {
		return err
	}

	var (
		snapName string
		readOnly bool
	)
	for _, arg := range args {
		switch t := arg.(type) {
		case string:
			snapName = t
		case bool:
			readOnly = t
		default:
			return errors.New("Unexpected argument")
		}
	}

	var (
		tmp *Image
		err error
	)
	if readOnly {
		tmp, err = OpenImageReadOnly(image.ioctx, image.name, snapName)
	} else {
		tmp, err = OpenImage(image.ioctx, image.name, snapName)
	}
	if err != nil {
		return err
	}

	image.image = tmp.image
	return nil
}

// Close an open rbd image.
//
// Implements:
//
//	int rbd_close(rbd_image_t image);
func (image *Image) Close() error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	if ret := C.rbd_close(image.image); ret != 0 {
		return rbdError(ret)
	}

	image.image = nil
	return nil
}

// Resize an rbd image.
//
// Implements:
//
//	int rbd_resize(rbd_image_t image, uint64_t size);
func (image *Image) Resize(size uint64) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	return getError(C.rbd_resize(image.image, C.uint64_t(size)))
}

// Stat an rbd image.
//
// Implements:
//
//	int rbd_stat(rbd_image_t image, rbd_image_info_t *info, size_t infosize);
func (image *Image) Stat() (info *ImageInfo, err error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, err
	}

	var cStat C.rbd_image_info_t

	if ret := C.rbd_stat(image.image, &cStat, C.size_t(unsafe.Sizeof(info))); ret < 0 {
		return info, rbdError(ret)
	}

	return &ImageInfo{
		Size:              uint64(cStat.size),
		Obj_size:          uint64(cStat.obj_size),
		Num_objs:          uint64(cStat.num_objs),
		Order:             int(cStat.order),
		Block_name_prefix: C.GoString((*C.char)(&cStat.block_name_prefix[0]))}, nil
}

// IsOldFormat returns true if the rbd image uses the old format.
//
// Implements:
//
//	int rbd_get_old_format(rbd_image_t image, uint8_t *old);
func (image *Image) IsOldFormat() (bool, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return false, err
	}

	var cOldFormat C.uint8_t
	ret := C.rbd_get_old_format(image.image,
		&cOldFormat)
	if ret < 0 {
		return false, rbdError(ret)
	}

	return cOldFormat != 0, nil
}

// GetSize returns the size of the rbd image.
//
// Implements:
//
//	int rbd_size(rbd_image_t image, uint64_t *size);
func (image *Image) GetSize() (size uint64, err error) {
	if err := image.validate(imageIsOpen); err != nil {
		return 0, err
	}

	if ret := C.rbd_get_size(image.image, (*C.uint64_t)(&size)); ret < 0 {
		return 0, rbdError(ret)
	}

	return size, nil
}

// GetStripeUnit returns the stripe-unit value for the rbd image.
//
// Implements:
//
//	int rbd_get_stripe_unit(rbd_image_t image, uint64_t *stripe_unit);
func (image *Image) GetStripeUnit() (uint64, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return 0, err
	}

	var stripeUnit uint64
	if ret := C.rbd_get_stripe_unit(image.image, (*C.uint64_t)(&stripeUnit)); ret < 0 {
		return 0, rbdError(ret)
	}

	return stripeUnit, nil
}

// GetStripeCount returns the stripe-count value for the rbd image.
//
// Implements:
//
//	int rbd_get_stripe_count(rbd_image_t image, uint64_t *stripe_count);
func (image *Image) GetStripeCount() (uint64, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return 0, err
	}

	var stripeCount uint64
	if ret := C.rbd_get_stripe_count(image.image, (*C.uint64_t)(&stripeCount)); ret < 0 {
		return 0, rbdError(ret)
	}

	return stripeCount, nil
}

// GetOverlap returns the overlapping bytes between the rbd image and its
// parent.
//
// Implements:
//
//	int rbd_get_overlap(rbd_image_t image, uint64_t *overlap);
func (image *Image) GetOverlap() (overlap uint64, err error) {
	if err := image.validate(imageIsOpen); err != nil {
		return 0, err
	}

	if ret := C.rbd_get_overlap(image.image, (*C.uint64_t)(&overlap)); ret < 0 {
		return overlap, rbdError(ret)
	}

	return overlap, nil
}

// Copy one rbd image to another.
//
// Implements:
//
//	int rbd_copy(rbd_image_t image, rados_ioctx_t dest_io_ctx, const char *destname);
func (image *Image) Copy(ioctx *rados.IOContext, destname string) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	} else if ioctx == nil {
		return ErrNoIOContext
	} else if len(destname) == 0 {
		return ErrNoName
	}

	cDestName := C.CString(destname)
	defer C.free(unsafe.Pointer(cDestName))

	return getError(C.rbd_copy(image.image,
		cephIoctx(ioctx), cDestName))
}

// Copy2 copies one rbd image to another, using an image handle.
//
// Implements:
//
//	int rbd_copy2(rbd_image_t src, rbd_image_t dest);
func (image *Image) Copy2(dest *Image) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	} else if err := dest.validate(imageIsOpen); err != nil {
		return err
	}

	return getError(C.rbd_copy2(image.image, dest.image))
}

// DeepCopy an rbd image to a new image with specific options.
//
// Implements:
//
//	int rbd_deep_copy(rbd_image_t src, rados_ioctx_t dest_io_ctx,
//	        const char *destname, rbd_image_options_t dest_opts);
func (image *Image) DeepCopy(ioctx *rados.IOContext, destname string, rio *ImageOptions) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}
	if ioctx == nil {
		return ErrNoIOContext
	}
	if destname == "" {
		return ErrNoName
	}
	if rio == nil {
		return rbdError(C.EINVAL)
	}

	cDestname := C.CString(destname)
	defer C.free(unsafe.Pointer(cDestname))

	ret := C.rbd_deep_copy(image.image, cephIoctx(ioctx), cDestname,
		C.rbd_image_options_t(rio.options))
	return getError(ret)
}

// Flatten removes snapshot references from the image.
//
// Implements:
//
//	int rbd_flatten(rbd_image_t image);
func (image *Image) Flatten() error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	return getError(C.rbd_flatten(image.image))
}

// ListLockers returns a list of clients that have locks on the image.
//
// Impelemnts:
//
//	ssize_t rbd_list_lockers(rbd_image_t image, int *exclusive,
//	            char *tag, size_t *tag_len,
//	            char *clients, size_t *clients_len,
//	            char *cookies, size_t *cookies_len,
//	            char *addrs, size_t *addrs_len);
func (image *Image) ListLockers() (tag string, lockers []Locker, err error) {
	if err := image.validate(imageIsOpen); err != nil {
		return "", nil, err
	}

	var cExclusive C.int
	var cTagLen, cClientsLen, cCookiesLen, cAddrsLen C.size_t
	var cLockerCount C.ssize_t

	C.rbd_list_lockers(image.image, &cExclusive,
		nil, (*C.size_t)(&cTagLen),
		nil, (*C.size_t)(&cClientsLen),
		nil, (*C.size_t)(&cCookiesLen),
		nil, (*C.size_t)(&cAddrsLen))

	// no locker held on rbd image when either c_clients_len,
	// c_cookies_len or c_addrs_len is *0*, so just quickly returned
	if int(cClientsLen) == 0 || int(cCookiesLen) == 0 ||
		int(cAddrsLen) == 0 {
		lockers = make([]Locker, 0)
		return "", lockers, nil
	}

	tagBuf := make([]byte, cTagLen)
	clientsBuf := make([]byte, cClientsLen)
	cookiesBuf := make([]byte, cCookiesLen)
	addrsBuf := make([]byte, cAddrsLen)

	cLockerCount = C.rbd_list_lockers(image.image, &cExclusive,
		(*C.char)(unsafe.Pointer(&tagBuf[0])), (*C.size_t)(&cTagLen),
		(*C.char)(unsafe.Pointer(&clientsBuf[0])), (*C.size_t)(&cClientsLen),
		(*C.char)(unsafe.Pointer(&cookiesBuf[0])), (*C.size_t)(&cCookiesLen),
		(*C.char)(unsafe.Pointer(&addrsBuf[0])), (*C.size_t)(&cAddrsLen))

	// rbd_list_lockers returns negative value for errors
	// and *0* means no locker held on rbd image.
	// but *0* is unexpected here because first rbd_list_lockers already
	// dealt with no locker case
	if int(cLockerCount) <= 0 {
		return "", nil, rbdError(cLockerCount)
	}

	clients := cutil.SplitSparseBuffer(clientsBuf)
	cookies := cutil.SplitSparseBuffer(cookiesBuf)
	addrs := cutil.SplitSparseBuffer(addrsBuf)

	lockers = make([]Locker, cLockerCount)
	for i := 0; i < int(cLockerCount); i++ {
		lockers[i] = Locker{Client: clients[i],
			Cookie: cookies[i],
			Addr:   addrs[i]}
	}

	return string(tagBuf), lockers, nil
}

// LockExclusive acquires an exclusive lock on the rbd image.
//
// Implements:
//
//	int rbd_lock_exclusive(rbd_image_t image, const char *cookie);
func (image *Image) LockExclusive(cookie string) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	cCookie := C.CString(cookie)
	defer C.free(unsafe.Pointer(cCookie))

	return getError(C.rbd_lock_exclusive(image.image, cCookie))
}

// LockShared acquires a shared lock on the rbd image.
//
// Implements:
//
//	int rbd_lock_shared(rbd_image_t image, const char *cookie, const char *tag);
func (image *Image) LockShared(cookie string, tag string) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	cCookie := C.CString(cookie)
	cTag := C.CString(tag)
	defer C.free(unsafe.Pointer(cCookie))
	defer C.free(unsafe.Pointer(cTag))

	return getError(C.rbd_lock_shared(image.image, cCookie, cTag))
}

// Unlock releases a lock on the image.
//
// Implements:
//
//	int rbd_lock_shared(rbd_image_t image, const char *cookie, const char *tag);
func (image *Image) Unlock(cookie string) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	cCookie := C.CString(cookie)
	defer C.free(unsafe.Pointer(cCookie))

	return getError(C.rbd_unlock(image.image, cCookie))
}

// BreakLock forces the release of a lock held by another client.
//
// Implements:
//
//	int rbd_break_lock(rbd_image_t image, const char *client, const char *cookie);
func (image *Image) BreakLock(client string, cookie string) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	cClient := C.CString(client)
	cCookie := C.CString(cookie)
	defer C.free(unsafe.Pointer(cClient))
	defer C.free(unsafe.Pointer(cCookie))

	return getError(C.rbd_break_lock(image.image, cClient, cCookie))
}

// Read data from the image. The length of the read is determined by the length
// of the buffer slice. The position of the read is determined by an internal
// offset which is not safe in concurrent code. Prefer ReadAt when possible.
//
// Implements:
//
//	ssize_t rbd_read(rbd_image_t image, uint64_t ofs, size_t len,
//	                 char *buf);
func (image *Image) Read(data []byte) (int, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return 0, err
	}

	if len(data) == 0 {
		return 0, nil
	}

	ret := int(C.rbd_read(
		image.image,
		(C.uint64_t)(image.offset),
		(C.size_t)(len(data)),
		(*C.char)(unsafe.Pointer(&data[0]))))

	if ret < 0 {
		return 0, rbdError(ret)
	}

	image.offset += int64(ret)
	if ret < len(data) {
		return ret, io.EOF
	}

	return ret, nil
}

// Write data to an image. The length of the write is determined by the length of
// the buffer slice. The position of the write is determined by an internal
// offset which is not safe in concurrent code. Prefer WriteAt when possible.
//
// Implements:
//
//	ssize_t rbd_write(rbd_image_t image, uint64_t ofs, size_t len,
//	                  const char *buf);
func (image *Image) Write(data []byte) (n int, err error) {
	if err := image.validate(imageIsOpen); err != nil {
		return 0, err
	}

	ret := int(C.rbd_write(image.image, C.uint64_t(image.offset),
		C.size_t(len(data)), (*C.char)(unsafe.Pointer(&data[0]))))

	if ret >= 0 {
		image.offset += int64(ret)
	}

	if ret != len(data) {
		err = rbdError(-C.EPERM)
	}

	return ret, err
}

// Seek updates the internal file position for the current image.
func (image *Image) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case SeekSet:
		image.offset = offset
	case SeekCur:
		image.offset += offset
	case SeekEnd:
		stats, err := image.Stat()
		if err != nil {
			return 0, err
		}
		image.offset = int64(stats.Size) - offset
	default:
		return 0, errors.New("Wrong value for whence")
	}
	return image.offset, nil
}

// Discard the supplied range from the image. The supplied range will be read
// as zeros once Discard returns. The discarded range will no longer take up
// space.
//
// Implements:
//
//	int rbd_discard(rbd_image_t image, uint64_t ofs, uint64_t len);
func (image *Image) Discard(ofs uint64, length uint64) (int, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return 0, err
	}

	ret := C.rbd_discard(image.image, C.uint64_t(ofs), C.uint64_t(length))
	if ret < 0 {
		return 0, rbdError(ret)
	}

	return int(ret), nil
}

// ReadAt copies data from the image into the supplied buffer.
func (image *Image) ReadAt(data []byte, off int64) (int, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return 0, err
	}

	if len(data) == 0 {
		return 0, nil
	}

	ret := int(C.rbd_read(
		image.image,
		(C.uint64_t)(off),
		(C.size_t)(len(data)),
		(*C.char)(unsafe.Pointer(&data[0]))))

	if ret < 0 {
		return 0, rbdError(ret)
	}

	if ret < len(data) {
		return ret, io.EOF
	}

	return ret, nil
}

// WriteAt copies data from the supplied buffer to the image.
func (image *Image) WriteAt(data []byte, off int64) (n int, err error) {
	if err := image.validate(imageIsOpen); err != nil {
		return 0, err
	}

	if len(data) == 0 {
		return 0, nil
	}

	ret := int(C.rbd_write(image.image, C.uint64_t(off),
		C.size_t(len(data)), (*C.char)(unsafe.Pointer(&data[0]))))

	if ret != len(data) {
		err = rbdError(-C.EPERM)
	}

	return ret, err
}

// WriteSame repeats writing data from starting point ofs until n bytes have
// been written.
//
// Implements:
//
//	ssize_t rbd_writesame(rbd_image_t image, uint64_t ofs, size_t len,
//	                      const char *buf, size_t data_len, int op_flags);
func (image *Image) WriteSame(ofs, n uint64, data []byte, flags rados.OpFlags) (int64, error) {
	var err error

	if err = image.validate(imageIsOpen); err != nil {
		return 0, err
	}

	if len(data) == 0 {
		return 0, nil
	}

	ret := C.rbd_writesame(image.image,
		C.uint64_t(ofs),
		C.size_t(n),
		(*C.char)(unsafe.Pointer(&data[0])),
		C.size_t(len(data)),
		C.int(flags))
	if ret < 0 {
		err = getError(C.int(ret))
	}

	return int64(ret), err
}

// Flush all cached writes to storage.
//
// Implements:
//
//	int rbd_flush(rbd_image_t image);
func (image *Image) Flush() error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	return getError(C.rbd_flush(image.image))
}

// GetSnapshotNames returns more than just the names of snapshots
// associated with the rbd image.
//
// Implements:
//
//	int rbd_snap_list(rbd_image_t image, rbd_snap_info_t *snaps, int *max_snaps);
func (image *Image) GetSnapshotNames() (snaps []SnapInfo, err error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, err
	}

	var cMaxSnaps C.int

	ret := C.rbd_snap_list(image.image, nil, &cMaxSnaps)

	cSnaps := make([]C.rbd_snap_info_t, cMaxSnaps)
	snaps = make([]SnapInfo, cMaxSnaps)

	ret = C.rbd_snap_list(image.image,
		&cSnaps[0], &cMaxSnaps)
	if ret < 0 {
		return nil, rbdError(ret)
	}

	for i, s := range cSnaps {
		snaps[i] = SnapInfo{Id: uint64(s.id),
			Size: uint64(s.size),
			Name: C.GoString(s.name)}
	}

	C.rbd_snap_list_end(&cSnaps[0])
	return snaps[:len(snaps)-1], nil
}

// GetId returns the internal image ID string.
//
// Implements:
//
//	int rbd_get_id(rbd_image_t image, char *id, size_t id_len);
func (image *Image) GetId() (string, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return "", err
	}
	var (
		err error
		buf []byte
	)
	retry.WithSizes(1, 8192, func(size int) retry.Hint {
		buf = make([]byte, size)
		ret := C.rbd_get_id(
			image.image,
			(*C.char)(unsafe.Pointer(&buf[0])),
			C.size_t(size))
		err = getErrorIfNegative(ret)
		return retry.DoubleSize.If(err == errRange)
	})
	if err != nil {
		return "", err
	}
	id := C.GoString((*C.char)(unsafe.Pointer(&buf[0])))
	return id, nil

}

// GetName returns the image name.
func (image *Image) GetName() string {
	return image.name
}

// SetSnapshot updates the rbd image (not the Snapshot) such that the snapshot
// is the source of readable data.
//
// Implements:
//
//	int rbd_snap_set(rbd_image_t image, const char *snapname);
func (image *Image) SetSnapshot(snapname string) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}

	cSnapName := C.CString(snapname)
	defer C.free(unsafe.Pointer(cSnapName))

	return getError(C.rbd_snap_set(image.image, cSnapName))
}

// GetTrashList returns a slice of TrashInfo structs, containing information about all RBD images
// currently residing in the trash.
func GetTrashList(ioctx *rados.IOContext) ([]TrashInfo, error) {
	var (
		err     error
		count   C.size_t
		entries []C.rbd_trash_image_info_t
	)
	retry.WithSizes(32, 10240, func(size int) retry.Hint {
		count = C.size_t(size)
		entries = make([]C.rbd_trash_image_info_t, count)
		ret := C.rbd_trash_list(cephIoctx(ioctx), &entries[0], &count)
		err = getErrorIfNegative(ret)
		return retry.Size(int(count)).If(err == errRange)
	})
	if err != nil {
		return nil, err
	}
	// Free rbd_trash_image_info_t pointers
	defer C.rbd_trash_list_cleanup(&entries[0], count)

	trashList := make([]TrashInfo, count)
	for i, ti := range entries[:count] {
		trashList[i] = TrashInfo{
			Id:               C.GoString(ti.id),
			Name:             C.GoString(ti.name),
			DeletionTime:     time.Unix(int64(ti.deletion_time), 0),
			DefermentEndTime: time.Unix(int64(ti.deferment_end_time), 0),
		}
	}
	return trashList, nil
}

// TrashRemove permanently deletes the trashed RBD with the specified id.
func TrashRemove(ioctx *rados.IOContext, id string, force bool) error {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))

	return getError(C.rbd_trash_remove(cephIoctx(ioctx), cid, C.bool(force)))
}

// TrashRestore restores the trashed RBD with the specified id back to the pool from whence it
// came, with the specified new name.
func TrashRestore(ioctx *rados.IOContext, id, name string) error {
	cid := C.CString(id)
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cid))
	defer C.free(unsafe.Pointer(cName))

	return getError(C.rbd_trash_restore(cephIoctx(ioctx), cid, cName))
}

// OpenImage will open an existing rbd image by name and snapshot name,
// returning a new opened image. Pass the NoSnapshot sentinel value as the
// snapName to explicitly indicate that no snapshot name is being provided.
//
// Implements:
//
//	int rbd_open(rados_ioctx_t io, const char *name,
//	             rbd_image_t *image, const char *snap_name);
func OpenImage(ioctx *rados.IOContext, name, snapName string) (*Image, error) {
	if ioctx == nil {
		return nil, ErrNoIOContext
	}
	if name == "" {
		return nil, ErrNoName
	}

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	var cSnapName *C.char
	if snapName != NoSnapshot {
		cSnapName = C.CString(snapName)
		defer C.free(unsafe.Pointer(cSnapName))
	}

	var cImage C.rbd_image_t
	ret := C.rbd_open(
		cephIoctx(ioctx),
		cName,
		&cImage,
		cSnapName)

	if ret != 0 {
		return nil, getError(ret)
	}

	return &Image{
		ioctx: ioctx,
		name:  name,
		image: cImage,
	}, nil
}

// OpenImageReadOnly will open an existing rbd image by name and snapshot name,
// returning a new opened-for-read image.  Pass the NoSnapshot sentinel value
// as the snapName to explicitly indicate that no snapshot name is being
// provided.
//
// Implements:
//
//	int rbd_open_read_only(rados_ioctx_t io, const char *name,
//	                       rbd_image_t *image, const char *snap_name);
func OpenImageReadOnly(ioctx *rados.IOContext, name, snapName string) (*Image, error) {
	if ioctx == nil {
		return nil, ErrNoIOContext
	}
	if name == "" {
		return nil, ErrNoName
	}

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	var cSnapName *C.char
	if snapName != NoSnapshot {
		cSnapName = C.CString(snapName)
		defer C.free(unsafe.Pointer(cSnapName))
	}

	var cImage C.rbd_image_t
	ret := C.rbd_open_read_only(
		cephIoctx(ioctx),
		cName,
		&cImage,
		cSnapName)

	if ret != 0 {
		return nil, getError(ret)
	}

	return &Image{
		ioctx: ioctx,
		name:  name,
		image: cImage,
	}, nil
}

// OpenImageById will open an existing rbd image by ID and snapshot name,
// returning a new opened image. Pass the NoSnapshot sentinel value as the
// snapName to explicitly indicate that no snapshot name is being provided.
// Error handling will fail & segfault unless compiled with a version of ceph
// that fixes https://tracker.ceph.com/issues/43178
//
// Implements:
//
//	int rbd_open_by_id(rados_ioctx_t io, const char *id,
//	                   rbd_image_t *image, const char *snap_name);
func OpenImageById(ioctx *rados.IOContext, id, snapName string) (*Image, error) {
	if ioctx == nil {
		return nil, ErrNoIOContext
	}
	if id == "" {
		return nil, ErrNoName
	}

	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))

	var cSnapName *C.char
	if snapName != NoSnapshot {
		cSnapName = C.CString(snapName)
		defer C.free(unsafe.Pointer(cSnapName))
	}

	var cImage C.rbd_image_t
	ret := C.rbd_open_by_id(
		cephIoctx(ioctx),
		cid,
		&cImage,
		cSnapName)

	if ret != 0 {
		return nil, getError(ret)
	}

	return &Image{
		ioctx: ioctx,
		image: cImage,
	}, nil
}

// OpenImageByIdReadOnly will open an existing rbd image by ID and snapshot
// name, returning a new opened-for-read image. Pass the NoSnapshot sentinel
// value as the snapName to explicitly indicate that no snapshot name is being
// provided.
// Error handling will fail & segfault unless compiled with a version of ceph
// that fixes https://tracker.ceph.com/issues/43178
//
// Implements:
//
//	int rbd_open_by_id_read_only(rados_ioctx_t io, const char *id,
//	                             rbd_image_t *image, const char *snap_name);
func OpenImageByIdReadOnly(ioctx *rados.IOContext, id, snapName string) (*Image, error) {
	if ioctx == nil {
		return nil, ErrNoIOContext
	}
	if id == "" {
		return nil, ErrNoName
	}

	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))

	var cSnapName *C.char
	if snapName != NoSnapshot {
		cSnapName = C.CString(snapName)
		defer C.free(unsafe.Pointer(cSnapName))
	}

	var cImage C.rbd_image_t
	ret := C.rbd_open_by_id_read_only(
		cephIoctx(ioctx),
		cid,
		&cImage,
		cSnapName)

	if ret != 0 {
		return nil, getError(ret)
	}

	return &Image{
		ioctx: ioctx,
		image: cImage,
	}, nil
}

// CreateImage creates a new rbd image using provided image options.
//
// Implements:
//
//	int rbd_create4(rados_ioctx_t io, const char *name, uint64_t size,
//	               rbd_image_options_t opts);
func CreateImage(ioctx *rados.IOContext, name string, size uint64, rio *ImageOptions) error {
	if ioctx == nil {
		return ErrNoIOContext
	}
	if name == "" {
		return ErrNoName
	}
	if rio == nil {
		return rbdError(C.EINVAL)
	}

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	ret := C.rbd_create4(cephIoctx(ioctx), cName,
		C.uint64_t(size), C.rbd_image_options_t(rio.options))
	return getError(ret)
}

// RemoveImage removes the specified rbd image.
//
// Implements:
//
//	int rbd_remove(rados_ioctx_t io, const char *name);
func RemoveImage(ioctx *rados.IOContext, name string) error {
	if ioctx == nil {
		return ErrNoIOContext
	}
	if name == "" {
		return ErrNoName
	}

	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	return getError(C.rbd_remove(cephIoctx(ioctx), cName))
}

// CloneImage creates a clone of the image from the named snapshot in the
// provided io-context with the given name and image options.
//
// Implements:
//
//	int rbd_clone3(rados_ioctx_t p_ioctx, const char *p_name,
//	               const char *p_snapname, rados_ioctx_t c_ioctx,
//	               const char *c_name, rbd_image_options_t c_opts);
func CloneImage(ioctx *rados.IOContext, parentName, snapName string,
	destctx *rados.IOContext, name string, rio *ImageOptions) error {

	if rio == nil {
		return rbdError(C.EINVAL)
	}

	cParentName := C.CString(parentName)
	defer C.free(unsafe.Pointer(cParentName))
	cParentSnapName := C.CString(snapName)
	defer C.free(unsafe.Pointer(cParentSnapName))
	cCloneName := C.CString(name)
	defer C.free(unsafe.Pointer(cCloneName))

	ret := C.rbd_clone3(
		cephIoctx(ioctx),
		cParentName,
		cParentSnapName,
		cephIoctx(destctx),
		cCloneName,
		C.rbd_image_options_t(rio.options))
	return getError(ret)
}

// CloneFromImage creates a clone of the image from the named snapshot in the
// provided io-context with the given name and image options.
// This function is a convenience wrapper around CloneImage to support cloning
// from an existing Image.
func CloneFromImage(parent *Image, snapName string,
	destctx *rados.IOContext, name string, rio *ImageOptions) error {

	if err := parent.validate(imageNeedsIOContext); err != nil {
		return err
	}
	return CloneImage(parent.ioctx, parent.name, snapName, destctx, name, rio)
}
