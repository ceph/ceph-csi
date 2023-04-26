//go:build !nautilus
// +build !nautilus

package rbd

// #cgo LDFLAGS: -lrbd
// #include <stdlib.h>
// #include <rbd/librbd.h>
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/ceph/go-ceph/internal/cutil"
	"github.com/ceph/go-ceph/internal/retry"
	"github.com/ceph/go-ceph/rados"
)

// MirrorMode is used to indicate an approach used for RBD mirroring.
type MirrorMode int64

const (
	// MirrorModeDisabled disables mirroring.
	MirrorModeDisabled = MirrorMode(C.RBD_MIRROR_MODE_DISABLED)
	// MirrorModeImage enables mirroring on a per-image basis.
	MirrorModeImage = MirrorMode(C.RBD_MIRROR_MODE_IMAGE)
	// MirrorModePool enables mirroring on all journaled images.
	MirrorModePool = MirrorMode(C.RBD_MIRROR_MODE_POOL)
)

// String representation of MirrorMode.
func (m MirrorMode) String() string {
	switch m {
	case MirrorModeDisabled:
		return "disabled"
	case MirrorModeImage:
		return "image"
	case MirrorModePool:
		return "pool"
	default:
		return "<unknown>"
	}
}

// ImageMirrorMode is used to indicate the mirroring approach for an RBD image.
type ImageMirrorMode int64

// ImageMirrorModeFilter is a ImageMirrorMode or nil for no filtering
type ImageMirrorModeFilter interface {
	mode() ImageMirrorMode
}

// Mode returns the ImageMirrorMode
func (imm ImageMirrorMode) mode() ImageMirrorMode {
	return imm
}

const (
	// ImageMirrorModeJournal uses journaling to propagate RBD images between
	// ceph clusters.
	ImageMirrorModeJournal = ImageMirrorMode(C.RBD_MIRROR_IMAGE_MODE_JOURNAL)
	// ImageMirrorModeSnapshot uses snapshots to propagate RBD images between
	// ceph clusters.
	ImageMirrorModeSnapshot = ImageMirrorMode(C.RBD_MIRROR_IMAGE_MODE_SNAPSHOT)
)

// String representation of ImageMirrorMode.
func (imm ImageMirrorMode) String() string {
	switch imm {
	case ImageMirrorModeJournal:
		return "journal"
	case ImageMirrorModeSnapshot:
		return "snapshot"
	default:
		return "<unknown>"
	}
}

// GetMirrorUUID returns a string naming the mirroring uuid for the pool
// associated with the ioctx.
//
// Implements:
//
//	int rbd_mirror_uuid_get(rados_ioctx_t io_ctx, char *uuid, size_t
//	                        *max_len);
func GetMirrorUUID(ioctx *rados.IOContext) (string, error) {
	var (
		err   error
		buf   []byte
		cSize C.size_t
	)
	retry.WithSizes(1024, 1<<16, func(size int) retry.Hint {
		cSize = C.size_t(size)
		buf = make([]byte, cSize)
		ret := C.rbd_mirror_uuid_get(
			cephIoctx(ioctx),
			(*C.char)(unsafe.Pointer(&buf[0])),
			&cSize)
		err = getErrorIfNegative(ret)
		return retry.Size(int(cSize)).If(err == errRange)
	})
	if err != nil {
		return "", err
	}
	return string(buf[:cSize]), nil
}

// SetMirrorMode is used to enable or disable pool level mirroring with either
// an automatic or per-image behavior.
//
// Implements:
//
//	int rbd_mirror_mode_set(rados_ioctx_t io_ctx,
//	                        rbd_mirror_mode_t mirror_mode);
func SetMirrorMode(ioctx *rados.IOContext, mode MirrorMode) error {
	ret := C.rbd_mirror_mode_set(
		cephIoctx(ioctx),
		C.rbd_mirror_mode_t(mode))
	return getError(ret)
}

// GetMirrorMode is used to fetch the current mirroring mode for a pool.
//
// Implements:
//
//	int rbd_mirror_mode_get(rados_ioctx_t io_ctx,
//	                        rbd_mirror_mode_t *mirror_mode);
func GetMirrorMode(ioctx *rados.IOContext) (MirrorMode, error) {
	var mode C.rbd_mirror_mode_t

	ret := C.rbd_mirror_mode_get(
		cephIoctx(ioctx),
		&mode)
	if err := getError(ret); err != nil {
		return MirrorModeDisabled, err
	}
	return MirrorMode(mode), nil
}

