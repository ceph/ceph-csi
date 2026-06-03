//
// Ceph Nautilus is the first release that includes rbd_pool_metadata_get(),
// rbd_pool_metadata_set() and rbd_pool_metadata_remove().

package rbd

// #cgo LDFLAGS: -lrbd
// #include <rados/librados.h>
// #include <rbd/librbd.h>
// #include <stdlib.h>
import "C"

import (
	"unsafe"

	"github.com/ceph/go-ceph/internal/retry"
	"github.com/ceph/go-ceph/rados"
)

// GetPoolMetadata returns pool metadata associated with the given key.
//
// Implements:
//
//	int rbd_pool_metadata_get(rados_ioctx_t io_ctx, const char *key, char *value, size_t *val_len);
func GetPoolMetadata(ioctx *rados.IOContext, key string) (string, error) {
	if ioctx == nil {
		return "", ErrNoIOContext
	}

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	var (
		buf []byte
		err error
	)
	retry.WithSizes(4096, 262144, func(size int) retry.Hint {
		cSize := C.size_t(size)
		buf = make([]byte, cSize)
		ret := C.rbd_pool_metadata_get(cephIoctx(ioctx),
			cKey,
			(*C.char)(unsafe.Pointer(&buf[0])),
			&cSize)
		err = getError(ret)
		return retry.Size(int(cSize)).If(err == errRange)
	})

	if err != nil {
		return "", err
	}
	return C.GoString((*C.char)(unsafe.Pointer(&buf[0]))), nil
}

// SetPoolMetadata updates the pool metadata string associated with the given key.
//
// Implements:
//
//	int rbd_pool_metadata_set(rados_ioctx_t io_ctx, const char *key, const char *value);
func SetPoolMetadata(ioctx *rados.IOContext, key, value string) error {
	if ioctx == nil {
		return ErrNoIOContext
	}

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cValue))

	ret := C.rbd_pool_metadata_set(cephIoctx(ioctx), cKey, cValue)
	return getError(ret)
}

// RemovePoolMetadata removes the pool metadata value for a given pool metadata key.
//
// Implements:
//
//	int rbd_pool_metadata_remove(rados_ioctx_t io_ctx, const char *key)
func RemovePoolMetadata(ioctx *rados.IOContext, key string) error {
	if ioctx == nil {
		return ErrNoIOContext
	}

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	ret := C.rbd_pool_metadata_remove(cephIoctx(ioctx), cKey)
	return getError(ret)
}

// PoolInit initializes a pool for use by rbd.
// This function does not create new pools, rather it prepares the pool
// to host rbd images.
//
// Implements:
//
//	int rbd_pool_init(rados_ioctx_t io, bool force)
func PoolInit(ioctx *rados.IOContext, force bool) error {
	if ioctx == nil {
		return ErrNoIOContext
	}

	ret := C.rbd_pool_init(cephIoctx(ioctx), C.bool(force))
	return getError(ret)
}

// poolStats represents RBD pool stats variable.
type poolStats struct {
	stats C.rbd_pool_stats_t
}

// poolStatsCreate creates a new poolStats struct.
//
// Implements:
//
//	void rbd_pool_stats_create(rbd_pool_stats_t *stats)
func poolStatsCreate() *poolStats {
	poolstats := &poolStats{}
	C.rbd_pool_stats_create(&poolstats.stats)
	return poolstats
}

// destroy a poolStats struct and free the associated resources.
//
// Implements:
//
//	void rbd_pool_stats_destroy(rbd_pool_stats_t stats)
func (poolstats *poolStats) destroy() {
	C.rbd_pool_stats_destroy(poolstats.stats)

	if poolstats.stats != nil {
		poolstats.stats = nil
	}
}

// PoolStatOption represents a group of configurable pool stat options.
type PoolStatOption C.rbd_pool_stat_option_t

