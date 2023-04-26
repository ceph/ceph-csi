//go:build nautilus
// +build nautilus

package rbd

// #cgo LDFLAGS: -lrbd
// #include <errno.h>
// #include <stdlib.h>
// #include <rados/librados.h>
// #include <rbd/librbd.h>
import "C"
import (
	"unsafe"

	"github.com/ceph/go-ceph/rados"
)

// MirrorMode indicates the current mode of mirroring that is applied onto a
// pool. A pool that doesn't have an explicit mirroring mode applied to it is
// said to be disabled - that's the default.
type MirrorMode int

const (
	// MirrorModeDisabled disables mirroring.
	MirrorModeDisabled = MirrorMode(C.RBD_MIRROR_MODE_DISABLED)
	// MirrorModeImage enables mirroring on a per-image basis.
	MirrorModeImage = MirrorMode(C.RBD_MIRROR_MODE_IMAGE)
	// MirrorModePool enables mirroring on all journaled images.
	MirrorModePool = MirrorMode(C.RBD_MIRROR_MODE_POOL)
)

// MirrorModeGet returns the mode of mirroring currently applied to a pool.
//
// Note: this can only be used if go-ceph is compiled with the `nautilus` build
// tag.
//
// Implements:
//
//	int rbd_mirror_mode_get(rados_ioctx_t p, rbd_mirror_mode_t *mirror_mode)
func MirrorModeGet(ioctx *rados.IOContext) (MirrorMode, error) {
	var rmm C.rbd_mirror_mode_t

	ret := C.rbd_mirror_mode_get(cephIoctx(ioctx), &rmm)
	if ret != 0 {
		return -1, getError(ret)
	}

	return MirrorMode(rmm), nil
}

// MirrorModeSet sets the mirror mode for a pool.
//
// Note: this can only be used if go-ceph is compiled with the `nautilus` build
// tag.
//
// Implements:
//
//	rbd_mirror_mode_set(rados_ioctx_t p, rbd_mirror_mode_t mirror_mode)
func MirrorModeSet(ioctx *rados.IOContext, mode MirrorMode) error {
	cMode := C.rbd_mirror_mode_t(mode)

	ret := C.rbd_mirror_mode_set(cephIoctx(ioctx), cMode)

	return getError(ret)
}

// MirrorPeerAdd configures a peering relationship with another cluster. Note
// that it does not transfer over that cluster's config or keyrings, which must
// already be available to the rbd-mirror daemon(s).
//
// Note: this can only be used if go-ceph is compiled with the `nautilus` build
// tag.
//
// Implements:
//
//	 int rbd_mirror_peer_add(rados_ioctx_t p, char *uuid,
//	 												size_t uuid_max_length,
//														const char *cluster_name,
//														const char *client_name)
func MirrorPeerAdd(ioctx *rados.IOContext, clusterName, clientName string) (string, error) {
	// librbd uses 36-byte UUIDs with a trailing null. rbd_mirror_add_peer will
	// return -E2BIG if we pass a UUID buffer smaller than 37 bytes.
	const cUUIDMaxLen = C.size_t(37)
	cUUID := make([]C.char, cUUIDMaxLen)

	cClusterName := C.CString(clusterName)
	defer C.free(unsafe.Pointer(cClusterName))

	cClientName := C.CString(clientName)
	defer C.free(unsafe.Pointer(cClientName))

	ret := C.rbd_mirror_peer_add(cephIoctx(ioctx), &cUUID[0], cUUIDMaxLen,
		cClusterName, cClientName)

	return C.GoString(&cUUID[0]), getError(ret)
}

// MirrorPeerRemove tears down a peering relationship.
//
// Note: this can only be used if go-ceph is compiled with the `nautilus` build
// tag.
//
// Implements:
//
//	int rbd_mirror_peer_remove(rados_ioctx_t io_ctx, const char *uuid)
func MirrorPeerRemove(ioctx *rados.IOContext, uuid string) error {
	cUUID := C.CString(uuid)
	defer C.free(unsafe.Pointer(cUUID))

	ret := C.rbd_mirror_peer_remove(cephIoctx(ioctx), cUUID)

	return getError(ret)
}

// MirrorPeerInfo contains information about a configured mirroring peer.
type MirrorPeerInfo struct {
	UUID        string
	ClusterName string
	ClientName  string
}