// MirrorEnable will enable mirroring for an image using the specified mode.
//
// Implements:
//
//	int rbd_mirror_image_enable2(rbd_image_t image,
//	                             rbd_mirror_image_mode_t mode);
func (image *Image) MirrorEnable(mode ImageMirrorMode) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}
	ret := C.rbd_mirror_image_enable2(image.image, C.rbd_mirror_image_mode_t(mode))
	return getError(ret)
}

// MirrorDisable will disable mirroring for the image.
//
// Implements:
//
//	int rbd_mirror_image_disable(rbd_image_t image, bool force);
func (image *Image) MirrorDisable(force bool) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}
	ret := C.rbd_mirror_image_disable(image.image, C.bool(force))
	return getError(ret)
}

// MirrorPromote will promote the image to primary status.
//
// Implements:
//
//	int rbd_mirror_image_promote(rbd_image_t image, bool force);
func (image *Image) MirrorPromote(force bool) error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}
	ret := C.rbd_mirror_image_promote(image.image, C.bool(force))
	return getError(ret)
}

// MirrorDemote will demote the image to secondary status.
//
// Implements:
//
//	int rbd_mirror_image_demote(rbd_image_t image);
func (image *Image) MirrorDemote() error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}
	ret := C.rbd_mirror_image_demote(image.image)
	return getError(ret)
}

// MirrorResync is used to manually resolve split-brain status by triggering
// resynchronization.
//
// Implements:
//
//	int rbd_mirror_image_resync(rbd_image_t image);
func (image *Image) MirrorResync() error {
	if err := image.validate(imageIsOpen); err != nil {
		return err
	}
	ret := C.rbd_mirror_image_resync(image.image)
	return getError(ret)
}

// MirrorInstanceID returns a string naming the instance id for the image.
//
// Implements:
//
//	int rbd_mirror_image_get_instance_id(rbd_image_t image,
//	                                     char *instance_id,
//	                                     size_t *id_max_length);
func (image *Image) MirrorInstanceID() (string, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return "", err
	}
	var (
		err   error
		buf   []byte
		cSize C.size_t
	)
	retry.WithSizes(1024, 1<<16, func(size int) retry.Hint {
		cSize = C.size_t(size)
		buf = make([]byte, cSize)
		ret := C.rbd_mirror_image_get_instance_id(
			image.image,
			(*C.char)(unsafe.Pointer(&buf[0])),
			&cSize)
		err = getErrorIfNegative(ret)
		return retry.Size(int(cSize)).If(err == errRange)
	})
	if err != nil {
		return "", err
	}
	return string(buf[:cSize]), nil
}

// MirrorImageState represents the mirroring state of a RBD image.
type MirrorImageState C.rbd_mirror_image_state_t

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

// String representation of MirrorImageState.
func (mis MirrorImageState) String() string {
	switch mis {
	case MirrorImageDisabling:
		return "disabling"
	case MirrorImageEnabled:
		return "enabled"
	case MirrorImageDisabled:
		return "disabled"
	default:
		return "<unknown>"
	}
}

// MirrorImageInfo represents the mirroring status information of a RBD image.
type MirrorImageInfo struct {
	GlobalID string
	State    MirrorImageState
	Primary  bool
}

func convertMirrorImageInfo(cInfo *C.rbd_mirror_image_info_t) MirrorImageInfo {
	return MirrorImageInfo{
		GlobalID: C.GoString(cInfo.global_id),
		State:    MirrorImageState(cInfo.state),
		Primary:  bool(cInfo.primary),
	}
}

// GetMirrorImageInfo fetches the mirroring status information of a RBD image.
//
// Implements:
//
//	int rbd_mirror_image_get_info(rbd_image_t image,
//	                              rbd_mirror_image_info_t *mirror_image_info,
//	                              size_t info_size)
func (image *Image) GetMirrorImageInfo() (*MirrorImageInfo, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return nil, err
	}

	var cInfo C.rbd_mirror_image_info_t

	ret := C.rbd_mirror_image_get_info(
		image.image,
		&cInfo,
		C.sizeof_rbd_mirror_image_info_t)
	if ret < 0 {
		return nil, getError(ret)
	}

	mii := convertMirrorImageInfo(&cInfo)

	// free C memory allocated by C.rbd_mirror_image_get_info call
	C.rbd_mirror_image_get_info_cleanup(&cInfo)
	return &mii, nil
}

