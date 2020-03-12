package rados

// #cgo LDFLAGS: -lrados
// #include <errno.h>
// #include <stdlib.h>
// #include <rados/librados.h>
//
// char* nextChunk(char **idx) {
// 	char *copy;
// 	copy = strdup(*idx);
// 	*idx += strlen(*idx) + 1;
// 	return copy;
// }
//
// #if __APPLE__
// #define ceph_time_t __darwin_time_t
// #define ceph_suseconds_t __darwin_suseconds_t
// #elif __GLIBC__
// #define ceph_time_t __time_t
// #define ceph_suseconds_t __suseconds_t
// #else
// #define ceph_time_t time_t
// #define ceph_suseconds_t suseconds_t
// #endif
import "C"

import (
	"syscall"
	"time"
	"unsafe"
)

// CreateOption is passed to IOContext.Create() and should be one of
// CreateExclusive or CreateIdempotent.
type CreateOption int

const (
	// CreateExclusive if used with IOContext.Create() and the object
	// already exists, the function will return an error.
	CreateExclusive = C.LIBRADOS_CREATE_EXCLUSIVE
	// CreateIdempotent if used with IOContext.Create() and the object
	// already exists, the function will not return an error.
	CreateIdempotent = C.LIBRADOS_CREATE_IDEMPOTENT
)

// PoolStat represents Ceph pool statistics.
type PoolStat struct {
	// space used in bytes
	Num_bytes uint64
	// space used in KB
	Num_kb uint64
	// number of objects in the pool
	Num_objects uint64
	// number of clones of objects
	Num_object_clones uint64
	// num_objects * num_replicas
	Num_object_copies              uint64
	Num_objects_missing_on_primary uint64
	// number of objects found on no OSDs
	Num_objects_unfound uint64
	// number of objects replicated fewer times than they should be
	// (but found on at least one OSD)
	Num_objects_degraded uint64
	Num_rd               uint64
	Num_rd_kb            uint64
	Num_wr               uint64
	Num_wr_kb            uint64
}

// ObjectStat represents an object stat information
type ObjectStat struct {
	// current length in bytes
	Size uint64
	// last modification time
	ModTime time.Time
}

// LockInfo represents information on a current Ceph lock
type LockInfo struct {
	NumLockers int
	Exclusive  bool
	Tag        string
	Clients    []string
	Cookies    []string
	Addrs      []string
}

// IOContext represents a context for performing I/O within a pool.
type IOContext struct {
	ioctx C.rados_ioctx_t
}

// Pointer returns a pointer reference to an internal structure.
// This function should NOT be used outside of go-ceph itself.
func (ioctx *IOContext) Pointer() unsafe.Pointer {
	return unsafe.Pointer(ioctx.ioctx)
}

// SetNamespace sets the namespace for objects within this IO context (pool).
// Setting namespace to a empty or zero length string sets the pool to the default namespace.
func (ioctx *IOContext) SetNamespace(namespace string) {
	var c_ns *C.char
	if len(namespace) > 0 {
		c_ns = C.CString(namespace)
		defer C.free(unsafe.Pointer(c_ns))
	}
	C.rados_ioctx_set_namespace(ioctx.ioctx, c_ns)
}

// Create a new object with key oid.
//
// Implements:
//  void rados_write_op_create(rados_write_op_t write_op, int exclusive,
//                             const char* category)
func (ioctx *IOContext) Create(oid string, exclusive CreateOption) error {
	c_oid := C.CString(oid)
	defer C.free(unsafe.Pointer(c_oid))

	op := C.rados_create_write_op()
	C.rados_write_op_create(op, C.int(exclusive), nil)
	ret := C.rados_write_op_operate(op, ioctx.ioctx, c_oid, nil, 0)
	C.rados_release_write_op(op)

	return getRadosError(int(ret))
}

// Write writes len(data) bytes to the object with key oid starting at byte
// offset offset. It returns an error, if any.
func (ioctx *IOContext) Write(oid string, data []byte, offset uint64) error {
	c_oid := C.CString(oid)
	defer C.free(unsafe.Pointer(c_oid))

	dataPointer := unsafe.Pointer(nil)
	if len(data) > 0 {
		dataPointer = unsafe.Pointer(&data[0])
	}

	ret := C.rados_write(ioctx.ioctx, c_oid,
		(*C.char)(dataPointer),
		(C.size_t)(len(data)),
		(C.uint64_t)(offset))

	return getRadosError(int(ret))
}