// MirrorPeerList returns a list of configured mirroring peers.
//
// Note: this can only be used if go-ceph is compiled with the `nautilus` build
// tag.
//
// Implements:
//
//	 int rbd_mirror_peer_list(rados_ioctx_t io_ctx,
//	 												 rbd_mirror_peer_list_t *peers,
//											 			 int *max_peers);
func MirrorPeerList(ioctx *rados.IOContext) ([]*MirrorPeerInfo, error) {
	var mpi []*MirrorPeerInfo
	cMaxPeers := C.int(5)

	var cPeers []C.rbd_mirror_peer_t
	for {
		cPeers = make([]C.rbd_mirror_peer_t, cMaxPeers)
		ret := C.rbd_mirror_peer_list(cephIoctx(ioctx), &cPeers[0], &cMaxPeers)
		if ret == -C.ERANGE {
			// There are too many peers to fit in the list, and the number of peers has been
			// returned in cMaxPeers. Try again with the returned value.
			continue
		}
		if ret != 0 {
			return nil, getError(ret)
		}

		// ret == 0
		break
	}
	defer C.rbd_mirror_peer_list_cleanup(&cPeers[0], cMaxPeers)
	cPeers = cPeers[:cMaxPeers]

	for _, cPeer := range cPeers {
		mpi = append(mpi, &MirrorPeerInfo{
			UUID:        C.GoString(cPeer.uuid),
			ClusterName: C.GoString(cPeer.cluster_name),
			ClientName:  C.GoString(cPeer.client_name),
		})
	}

	return mpi, nil
}

// MirrorImageState indicates whether mirroring is enabled or disabled on an
// image.
//
// A mirrored image might not immediately change its status to disabled if it has
// offsets left to sync with its peers - this is denoted by 'disabling' state.
//
// It is important to note that mirroring cannot be enabled on an image without
// first flipping on the 'journaling' image feature for it.
type MirrorImageState int

const (
	// MirrorImageDisabling is the representation of
	// RBD_MIRROR_IMAGE_DISABLING from librbd.
	MirrorImageDisabling = MirrorImageState(C.RBD_MIRROR_IMAGE_DISABLING)
	// MirrorImageEnabled is the representation of
	// RBD_MIRROR_IMAGE_ENABLED from librbd.
	MirrorImageEnabled = MirrorImageState(C.RBD_MIRROR_IMAGE_ENABLED)
	// MirrorImageDisabled is the representation of
	// RBD_MIRROR_IMAGE_DISABLED from librbd.
	MirrorImageDisabled = MirrorImageState(C.RBD_MIRROR_IMAGE_DISABLED)
)

// MirrorImageStatusState denotes the current replication status of a given
// image.
type MirrorImageStatusState int

const (
	// MirrorImageStatusStateUnknown is equivalent to MIRROR_IMAGE_STATUS_STATE_UNKNOWN.
	MirrorImageStatusStateUnknown = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_UNKNOWN)
	// MirrorImageStatusStateError is equivalent to MIRROR_IMAGE_STATUS_STATE_ERROR.
	MirrorImageStatusStateError = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_ERROR)
	// MirrorImageStatusStateSyncing is equivalent to MIRROR_IMAGE_STATUS_STATE_SYNCING.
	MirrorImageStatusStateSyncing = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_SYNCING)
	// MirrorImageStatusStateStartingReplay is equivalent to MIRROR_IMAGE_STATUS_STATE_STARTING_REPLAY.
	MirrorImageStatusStateStartingReplay = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_STARTING_REPLAY)
	// MirrorImageStatusStateReplaying is equivalent to MIRROR_IMAGE_STATUS_STATE_REPLAYING.
	MirrorImageStatusStateReplaying = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_REPLAYING)
	// MirrorImageStatusStateStoppingReplay is equivalent to MIRROR_IMAGE_STATUS_STATE_STOPPING_REPLAY.
	MirrorImageStatusStateStoppingReplay = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_STOPPING_REPLAY)
	// MirrorImageStatusStateStopped is equivalent to MIRROR_IMAGE_STATUS_STATE_STOPPED.
	MirrorImageStatusStateStopped = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_STOPPED)
)

// MirrorImageInfo provides information about the mirroring progress of an image.
type MirrorImageInfo struct {
	Name        string
	Description string
	State       MirrorImageState
	StatusState MirrorImageStatusState
	GlobalID    string
	IsPrimary   bool
	IsUp        bool
}

// MirrorGetImage returns the MirrorImageInfo for an image.
//
// Note: this can only be used if go-ceph is compiled with the `nautilus` build
// tag.
//
// Implements:
//
//	rbd_mirror_image_get_info(rbd_image_t image,
//	                          rbd_mirror_image_info_t *mirror_image_info,
//	                          size_t info_size)
func (image *Image) MirrorGetImage() (*MirrorImageInfo, error) {
	err := image.validate(imageIsOpen)
	if err != nil {
		return nil, err
	}

	var status C.rbd_mirror_image_status_t
	ret := C.rbd_mirror_image_get_status(image.image, &status, C.sizeof_rbd_mirror_image_status_t)
	if ret != 0 {
		return nil, getError(ret)
	}

	return &MirrorImageInfo{
		Name:        C.GoString(status.name),
		Description: C.GoString(status.description),
		State:       MirrorImageState(status.info.state),
		StatusState: MirrorImageStatusState(status.state),
		GlobalID:    C.GoString(status.info.global_id),
		IsPrimary:   bool(status.info.primary),
		IsUp:        bool(status.up),
	}, nil
}