// GetImageMirrorMode fetches the mirroring approach for an RBD image.
//
// Implements:
//
//	int rbd_mirror_image_get_mode(rbd_image_t image, rbd_mirror_image_mode_t *mode);
func (image *Image) GetImageMirrorMode() (ImageMirrorMode, error) {
	var mode C.rbd_mirror_image_mode_t
	if err := image.validate(imageIsOpen); err != nil {
		return ImageMirrorMode(mode), err
	}

	ret := C.rbd_mirror_image_get_mode(image.image, &mode)
	return ImageMirrorMode(mode), getError(ret)
}

// MirrorImageStatusState is used to indicate the state of a mirrored image
// within the site status info.
type MirrorImageStatusState int64

const (
	// MirrorImageStatusStateUnknown is equivalent to MIRROR_IMAGE_STATUS_STATE_UNKNOWN
	MirrorImageStatusStateUnknown = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_UNKNOWN)
	// MirrorImageStatusStateError is equivalent to MIRROR_IMAGE_STATUS_STATE_ERROR
	MirrorImageStatusStateError = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_ERROR)
	// MirrorImageStatusStateSyncing is equivalent to MIRROR_IMAGE_STATUS_STATE_SYNCING
	MirrorImageStatusStateSyncing = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_SYNCING)
	// MirrorImageStatusStateStartingReplay is equivalent to MIRROR_IMAGE_STATUS_STATE_STARTING_REPLAY
	MirrorImageStatusStateStartingReplay = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_STARTING_REPLAY)
	// MirrorImageStatusStateReplaying is equivalent to MIRROR_IMAGE_STATUS_STATE_REPLAYING
	MirrorImageStatusStateReplaying = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_REPLAYING)
	// MirrorImageStatusStateStoppingReplay is equivalent to MIRROR_IMAGE_STATUS_STATE_STOPPING_REPLAY
	MirrorImageStatusStateStoppingReplay = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_STOPPING_REPLAY)
	// MirrorImageStatusStateStopped is equivalent to MIRROR_IMAGE_STATUS_STATE_STOPPED
	MirrorImageStatusStateStopped = MirrorImageStatusState(C.MIRROR_IMAGE_STATUS_STATE_STOPPED)
)

// String represents the MirrorImageStatusState as a short string.
func (state MirrorImageStatusState) String() (s string) {
	switch state {
	case MirrorImageStatusStateUnknown:
		s = "unknown"
	case MirrorImageStatusStateError:
		s = "error"
	case MirrorImageStatusStateSyncing:
		s = "syncing"
	case MirrorImageStatusStateStartingReplay:
		s = "starting_replay"
	case MirrorImageStatusStateReplaying:
		s = "replaying"
	case MirrorImageStatusStateStoppingReplay:
		s = "stopping_replay"
	case MirrorImageStatusStateStopped:
		s = "stopped"
	default:
		s = fmt.Sprintf("unknown(%d)", state)
	}
	return s
}

// SiteMirrorImageStatus contains information pertaining to the status of
// a mirrored image within a site.
type SiteMirrorImageStatus struct {
	MirrorUUID  string
	State       MirrorImageStatusState
	Description string
	LastUpdate  int64
	Up          bool
}

// GlobalMirrorImageStatus contains information pertaining to the global
// status of a mirrored image. It contains general information as well
// as per-site information stored in the SiteStatuses slice.
type GlobalMirrorImageStatus struct {
	Name         string
	Info         MirrorImageInfo
	SiteStatuses []SiteMirrorImageStatus
}

// LocalStatus returns one SiteMirrorImageStatus item from the SiteStatuses
// slice that corresponds to the local site's status. If the local status
// is not found than the error ErrNotExist will be returned.
func (gmis GlobalMirrorImageStatus) LocalStatus() (SiteMirrorImageStatus, error) {
	var (
		ss  SiteMirrorImageStatus
		err error = ErrNotExist
	)
	for i := range gmis.SiteStatuses {
		// I couldn't find it explicitly documented, but a site mirror uuid
		// of an empty string indicates that this is the local site.
		// This pattern occurs in both the pybind code and ceph c++.
		if gmis.SiteStatuses[i].MirrorUUID == "" {
			ss = gmis.SiteStatuses[i]
			err = nil
			break
		}
	}
	return ss, err
}

type siteArray [cutil.MaxIdx]C.rbd_mirror_image_site_status_t