// WriteFull writes len(data) bytes to the object with key oid.
// The object is filled with the provided data. If the object exists,
// it is atomically truncated and then written. It returns an error, if any.
func (ioctx *IOContext) WriteFull(oid string, data []byte) error {
	c_oid := C.CString(oid)
	defer C.free(unsafe.Pointer(c_oid))

	ret := C.rados_write_full(ioctx.ioctx, c_oid,
		(*C.char)(unsafe.Pointer(&data[0])),
		(C.size_t)(len(data)))
	return getRadosError(int(ret))
}

// Append appends len(data) bytes to the object with key oid.
// The object is appended with the provided data. If the object exists,
// it is atomically appended to. It returns an error, if any.
func (ioctx *IOContext) Append(oid string, data []byte) error {
	c_oid := C.CString(oid)
	defer C.free(unsafe.Pointer(c_oid))

	ret := C.rados_append(ioctx.ioctx, c_oid,
		(*C.char)(unsafe.Pointer(&data[0])),
		(C.size_t)(len(data)))
	return getRadosError(int(ret))
}

// Read reads up to len(data) bytes from the object with key oid starting at byte
// offset offset. It returns the number of bytes read and an error, if any.
func (ioctx *IOContext) Read(oid string, data []byte, offset uint64) (int, error) {
	c_oid := C.CString(oid)
	defer C.free(unsafe.Pointer(c_oid))

	var buf *C.char
	if len(data) > 0 {
		buf = (*C.char)(unsafe.Pointer(&data[0]))
	}

	ret := C.rados_read(
		ioctx.ioctx,
		c_oid,
		buf,
		(C.size_t)(len(data)),
		(C.uint64_t)(offset))

	if ret >= 0 {
		return int(ret), nil
	}
	return 0, getRadosError(int(ret))
}

// Delete deletes the object with key oid. It returns an error, if any.
func (ioctx *IOContext) Delete(oid string) error {
	c_oid := C.CString(oid)
	defer C.free(unsafe.Pointer(c_oid))

	return getRadosError(int(C.rados_remove(ioctx.ioctx, c_oid)))
}

// Truncate resizes the object with key oid to size size. If the operation
// enlarges the object, the new area is logically filled with zeroes. If the
// operation shrinks the object, the excess data is removed. It returns an
// error, if any.
func (ioctx *IOContext) Truncate(oid string, size uint64) error {
	c_oid := C.CString(oid)
	defer C.free(unsafe.Pointer(c_oid))

	return getRadosError(int(C.rados_trunc(ioctx.ioctx, c_oid, (C.uint64_t)(size))))
}

// Destroy informs librados that the I/O context is no longer in use.
// Resources associated with the context may not be freed immediately, and the
// context should not be used again after calling this method.
func (ioctx *IOContext) Destroy() {
	C.rados_ioctx_destroy(ioctx.ioctx)
}

// GetPoolStats returns a set of statistics about the pool associated with this I/O
// context.
//
// Implements:
//  int rados_ioctx_pool_stat(rados_ioctx_t io,
//                            struct rados_pool_stat_t *stats);
func (ioctx *IOContext) GetPoolStats() (stat PoolStat, err error) {
	c_stat := C.struct_rados_pool_stat_t{}
	ret := C.rados_ioctx_pool_stat(ioctx.ioctx, &c_stat)
	if ret < 0 {
		return PoolStat{}, getRadosError(int(ret))
	}
	return PoolStat{
		Num_bytes:                      uint64(c_stat.num_bytes),
		Num_kb:                         uint64(c_stat.num_kb),
		Num_objects:                    uint64(c_stat.num_objects),
		Num_object_clones:              uint64(c_stat.num_object_clones),
		Num_object_copies:              uint64(c_stat.num_object_copies),
		Num_objects_missing_on_primary: uint64(c_stat.num_objects_missing_on_primary),
		Num_objects_unfound:            uint64(c_stat.num_objects_unfound),
		Num_objects_degraded:           uint64(c_stat.num_objects_degraded),
		Num_rd:                         uint64(c_stat.num_rd),
		Num_rd_kb:                      uint64(c_stat.num_rd_kb),
		Num_wr:                         uint64(c_stat.num_wr),
		Num_wr_kb:                      uint64(c_stat.num_wr_kb),
	}, nil
}