// MirrorImageList returns a MirrorImageInfo for each mirrored image.
//
// Note: this can only be used if go-ceph is compiled with the `nautilus` build
// tag.
//
// Implements:
//
//	int rbd_mirror_image_status_list(rados_ioctx_t io_ctx,
//					      									 const char *start_id, size_t max,
//					      									 char **image_ids,
//					      									 rbd_mirror_image_status_t *images,
//					      									 size_t *len)
func MirrorImageList(ioctx *rados.IOContext) ([]*MirrorImageInfo, error) {
	imageInfos := make([]*MirrorImageInfo, 0)
	const cMaxIter C.size_t = 100
	var startID string

	for {
		// We need to wrap all the actions within the for loop in a function
		// in order to ensure that we correctly reclaim all allocated memory
		// from C at the end of every iteration.
		ret, done := iterateImageList(ioctx, &imageInfos, &startID, cMaxIter)
		if ret != 0 {
			return imageInfos, getError(ret)
		}

		if done {
			break
		}
	}
	return imageInfos, nil
}

func iterateImageList(ioctx *rados.IOContext, imageInfos *[]*MirrorImageInfo, startID *string, cMaxIter C.size_t) (C.int, bool) {
	cImageIDs := make([]*C.char, cMaxIter)
	cImageStatus := make([]C.rbd_mirror_image_status_t, cMaxIter)
	done := false

	var cLen C.size_t
	ret := C.rbd_mirror_image_status_list(cephIoctx(ioctx), C.CString(*startID),
		cMaxIter, &cImageIDs[0], &cImageStatus[0], &cLen)
	if ret != 0 {
		return ret, done
	}

	// If the list length is 0 or less than the max size
	// specified we know we are on the last page of the list,
	// and we don't need to continue iterating.
	if cLen < cMaxIter {
		done = true
	}

	if cLen == 0 {
		return C.int(0), done
	}

	defer func() {
		C.rbd_mirror_image_status_list_cleanup(&cImageIDs[0], &cImageStatus[0], cLen)
	}()

	for i := 0; i < int(cLen); i++ {
		mi := &MirrorImageInfo{
			Name:        C.GoString(cImageStatus[i].name),
			Description: C.GoString(cImageStatus[i].description),
			State:       MirrorImageState(cImageStatus[i].info.state),
			StatusState: MirrorImageStatusState(cImageStatus[i].state),
			GlobalID:    C.GoString(cImageStatus[i].info.global_id),
			IsPrimary:   bool(cImageStatus[i].info.primary),
			IsUp:        bool(cImageStatus[i].up),
		}

		*imageInfos = append(*imageInfos, mi)
	}

	*startID = C.GoString(cImageIDs[cLen-1])
	return C.int(0), done
}

// MirrorEnable will enable mirroring for an image.
//
// Note: this can only be used if go-ceph is compiled with the `nautilus` build
// tag.
//
// Implements:
//
//	int rbd_mirror_image_enable(rbd_image_t image)
func (image *Image) MirrorEnable() error {
	err := image.validate(imageIsOpen)
	if err != nil {
		return err
	}

	ret := C.rbd_mirror_image_enable(image.image)
	return getError(ret)
}

// MirrorDisable will disable mirroring for an image.
//
// Note: this can only be used if go-ceph is compiled with the `nautilus` build
// tag.
//
// Implements:
//
//	int rbd_mirror_image_disable(rbd_image_t image, bool force)
func (image *Image) MirrorDisable(force bool) error {
	err := image.validate(imageIsOpen)
	if err != nil {
		return err
	}

	ret := C.rbd_mirror_image_disable(image.image, C.bool(force))
	return getError(ret)
}

// MirrorPromote will promote an image to primary status.
//
// Note: this can only be used if go-ceph is compiled with the `nautilus` build
// tag.
//
// Implements:
//
//	int rbd_mirror_image_promote(rbd_image_t image, bool force)
func (image *Image) MirrorPromote(force bool) error {
	err := image.validate(imageIsOpen)
	if err != nil {
		return err
	}

	ret := C.rbd_mirror_image_promote(image.image, C.bool(force))
	return getError(ret)
}

// MirrorDemote will demote an image to secondary status.
//
// Note: this can only be used if go-ceph is compiled with the `nautilus` build
// tag.
//
// Implements:
//
//	int rbd_mirror_image_demote(rbd_image_t image)
func (image *Image) MirrorDemote() error {
	err := image.validate(imageIsOpen)
	if err != nil {
		return err
	}

	ret := C.rbd_mirror_image_demote(image.image)
	return getError(ret)
}

// MirrorResync is used to manually resolve split-brain status by triggering
// resynchronization.
//
// Note: this can only be used if go-ceph is compiled with the `nautilus` build
// tag.
//
// Implements:
//
//	int rbd_mirror_image_resync(rbd_image_t image)
func (image *Image) MirrorResync() error {
	err := image.validate(imageIsOpen)
	if err != nil {
		return err
	}

	ret := C.rbd_mirror_image_resync(image.image)
	return getError(ret)
}