// GetGlobalMirrorStatus returns status information pertaining to the state
// of the images's mirroring.
//
// Implements:
//
//	int rbd_mirror_image_get_global_status(
//	  rbd_image_t image,
//	  rbd_mirror_image_global_status_t *mirror_image_global_status,
//	  size_t status_size);
func (image *Image) GetGlobalMirrorStatus() (GlobalMirrorImageStatus, error) {
	if err := image.validate(imageIsOpen); err != nil {
		return GlobalMirrorImageStatus{}, err
	}

	s := C.rbd_mirror_image_global_status_t{}
	ret := C.rbd_mirror_image_get_global_status(
		image.image,
		&s,
		C.sizeof_rbd_mirror_image_global_status_t)
	if err := getError(ret); err != nil {
		return GlobalMirrorImageStatus{}, err
	}
	defer C.rbd_mirror_image_global_status_cleanup(&s)

	status := newGlobalMirrorImageStatus(&s)
	return status, nil
}

func newGlobalMirrorImageStatus(
	s *C.rbd_mirror_image_global_status_t) GlobalMirrorImageStatus {

	status := GlobalMirrorImageStatus{
		Name:         C.GoString(s.name),
		Info:         convertMirrorImageInfo(&s.info),
		SiteStatuses: make([]SiteMirrorImageStatus, s.site_statuses_count),
	}
	// use the "Sven Technique" to treat the C pointer as a go slice temporarily
	sscs := (*siteArray)(unsafe.Pointer(s.site_statuses))[:s.site_statuses_count:s.site_statuses_count]
	for i := C.uint32_t(0); i < s.site_statuses_count; i++ {
		ss := sscs[i]
		status.SiteStatuses[i] = SiteMirrorImageStatus{
			MirrorUUID:  C.GoString(ss.mirror_uuid),
			State:       MirrorImageStatusState(ss.state),
			Description: C.GoString(ss.description),
			LastUpdate:  int64(ss.last_update),
			Up:          bool(ss.up),
		}
	}
	return status
}

// CreateMirrorSnapshot creates a snapshot for image propagation to mirrors.
//
// Implements:
//
//	int rbd_mirror_image_create_snapshot(rbd_image_t image,
//	                                     uint64_t *snap_id);
func (image *Image) CreateMirrorSnapshot() (uint64, error) {
	var snapID C.uint64_t
	ret := C.rbd_mirror_image_create_snapshot(
		image.image,
		&snapID)
	return uint64(snapID), getError(ret)
}

// MirrorImageStatusSummary returns a map of images statuses and the count
// of images with said status.
//
// Implements:
//
//	int rbd_mirror_image_status_summary(
//	  rados_ioctx_t io_ctx, rbd_mirror_image_status_state_t *states, int *counts,
//	  size_t *maxlen);
func MirrorImageStatusSummary(
	ioctx *rados.IOContext) (map[MirrorImageStatusState]uint, error) {
	// ideally, we already know the size of the arrays - they should be
	// the size of all the values of the rbd_mirror_image_status_state_t
	// enum. But the C api doesn't enforce this so we give a little
	// wiggle room in case the server returns values outside the enum
	// we know about. This is the only case I can think of that we'd
	// be able to get -ERANGE.
	var (
		cioctx  = cephIoctx(ioctx)
		err     error
		cStates []C.rbd_mirror_image_status_state_t
		cCounts []C.int
		cSize   C.size_t
	)
	retry.WithSizes(16, 1<<16, func(size int) retry.Hint {
		cSize = C.size_t(size)
		cStates = make([]C.rbd_mirror_image_status_state_t, cSize)
		cCounts = make([]C.int, cSize)
		ret := C.rbd_mirror_image_status_summary(
			cioctx,
			(*C.rbd_mirror_image_status_state_t)(&cStates[0]),
			(*C.int)(&cCounts[0]),
			&cSize)
		err = getErrorIfNegative(ret)
		return retry.Size(int(cSize)).If(err == errRange)
	})
	if err != nil {
		return nil, err
	}

	m := map[MirrorImageStatusState]uint{}
	for i := 0; i < int(cSize); i++ {
		s := MirrorImageStatusState(cStates[i])
		m[s] = uint(cCounts[i])
	}
	return m, nil
}