// GetPoolName returns the name of the pool associated with the I/O context.
func (ioctx *IOContext) GetPoolName() (name string, err error) {
	buf := make([]byte, 128)
	for {
		ret := C.rados_ioctx_get_pool_name(ioctx.ioctx,
			(*C.char)(unsafe.Pointer(&buf[0])), C.unsigned(len(buf)))
		if ret == -C.ERANGE {
			buf = make([]byte, len(buf)*2)
			continue
		} else if ret < 0 {
			return "", getRadosError(int(ret))
		}
		name = C.GoStringN((*C.char)(unsafe.Pointer(&buf[0])), ret)
		return name, nil
	}
}

// ObjectListFunc is the type of the function called for each object visited
// by ListObjects.
type ObjectListFunc func(oid string)

// ListObjects lists all of the objects in the pool associated with the I/O
// context, and called the provided listFn function for each object, passing
// to the function the name of the object. Call SetNamespace with
// RadosAllNamespaces before calling this function to return objects from all
// namespaces
func (ioctx *IOContext) ListObjects(listFn ObjectListFunc) error {
	var ctx C.rados_list_ctx_t
	ret := C.rados_nobjects_list_open(ioctx.ioctx, &ctx)
	if ret < 0 {
		return getRadosError(int(ret))
	}
	defer func() { C.rados_nobjects_list_close(ctx) }()

	for {
		var c_entry *C.char
		ret := C.rados_nobjects_list_next(ctx, &c_entry, nil, nil)
		if ret == -C.ENOENT {
			return nil
		} else if ret < 0 {
			return getRadosError(int(ret))
		}
		listFn(C.GoString(c_entry))
	}
}

// Stat returns the size of the object and its last modification time
func (ioctx *IOContext) Stat(object string) (stat ObjectStat, err error) {
	var c_psize C.uint64_t
	var c_pmtime C.time_t
	c_object := C.CString(object)
	defer C.free(unsafe.Pointer(c_object))

	ret := C.rados_stat(
		ioctx.ioctx,
		c_object,
		&c_psize,
		&c_pmtime)

	if ret < 0 {
		return ObjectStat{}, getRadosError(int(ret))
	}
	return ObjectStat{
		Size:    uint64(c_psize),
		ModTime: time.Unix(int64(c_pmtime), 0),
	}, nil
}

// GetXattr gets an xattr with key `name`, it returns the length of
// the key read or an error if not successful
func (ioctx *IOContext) GetXattr(object string, name string, data []byte) (int, error) {
	c_object := C.CString(object)
	c_name := C.CString(name)
	defer C.free(unsafe.Pointer(c_object))
	defer C.free(unsafe.Pointer(c_name))

	ret := C.rados_getxattr(
		ioctx.ioctx,
		c_object,
		c_name,
		(*C.char)(unsafe.Pointer(&data[0])),
		(C.size_t)(len(data)))

	if ret >= 0 {
		return int(ret), nil
	}
	return 0, getRadosError(int(ret))
}

// SetXattr sets an xattr for an object with key `name` with value as `data`
func (ioctx *IOContext) SetXattr(object string, name string, data []byte) error {
	c_object := C.CString(object)
	c_name := C.CString(name)
	defer C.free(unsafe.Pointer(c_object))
	defer C.free(unsafe.Pointer(c_name))

	ret := C.rados_setxattr(
		ioctx.ioctx,
		c_object,
		c_name,
		(*C.char)(unsafe.Pointer(&data[0])),
		(C.size_t)(len(data)))

	return getRadosError(int(ret))
}