const (
	// PoolStatOptionImages is the representation of
	// RBD_POOL_STAT_OPTION_IMAGES from librbd.
	PoolStatOptionImages = PoolStatOption(C.RBD_POOL_STAT_OPTION_IMAGES)
	// PoolStatOptionImageProvisionedBytes is the representation of
	// RBD_POOL_STAT_OPTION_IMAGE_PROVISIONED_BYTES from librbd.
	PoolStatOptionImageProvisionedBytes = PoolStatOption(C.RBD_POOL_STAT_OPTION_IMAGE_PROVISIONED_BYTES)
	// PoolStatOptionImageMaxProvisionedBytes is the representation of
	// RBD_POOL_STAT_OPTION_IMAGE_MAX_PROVISIONED_BYTES from librbd.
	PoolStatOptionImageMaxProvisionedBytes = PoolStatOption(C.RBD_POOL_STAT_OPTION_IMAGE_MAX_PROVISIONED_BYTES)
	// PoolStatOptionImageSnapshots is the representation of
	// RBD_POOL_STAT_OPTION_IMAGE_SNAPSHOTS from librbd.
	PoolStatOptionImageSnapshots = PoolStatOption(C.RBD_POOL_STAT_OPTION_IMAGE_SNAPSHOTS)
	// PoolStatOptionTrashImages is the representation of
	// RBD_POOL_STAT_OPTION_TRASH_IMAGES from librbd.
	PoolStatOptionTrashImages = PoolStatOption(C.RBD_POOL_STAT_OPTION_TRASH_IMAGES)
	// PoolStatOptionTrashProvisionedBytes is the representation of
	// RBD_POOL_STAT_OPTION_TRASH_PROVISIONED_BYTES from librbd.
	PoolStatOptionTrashProvisionedBytes = PoolStatOption(C.RBD_POOL_STAT_OPTION_TRASH_PROVISIONED_BYTES)
	// PoolStatOptionTrashMaxProvisionedBytes is the representation of
	// RBD_POOL_STAT_OPTION_TRASH_MAX_PROVISIONED_BYTES from librbd.
	PoolStatOptionTrashMaxProvisionedBytes = PoolStatOption(C.RBD_POOL_STAT_OPTION_TRASH_MAX_PROVISIONED_BYTES)
	// PoolStatOptionTrashSnapshots is the representation of
	// RBD_POOL_STAT_OPTION_TRASH_SNAPSHOTS from librbd.
	PoolStatOptionTrashSnapshots = PoolStatOption(C.RBD_POOL_STAT_OPTION_TRASH_SNAPSHOTS)
)

// addPoolStatOption adds the given PoolStatOption to PoolStats.
//
// Implements:
//
//	int rbd_pool_stats_option_add_uint64(rbd_pool_stats_t stats, int stat_option, uint64_t* stat_val)
func (poolstats *poolStats) addPoolStatOption(option PoolStatOption, val *uint64) error {
	ret := C.rbd_pool_stats_option_add_uint64(
		poolstats.stats,
		C.int(option),
		(*C.uint64_t)(val))
	return getError(ret)
}

// GetAllPoolStats returns a map of all PoolStatOption(s) to their respective values.
//
// Implements:
//
//	int rbd_pool_stats_get(rados_ioctx_t io, rbd_pool_stats_t stats);
func GetAllPoolStats(ioctx *rados.IOContext) (map[PoolStatOption]uint64, error) {
	var omap = make(map[PoolStatOption]uint64)
	if ioctx == nil {
		return omap, ErrNoIOContext
	}

	poolstats := poolStatsCreate()
	defer func() {
		poolstats.destroy()
	}()

	var keys [8]PoolStatOption

	keys[0] = PoolStatOptionImages
	keys[1] = PoolStatOptionImageProvisionedBytes
	keys[2] = PoolStatOptionImageMaxProvisionedBytes
	keys[3] = PoolStatOptionImageSnapshots
	keys[4] = PoolStatOptionTrashImages
	keys[5] = PoolStatOptionTrashProvisionedBytes
	keys[6] = PoolStatOptionTrashMaxProvisionedBytes
	keys[7] = PoolStatOptionTrashSnapshots

	ovalArray := make([]uint64, len(keys))

	// add option with the address where the respective value would be stored.
	for i, key := range keys {
		err := poolstats.addPoolStatOption(key, &ovalArray[i])
		if err != nil {
			return omap, err
		}
	}

	ret := C.rbd_pool_stats_get(cephIoctx(ioctx), poolstats.stats)
	if ret < 0 {
		return omap, getError(ret)
	}

	for j, key := range keys {
		omap[key] = ovalArray[j]
	}
	return omap, nil
}