// SetMirrorSiteName sets the site name, used for rbd mirroring, for the ceph
// cluster associated with the provided rados connection.
//
// Implements:
//
//	int rbd_mirror_site_name_set(rados_t cluster,
//	                             const char *name);
func SetMirrorSiteName(conn *rados.Conn, name string) error {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	ret := C.rbd_mirror_site_name_set(
		C.rados_t(conn.Cluster()),
		cName)
	return getError(ret)
}

// GetMirrorSiteName gets the site name, used for rbd mirroring, for the ceph
// cluster associated with the provided rados connection.
//
// Implements:
// int rbd_mirror_site_name_get(rados_t cluster,
//
//	char *name, size_t *max_len);
func GetMirrorSiteName(conn *rados.Conn) (string, error) {

	var (
		cluster = C.rados_t(conn.Cluster())
		err     error
		buf     []byte
		cSize   C.size_t
	)
	retry.WithSizes(1024, 1<<16, func(size int) retry.Hint {
		cSize = C.size_t(size)
		buf = make([]byte, cSize)
		ret := C.rbd_mirror_site_name_get(
			cluster,
			(*C.char)(unsafe.Pointer(&buf[0])),
			&cSize)
		err = getErrorIfNegative(ret)
		return retry.Size(int(cSize)).If(err == errRange)
	})
	if err != nil {
		return "", err
	}
	// the C code sets the size including null byte
	return string(buf[:cSize-1]), nil
}

// CreateMirrorPeerBootstrapToken returns a token value, representing the
// cluster and pool associated with the given IO context,  that can be provided
// to ImportMirrorPeerBootstrapToken in order to set up mirroring between
// pools.
//
// Implements:
//
//	int rbd_mirror_peer_bootstrap_create(
//	  rados_ioctx_t io_ctx, char *token, size_t *max_len);
func CreateMirrorPeerBootstrapToken(ioctx *rados.IOContext) (string, error) {
	var (
		cioctx = cephIoctx(ioctx)
		err    error
		buf    []byte
		cSize  C.size_t
	)
	retry.WithSizes(1024, 1<<16, func(size int) retry.Hint {
		cSize = C.size_t(size)
		buf = make([]byte, cSize)
		ret := C.rbd_mirror_peer_bootstrap_create(
			cioctx,
			(*C.char)(unsafe.Pointer(&buf[0])),
			&cSize)
		err = getErrorIfNegative(ret)
		return retry.Size(int(cSize)).If(err == errRange)
	})
	if err != nil {
		return "", err
	}
	// the C code sets the size including null byte
	return string(buf[:cSize-1]), nil
}

// MirrorPeerDirection is used to indicate what direction data is mirrored.
type MirrorPeerDirection int

const (
	// MirrorPeerDirectionRx is equivalent to RBD_MIRROR_PEER_DIRECTION_RX
	MirrorPeerDirectionRx = MirrorPeerDirection(C.RBD_MIRROR_PEER_DIRECTION_RX)
	// MirrorPeerDirectionTx is equivalent to RBD_MIRROR_PEER_DIRECTION_TX
	MirrorPeerDirectionTx = MirrorPeerDirection(C.RBD_MIRROR_PEER_DIRECTION_TX)
	// MirrorPeerDirectionRxTx is equivalent to RBD_MIRROR_PEER_DIRECTION_RX_TX
	MirrorPeerDirectionRxTx = MirrorPeerDirection(C.RBD_MIRROR_PEER_DIRECTION_RX_TX)
)

// ImportMirrorPeerBootstrapToken applies the provided bootstrap token to the
// pool associated with the IO context to create a mirroring relationship
// between pools. The direction parameter controls if data in the pool is a
// source, destination, or both.
//
// Implements:
//
//	int rbd_mirror_peer_bootstrap_import(
//	  rados_ioctx_t io_ctx, rbd_mirror_peer_direction_t direction,
//	  const char *token);
func ImportMirrorPeerBootstrapToken(
	ioctx *rados.IOContext, direction MirrorPeerDirection, token string) error {
	// instead of taking a length, rbd_mirror_peer_bootstrap_import assumes a
	// null terminated "c string". We don't use CString because we don't use
	// Go's string type as we don't want to treat the token as something users
	// should interpret.  If we were doing CString we'd be doing a copy anyway.
	cToken := C.CString(token)
	defer C.free(unsafe.Pointer(cToken))

	ret := C.rbd_mirror_peer_bootstrap_import(
		cephIoctx(ioctx),
		C.rbd_mirror_peer_direction_t(direction),
		cToken)
	return getError(ret)
}