// ListXattrs lists all the xattrs for an object. The xattrs are returned as a
// mapping of string keys and byte-slice values.
func (ioctx *IOContext) ListXattrs(oid string) (map[string][]byte, error) {
	c_oid := C.CString(oid)
	defer C.free(unsafe.Pointer(c_oid))

	var it C.rados_xattrs_iter_t

	ret := C.rados_getxattrs(ioctx.ioctx, c_oid, &it)
	if ret < 0 {
		return nil, getRadosError(int(ret))
	}
	defer func() { C.rados_getxattrs_end(it) }()
	m := make(map[string][]byte)
	for {
		var c_name, c_val *C.char
		var c_len C.size_t
		defer C.free(unsafe.Pointer(c_name))
		defer C.free(unsafe.Pointer(c_val))

		ret := C.rados_getxattrs_next(it, &c_name, &c_val, &c_len)
		if ret < 0 {
			return nil, getRadosError(int(ret))
		}
		// rados api returns a null name,val & 0-length upon
		// end of iteration
		if c_name == nil {
			return m, nil // stop iteration
		}
		m[C.GoString(c_name)] = C.GoBytes(unsafe.Pointer(c_val), (C.int)(c_len))
	}
}

// RmXattr removes an xattr with key `name` from object `oid`
func (ioctx *IOContext) RmXattr(oid string, name string) error {
	c_oid := C.CString(oid)
	c_name := C.CString(name)
	defer C.free(unsafe.Pointer(c_oid))
	defer C.free(unsafe.Pointer(c_name))

	ret := C.rados_rmxattr(
		ioctx.ioctx,
		c_oid,
		c_name)

	return getRadosError(int(ret))
}

// LockExclusive takes an exclusive lock on an object.
func (ioctx *IOContext) LockExclusive(oid, name, cookie, desc string, duration time.Duration, flags *byte) (int, error) {
	c_oid := C.CString(oid)
	c_name := C.CString(name)
	c_cookie := C.CString(cookie)
	c_desc := C.CString(desc)

	var c_duration C.struct_timeval
	if duration != 0 {
		tv := syscall.NsecToTimeval(duration.Nanoseconds())
		c_duration = C.struct_timeval{tv_sec: C.ceph_time_t(tv.Sec), tv_usec: C.ceph_suseconds_t(tv.Usec)}
	}

	var c_flags C.uint8_t
	if flags != nil {
		c_flags = C.uint8_t(*flags)
	}

	defer C.free(unsafe.Pointer(c_oid))
	defer C.free(unsafe.Pointer(c_name))
	defer C.free(unsafe.Pointer(c_cookie))
	defer C.free(unsafe.Pointer(c_desc))

	ret := C.rados_lock_exclusive(
		ioctx.ioctx,
		c_oid,
		c_name,
		c_cookie,
		c_desc,
		&c_duration,
		c_flags)

	// 0 on success, negative error code on failure
	// -EBUSY if the lock is already held by another (client, cookie) pair
	// -EEXIST if the lock is already held by the same (client, cookie) pair

	switch ret {
	case 0:
		return int(ret), nil
	case -C.EBUSY:
		return int(ret), nil
	case -C.EEXIST:
		return int(ret), nil
	default:
		return int(ret), RadosError(int(ret))
	}
}

// LockShared takes a shared lock on an object.
func (ioctx *IOContext) LockShared(oid, name, cookie, tag, desc string, duration time.Duration, flags *byte) (int, error) {
	c_oid := C.CString(oid)
	c_name := C.CString(name)
	c_cookie := C.CString(cookie)
	c_tag := C.CString(tag)
	c_desc := C.CString(desc)

	var c_duration C.struct_timeval
	if duration != 0 {
		tv := syscall.NsecToTimeval(duration.Nanoseconds())
		c_duration = C.struct_timeval{tv_sec: C.ceph_time_t(tv.Sec), tv_usec: C.ceph_suseconds_t(tv.Usec)}
	}

	var c_flags C.uint8_t
	if flags != nil {
		c_flags = C.uint8_t(*flags)
	}

	defer C.free(unsafe.Pointer(c_oid))
	defer C.free(unsafe.Pointer(c_name))
	defer C.free(unsafe.Pointer(c_cookie))
	defer C.free(unsafe.Pointer(c_tag))
	defer C.free(unsafe.Pointer(c_desc))

	ret := C.rados_lock_shared(
		ioctx.ioctx,
		c_oid,
		c_name,
		c_cookie,
		c_tag,
		c_desc,
		&c_duration,
		c_flags)

	// 0 on success, negative error code on failure
	// -EBUSY if the lock is already held by another (client, cookie) pair
	// -EEXIST if the lock is already held by the same (client, cookie) pair

	switch ret {
	case 0:
		return int(ret), nil
	case -C.EBUSY:
		return int(ret), nil
	case -C.EEXIST:
		return int(ret), nil
	default:
		return int(ret), RadosError(int(ret))
	}
}

