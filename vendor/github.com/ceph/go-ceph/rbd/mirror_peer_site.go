//go:build !nautilus && ceph_preview
// +build !nautilus,ceph_preview

package rbd

// #cgo LDFLAGS: -lrbd
// #include <stdlib.h>
// #include <rbd/librbd.h>
import "C"

import (
	"bytes"
	"unsafe"

	"github.com/ceph/go-ceph/internal/cutil"
	"github.com/ceph/go-ceph/internal/retry"
	"github.com/ceph/go-ceph/rados"
)

// AddMirrorPeerSite adds a peer site to the list of existing sites
//
// Implements:
//
//	int rbd_mirror_peer_site_add(rados_ioctx_t p, char *uuid, size_t uuid_max_length,
//								 rbd_mirror_peer_direction_t direction,
//								 const char *site_name,
//								 const char *client_name);
func AddMirrorPeerSite(ioctx *rados.IOContext, siteName string, clientName string,
	direction MirrorPeerDirection) (string, error) {

	var (
		err   error
		buf   []byte
		cSize C.size_t
	)

	cSiteName := C.CString(siteName)
	defer C.free(unsafe.Pointer(cSiteName))
	cClientName := C.CString(clientName)
	defer C.free(unsafe.Pointer(cClientName))

	retry.WithSizes(512, 1<<16, func(size int) retry.Hint {
		cSize = C.size_t(size)
		buf = make([]byte, cSize)
		ret := C.rbd_mirror_peer_site_add(
			cephIoctx(ioctx),
			(*C.char)(unsafe.Pointer(&buf[0])),
			cSize, C.rbd_mirror_peer_direction_t(direction),
			cSiteName, cClientName)
		err = getError(ret)
		return retry.Size(int(cSize)).If(err != nil)
	})
	if err != nil {
		return "", err
	}
	return string(bytes.Trim(buf[:cSize], "\x00")), nil
}

// RemoveMirrorPeerSite removes the site with the provided uuid
//
// Implements:
//
//	int rbd_mirror_peer_site_remove(rados_ioctx_t p, const char *uuid)
func RemoveMirrorPeerSite(ioctx *rados.IOContext, uuid string) error {
	cUUID := C.CString(uuid)
	defer C.free(unsafe.Pointer(cUUID))

	ret := C.rbd_mirror_peer_site_remove(cephIoctx(ioctx), cUUID)

	return getError(ret)
}

// GetAttributesMirrorPeerSite fetches the list of key,value pair of attributes of a peer site
//
// Implements:
//
//	int rbd_mirror_peer_site_get_attributes(rados_ioctx_t p, const char *uuid, char *keys,
//											size_t *max_key_len, char *values, size_t *max_val_len,
//											size_t *key_value_count);
func GetAttributesMirrorPeerSite(ioctx *rados.IOContext, uuid string) (map[string]string, error) {

	var (
		err     error
		keys    []byte
		vals    []byte
		keySize C.size_t
		valSize C.size_t
		count   = C.size_t(0)
	)

	cUUID := C.CString(uuid)
	defer C.free(unsafe.Pointer(cUUID))

	retry.WithSizes(1024, 1<<16, func(size int) retry.Hint {
		keySize = C.size_t(size)
		valSize = C.size_t(size)
		keys = make([]byte, keySize)
		vals = make([]byte, valSize)
		ret := C.rbd_mirror_peer_site_get_attributes(
			cephIoctx(ioctx), cUUID, (*C.char)(unsafe.Pointer(&keys[0])),
			&keySize, (*C.char)(unsafe.Pointer(&vals[0])), &valSize,
			&count)
		err = getErrorIfNegative(ret)
		return retry.Size(int(keySize)).If(err == errRange)
	})
	if err != nil {
		return nil, err
	}

	keyList := cutil.SplitBuffer(keys[:keySize])
	valList := cutil.SplitBuffer(vals[:valSize])
	attributes := map[string]string{}
	for i := 0; i < int(len(keyList)); i++ {
		attributes[keyList[i]] = valList[i]
	}
	return attributes, nil
}