// GlobalMirrorImageIDAndStatus values contain an ID string for a RBD image
// and that image's GlobalMirrorImageStatus.
type GlobalMirrorImageIDAndStatus struct {
	ID     string
	Status GlobalMirrorImageStatus
}

// iterBufSize is intentionally not a constant. The unit tests alter
// this value in order to get more code coverage w/o needing to create
// very many images.
var iterBufSize = 64

// MirrorImageGlobalStatusList returns a slice of GlobalMirrorImageIDAndStatus.
// If the length of the returned slice equals max, the next chunk of the list
// can be obtained by setting start to the ID of the last item of the returned
// slice. If max is 0 a slice of all items is returned.
//
// Implements:
// int rbd_mirror_image_status_list(rados_ioctx_t p,
//
//	const char *start_id, size_t max, char **image_ids,
//	rbd_mirror_image_status_t *images, size_t *len)
func MirrorImageGlobalStatusList(
	ioctx *rados.IOContext, start string, max int) ([]GlobalMirrorImageIDAndStatus, error) {
	var (
		result   []GlobalMirrorImageIDAndStatus
		fetchAll bool
	)
	if max <= 0 {
		max = iterBufSize
		fetchAll = true
	}
	chunk := make([]GlobalMirrorImageIDAndStatus, max)
	for {
		length, err := mirrorImageGlobalStatusList(ioctx, start, chunk)
		if err != nil {
			return nil, err
		}
		result = append(result, chunk[:length]...)
		if !fetchAll || length < max {
			break
		}
		start = chunk[length-1].ID
	}
	return result, nil
}

func mirrorImageGlobalStatusList(
	ioctx *rados.IOContext, start string,
	results []GlobalMirrorImageIDAndStatus) (int, error) {
	// this C function is treated like a "batch" iterator. Based on it's
	// design it appears expected to call it multiple times to get
	// the entire result.
	cStart := C.CString(start)
	defer C.free(unsafe.Pointer(cStart))

	var (
		max    = C.size_t(len(results))
		length = C.size_t(0)
		ids    = make([]*C.char, len(results))
		images = make([]C.rbd_mirror_image_global_status_t, len(results))
	)
	ret := C.rbd_mirror_image_global_status_list(
		cephIoctx(ioctx),
		cStart,
		max,
		&ids[0],
		&images[0],
		&length)
	if err := getError(ret); err != nil {
		return 0, err
	}
	for i := 0; i < int(length); i++ {
		results[i].ID = C.GoString(ids[i])
		results[i].Status = newGlobalMirrorImageStatus(&images[0])
	}
	C.rbd_mirror_image_global_status_list_cleanup(
		&ids[0],
		&images[0],
		length)
	return int(length), getError(ret)
}

// MirrorImageGlobalStatusIter provide methods for iterating over all
// the GlobalMirrorImageIdAndStatus values in a pool.
type MirrorImageGlobalStatusIter struct {
	ioctx *rados.IOContext

	buf    []GlobalMirrorImageIDAndStatus
	lastID string
}

// NewMirrorImageGlobalStatusIter creates a new iterator type ready for use.
func NewMirrorImageGlobalStatusIter(ioctx *rados.IOContext) *MirrorImageGlobalStatusIter {
	return &MirrorImageGlobalStatusIter{
		ioctx: ioctx,
	}
}

// Next fetches one GlobalMirrorImageIDAndStatus value or a nil value if
// iteration is exhausted. The error return will be non-nil if an underlying
// error fetching more values occurred.
func (iter *MirrorImageGlobalStatusIter) Next() (*GlobalMirrorImageIDAndStatus, error) {
	if len(iter.buf) == 0 {
		if err := iter.fetch(); err != nil {
			return nil, err
		}
	}
	if len(iter.buf) == 0 {
		return nil, nil
	}
	item := iter.buf[0]
	iter.lastID = item.ID
	iter.buf = iter.buf[1:]
	return &item, nil
}

// Close terminates iteration regardless if iteration was completed and
// frees any associated resources.
//
// Deprecated: not required
func (*MirrorImageGlobalStatusIter) Close() error {
	return nil
}

func (iter *MirrorImageGlobalStatusIter) fetch() error {
	iter.buf = nil
	items := make([]GlobalMirrorImageIDAndStatus, iterBufSize)
	n, err := mirrorImageGlobalStatusList(
		iter.ioctx,
		iter.lastID,
		items)
	if err != nil {
		return err
	}
	if n > 0 {
		iter.buf = items[:n]
	}
	return nil
}