// Unlock releases a shared or exclusive lock on an object.
func (ioctx *IOContext) Unlock(oid, name, cookie string) (int, error) {
	c_oid := C.CString(oid)
	c_name := C.CString(name)
	c_cookie := C.CString(cookie)

	defer C.free(unsafe.Pointer(c_oid))
	defer C.free(unsafe.Pointer(c_name))
	defer C.free(unsafe.Pointer(c_cookie))

	// 0 on success, negative error code on failure
	// -ENOENT if the lock is not held by the specified (client, cookie) pair

	ret := C.rados_unlock(
		ioctx.ioctx,
		c_oid,
		c_name,
		c_cookie)

	switch ret {
	case 0:
		return int(ret), nil
	case -C.ENOENT:
		return int(ret), nil
	default:
		return int(ret), RadosError(int(ret))
	}
}

// ListLockers lists clients that have locked the named object lock and
// information about the lock.
// The number of bytes required in each buffer is put in the corresponding size
// out parameter.  If any of the provided buffers are too short, -ERANGE is
// returned after these sizes are filled in.
func (ioctx *IOContext) ListLockers(oid, name string) (*LockInfo, error) {
	c_oid := C.CString(oid)
	c_name := C.CString(name)

	c_tag := (*C.char)(C.malloc(C.size_t(1024)))
	c_clients := (*C.char)(C.malloc(C.size_t(1024)))
	c_cookies := (*C.char)(C.malloc(C.size_t(1024)))
	c_addrs := (*C.char)(C.malloc(C.size_t(1024)))

	var c_exclusive C.int
	c_tag_len := C.size_t(1024)
	c_clients_len := C.size_t(1024)
	c_cookies_len := C.size_t(1024)
	c_addrs_len := C.size_t(1024)

	defer C.free(unsafe.Pointer(c_oid))
	defer C.free(unsafe.Pointer(c_name))
	defer C.free(unsafe.Pointer(c_tag))
	defer C.free(unsafe.Pointer(c_clients))
	defer C.free(unsafe.Pointer(c_cookies))
	defer C.free(unsafe.Pointer(c_addrs))

	ret := C.rados_list_lockers(
		ioctx.ioctx,
		c_oid,
		c_name,
		&c_exclusive,
		c_tag,
		&c_tag_len,
		c_clients,
		&c_clients_len,
		c_cookies,
		&c_cookies_len,
		c_addrs,
		&c_addrs_len)

	splitCString := func(items *C.char, itemsLen C.size_t) []string {
		currLen := 0
		clients := []string{}
		for currLen < int(itemsLen) {
			client := C.GoString(C.nextChunk(&items))
			clients = append(clients, client)
			currLen += len(client) + 1
		}
		return clients
	}

	if ret < 0 {
		return nil, RadosError(int(ret))
	}
	return &LockInfo{int(ret), c_exclusive == 1, C.GoString(c_tag), splitCString(c_clients, c_clients_len), splitCString(c_cookies, c_cookies_len), splitCString(c_addrs, c_addrs_len)}, nil
}

// BreakLock releases a shared or exclusive lock on an object, which was taken by the specified client.
func (ioctx *IOContext) BreakLock(oid, name, client, cookie string) (int, error) {
	c_oid := C.CString(oid)
	c_name := C.CString(name)
	c_client := C.CString(client)
	c_cookie := C.CString(cookie)

	defer C.free(unsafe.Pointer(c_oid))
	defer C.free(unsafe.Pointer(c_name))
	defer C.free(unsafe.Pointer(c_client))
	defer C.free(unsafe.Pointer(c_cookie))

	// 0 on success, negative error code on failure
	// -ENOENT if the lock is not held by the specified (client, cookie) pair
	// -EINVAL if the client cannot be parsed

	ret := C.rados_break_lock(
		ioctx.ioctx,
		c_oid,
		c_name,
		c_client,
		c_cookie)

	switch ret {
	case 0:
		return int(ret), nil
	case -C.ENOENT:
		return int(ret), nil
	case -C.EINVAL: // -EINVAL
		return int(ret), nil
	default:
		return int(ret), RadosError(int(ret))
	}
}