// SetAttributesMirrorPeerSite sets the attributes for the site with the given uuid
//
// Implements:
//
//	int rbd_mirror_peer_site_set_attributes(rados_ioctx_t p, const char *uuid,
//											const char *keys, const char *values,
//											size_t count) ;
func SetAttributesMirrorPeerSite(ioctx *rados.IOContext, uuid string, attributes map[string]string) error {
	cUUID := C.CString(uuid)
	defer C.free(unsafe.Pointer(cUUID))

	var (
		key   string
		val   string
		count = C.size_t(len(attributes))
	)

	for k, v := range attributes {
		key += k + "\000"
		val += v + "\000"
	}

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	cVal := C.CString(val)
	defer C.free(unsafe.Pointer(cVal))

	ret := C.rbd_mirror_peer_site_set_attributes(cephIoctx(ioctx), cUUID, cKey, cVal, count)

	return getError(ret)
}

// MirrorPeerSite contains information about a mirroring peer site.
type MirrorPeerSite struct {
	UUID       string
	Direction  MirrorPeerDirection
	SiteName   string
	MirrorUUID string
	ClientName string
	LastSeen   C.time_t
}

// ListMirrorPeerSite returns the list of peer sites
//
// Implements:
//
//	int rbd_mirror_peer_site_list(rados_ioctx_t p, rbd_mirror_peer_site_t *peers, int *max_peers)
func ListMirrorPeerSite(ioctx *rados.IOContext) ([]*MirrorPeerSite, error) {
	var mps []*MirrorPeerSite
	cMaxPeers := C.int(10)

	var cSites []C.rbd_mirror_peer_site_t
	for {
		cSites = make([]C.rbd_mirror_peer_site_t, cMaxPeers)
		ret := C.rbd_mirror_peer_site_list(cephIoctx(ioctx), &cSites[0], &cMaxPeers)
		err := getError(ret)
		if err == errRange {
			// There are too many peer sites to fit in the list, and the number of peer sites has been
			// returned in cMaxPeers. Try again with the returned value.
			continue
		}
		if err != nil {
			return nil, err
		}

		// ret == 0
		break
	}

	defer C.rbd_mirror_peer_site_list_cleanup(&cSites[0], cMaxPeers)
	cSites = cSites[:cMaxPeers]

	for _, cSite := range cSites {
		mps = append(mps, &MirrorPeerSite{
			UUID:       C.GoString(cSite.uuid),
			Direction:  MirrorPeerDirection(cSite.direction),
			SiteName:   C.GoString(cSite.site_name),
			MirrorUUID: C.GoString(cSite.mirror_uuid),
			ClientName: C.GoString(cSite.client_name),
		})
	}

	return mps, nil
}

// SetMirrorPeerSiteClientName sets the client name for a mirror peer site
//
// Implements:
//
//	int rbd_mirror_peer_site_set_client_name(rados_ioctx_t p, const char *uuid,
//											 const char *client_name);
func SetMirrorPeerSiteClientName(ioctx *rados.IOContext, uuid string, clientName string) error {
	cUUID := C.CString(uuid)
	defer C.free(unsafe.Pointer(cUUID))

	cClientName := C.CString(clientName)
	defer C.free(unsafe.Pointer(cClientName))

	ret := C.rbd_mirror_peer_site_set_client_name(cephIoctx(ioctx), cUUID, cClientName)

	return getError(ret)
}

// SetMirrorPeerSiteDirection sets the direction of a mirror peer site
//
// Implements:
//
//	int rbd_mirror_peer_site_set_direction(rados_ioctx_t p, const char *uuid,
//										   rbd_mirror_peer_direction_t direction);
func SetMirrorPeerSiteDirection(ioctx *rados.IOContext, uuid string, direction MirrorPeerDirection) error {
	cUUID := C.CString(uuid)
	defer C.free(unsafe.Pointer(cUUID))

	ret := C.rbd_mirror_peer_site_set_direction(cephIoctx(ioctx), cUUID,
		C.rbd_mirror_peer_direction_t(direction))

	return getError(ret)
}

// SetMirrorPeerSiteName sets the name of a mirror peer site
//
// Implements:
//
//	int rbd_mirror_peer_site_set_name(rados_ioctx_t p, const char *uuid,
//									  const char *site_name);
func SetMirrorPeerSiteName(ioctx *rados.IOContext, uuid string, siteName string) error {
	cUUID := C.CString(uuid)
	defer C.free(unsafe.Pointer(cUUID))

	cSiteName := C.CString(siteName)
	defer C.free(unsafe.Pointer(cSiteName))

	ret := C.rbd_mirror_peer_site_set_name(cephIoctx(ioctx), cUUID, cSiteName)

	return getError(ret)
}