// MirrorImageInfoItem contains an ID string for a RBD image and that image's
// ImageMirrorMode and MirrorImageInfo.
type MirrorImageInfoItem struct {
	ID   string
	Mode ImageMirrorMode
	Info MirrorImageInfo
}

// MirrorImageInfoList returns a slice of MirrorImageInfoItem. If the length of
// the returned slice equals max, the next chunk of the list can be obtained by
// setting start to the ID of the last item of the returned slice. The returned
// items are filtered by the mirror mode specified with modeFilter. If max is 0
// a slice of all items is returned.
//
// Implements:
// int rbd_mirror_image_info_list(
//
//	rados_ioctx_t p, rbd_mirror_image_mode_t *mode_filter,
//	const char *start_id, size_t max, char **image_ids,
//	rbd_mirror_image_mode_t *mode_entries,
//	rbd_mirror_image_info_t *info_entries, size_t *num_entries)
func MirrorImageInfoList(
	ioctx *rados.IOContext, modeFilter ImageMirrorModeFilter, start string,
	max int) ([]MirrorImageInfoItem, error) {
	var (
		result   []MirrorImageInfoItem
		fetchAll bool
	)
	if max <= 0 {
		max = iterBufSize
		fetchAll = true
	}
	chunk := make([]MirrorImageInfoItem, max)
	for {
		length, err := mirrorImageInfoList(ioctx, start, modeFilter, chunk)
		if err != nil {
			return nil, err
		}
		result = append(result, chunk[:length]...)
		if !fetchAll || length < max {
			break
		}
		start = chunk[length-1].ID
	}
	return result, nil
}

func mirrorImageInfoList(ioctx *rados.IOContext, start string,
	modeFilter ImageMirrorModeFilter, results []MirrorImageInfoItem) (int, error) {

	cStart := C.CString(start)
	defer C.free(unsafe.Pointer(cStart))

	var (
		max           = C.size_t(len(results))
		length        = C.size_t(0)
		ids           = make([]*C.char, len(results))
		modes         = make([]C.rbd_mirror_image_mode_t, len(results))
		infos         = make([]C.rbd_mirror_image_info_t, len(results))
		modeFilterPtr *C.rbd_mirror_image_mode_t
	)
	if modeFilter != nil {
		cMode := C.rbd_mirror_image_mode_t(modeFilter.mode())
		modeFilterPtr = &cMode
	}
	ret := C.rbd_mirror_image_info_list(
		cephIoctx(ioctx),
		modeFilterPtr,
		cStart,
		max,
		&ids[0],
		&modes[0],
		&infos[0],
		&length,
	)
	if err := getError(ret); err != nil {
		return 0, err
	}
	for i := 0; i < int(length); i++ {
		results[i].ID = C.GoString(ids[i])
		results[i].Mode = ImageMirrorMode(modes[i])
		results[i].Info = convertMirrorImageInfo(&infos[i])
	}
	C.rbd_mirror_image_info_list_cleanup(
		&ids[0],
		&infos[0],
		length)
	return int(length), getError(ret)
}

// MirrorImageInfoIter provide methods for iterating over all
// the MirrorImageInfoItem values in a pool.
type MirrorImageInfoIter struct {
	ioctx *rados.IOContext

	modeFilter ImageMirrorModeFilter
	buf        []MirrorImageInfoItem
	lastID     string
}

// NewMirrorImageInfoIter creates a new iterator ready for use.
func NewMirrorImageInfoIter(ioctx *rados.IOContext, modeFilter ImageMirrorModeFilter) *MirrorImageInfoIter {
	return &MirrorImageInfoIter{
		ioctx:      ioctx,
		modeFilter: modeFilter,
	}
}

// Next fetches one MirrorImageInfoItem value or a nil value if iteration is
// exhausted. The error return will be non-nil if an underlying error fetching
// more values occurred.
func (iter *MirrorImageInfoIter) Next() (*MirrorImageInfoItem, error) {
	if len(iter.buf) == 0 {
		if err := iter.fetch(); err != nil {
			return nil, err
		}
		if len(iter.buf) == 0 {
			return nil, nil
		}
		iter.lastID = iter.buf[len(iter.buf)-1].ID
	}
	item := iter.buf[0]
	iter.buf = iter.buf[1:]
	return &item, nil
}

func (iter *MirrorImageInfoIter) fetch() error {
	iter.buf = nil
	items := make([]MirrorImageInfoItem, iterBufSize)
	n, err := mirrorImageInfoList(
		iter.ioctx,
		iter.lastID,
		iter.modeFilter,
		items)
	if err != nil {
		return err
	}
	if n > 0 {
		iter.buf = items[:n]
	}
	return nil
}

// MirrorImageInstanceIDItem contains an ID string for a RBD image and
// its corresponding mirrored image's Instance ID.
type MirrorImageInstanceIDItem struct {
	ID         string
	InstanceID string
}

// MirrorImageInstanceIDList returns a slice of MirrorImageInstanceIDItem. If
// the length of the returned slice equals max, the next chunk of the list can
// be obtained by setting start to the ID of the last item of the returned slice.
// If max is 0 a slice of all items is returned.
//
// Implements:
// int rbd_mirror_image_instance_id_list(
//
//	rados_ioctx_t io_ctx,
//	const char *start_id,
//	size_t max, char **image_ids,
//	char **instance_ids,
//	size_t *len)
func MirrorImageInstanceIDList(
	ioctx *rados.IOContext, start string,
	max int) ([]MirrorImageInstanceIDItem, error) {
	var (
		result   []MirrorImageInstanceIDItem
		fetchAll bool
	)
	if max <= 0 {
		max = iterBufSize
		fetchAll = true
	}
	chunk := make([]MirrorImageInstanceIDItem, max)
	for {
		length, err := mirrorImageInstanceIDList(ioctx, start, chunk)
		if err != nil {
			return nil, err
		}
		result = append(result, chunk[:length]...)
		if !fetchAll || length < max {
			break
		}
		start = chunk[length-1].ID
	}
	return result, nil
}

func mirrorImageInstanceIDList(ioctx *rados.IOContext, start string,
	results []MirrorImageInstanceIDItem) (int, error) {

	cStart := C.CString(start)
	defer C.free(unsafe.Pointer(cStart))

	var (
		max         = C.size_t(len(results))
		length      = C.size_t(0)
		ids         = make([]*C.char, len(results))
		instanceIDs = make([]*C.char, len(results))
	)
	ret := C.rbd_mirror_image_instance_id_list(
		cephIoctx(ioctx),
		cStart,
		max,
		&ids[0],
		&instanceIDs[0],
		&length,
	)
	if err := getError(ret); err != nil {
		return 0, err
	}
	for i := 0; i < int(length); i++ {
		results[i].ID = C.GoString(ids[i])
		results[i].InstanceID = C.GoString(instanceIDs[i])
	}
	C.rbd_mirror_image_instance_id_list_cleanup(
		&ids[0],
		&instanceIDs[0],
		length)
	return int(length), getError(ret)
}

// MirrorImageInstanceIDIter provide methods for iterating over all
// the MirrorImageInstanceIDItem values in a pool.
type MirrorImageInstanceIDIter struct {
	ioctx *rados.IOContext

	buf    []MirrorImageInstanceIDItem
	lastID string
}

// NewMirrorImageInstanceIDIter creates a new iterator ready for use.
func NewMirrorImageInstanceIDIter(ioctx *rados.IOContext) *MirrorImageInstanceIDIter {
	return &MirrorImageInstanceIDIter{
		ioctx: ioctx,
	}
}

// Next fetches one MirrorImageInstanceIDItem value or a nil value if iteration is
// exhausted. The error return will be non-nil if an underlying error fetching
// more values occurred.
func (iter *MirrorImageInstanceIDIter) Next() (*MirrorImageInstanceIDItem, error) {
	if len(iter.buf) == 0 {
		if err := iter.fetch(); err != nil {
			return nil, err
		}
		if len(iter.buf) == 0 {
			return nil, nil
		}
		iter.lastID = iter.buf[len(iter.buf)-1].ID
	}
	item := iter.buf[0]
	iter.buf = iter.buf[1:]
	return &item, nil
}

func (iter *MirrorImageInstanceIDIter) fetch() error {
	iter.buf = nil
	items := make([]MirrorImageInstanceIDItem, iterBufSize)
	n, err := mirrorImageInstanceIDList(
		iter.ioctx,
		iter.lastID,
		items)
	if err != nil {
		return err
	}
	if n > 0 {
		iter.buf = items[:n]
	}
	return nil
}
