/*
Copyright 2026 The Ceph-CSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rbd

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	librbd "github.com/ceph/go-ceph/rbd"
	"golang.org/x/sys/unix"

	"github.com/ceph/ceph-csi/internal/util/log"
)

// QoSHandler defines the interface for different QoS implementations.
// This abstraction allows clean separation between:
//   - Traditional NBD QoS (applied via librbd to rbd-nbd device)
//   - Cgroup v2 QoS (applied via cgroup io.max to krbd device)
type QoSHandler interface {
	// HasParams checks if this QoS type has parameters in the request.
	HasParams(params map[string]string) bool

	// Validate validates the QoS parameters.
	Validate(params map[string]string) error

	// Apply saves QoS parameters and applies them to the volume.
	// For cgroup QoS: stores metadata to be applied later during NodePublishVolume.
	// For NBD QoS: applies immediately to the rbd-nbd device configuration.
	Apply(ctx context.Context, params map[string]string) error

	// Clear removes all QoS settings for this type.
	Clear(ctx context.Context) error
}

const (

	// qosMetadataKeyPrefix is the prefix for QoS metadata keys stored in RBD image.
	// Keys starting with `.` are not copied to cloned or snapshotted volumes.
	qosMetadataKeyPrefix = ".rbd.csi.ceph.com/"

	// RBD image metadata keys for cgroup QoS parameters.
	// Prefixed to prevent propagation during clone/snapshot operations.
	qosMetadataMaxReadIops  = qosMetadataKeyPrefix + "cgroup_qos_max_read_iops"
	qosMetadataMaxWriteIops = qosMetadataKeyPrefix + "cgroup_qos_max_write_iops"
	qosMetadataMaxReadBps   = qosMetadataKeyPrefix + "cgroup_qos_max_read_bps"
	qosMetadataMaxWriteBps  = qosMetadataKeyPrefix + "cgroup_qos_max_write_bps"

	// cgroupQoSMaxLimit is the cgroup v2 value for unlimited I/O.
	cgroupQoSMaxLimit = "max"

	// io.max file for cgroup v2.
	ioMaxFile = "io.max"

	// Cgroup v2 base paths for systemd and cgroupfs drivers.
	cgroupV2SystemdBase  = "/sys/fs/cgroup/kubepods.slice"
	cgroupV2CgroupfsBase = "/sys/fs/cgroup/kubepods"
)

// qosParamToMetadataKey maps VolumeAttributesClass parameter keys to RBD image metadata keys.
// Metadata keys are prefixed with `.rbd.csi.ceph.com/` to prevent copying during clone/snapshot.
var qosParamToMetadataKey = map[string]string{
	maxReadIops:  qosMetadataMaxReadIops,
	maxWriteIops: qosMetadataMaxWriteIops,
	maxReadBps:   qosMetadataMaxReadBps,
	maxWriteBps:  qosMetadataMaxWriteBps,
}

// qosMetadataToParamKey is the reverse mapping: RBD image metadata keys to parameter keys.
var qosMetadataToParamKey = func() map[string]string {
	m := make(map[string]string, len(qosParamToMetadataKey))
	for k, v := range qosParamToMetadataKey {
		m[v] = k
	}

	return m
}()

// cgroupQoSHandler implements QoSHandler for cgroup v2 QoS (krbd mounter).
type cgroupQoSHandler struct {
	volume *rbdVolume
}

// newCgroupQoSHandler creates a new cgroup QoS handler.
func newCgroupQoSHandler(volume *rbdVolume) QoSHandler {
	return &cgroupQoSHandler{volume: volume}
}

// cgroupQoSKnownKeys returns the cgroup v2 QoS parameter keys.
func cgroupQoSKnownKeys() []string {
	return slices.Collect(maps.Keys(qosParamToMetadataKey))
}

// HasParams checks if cgroup v2 QoS parameters are present in the request.
// Cgroup QoS only applies to krbd-mounted volumes; rbd-nbd volumes use the
// NBD QoS handler even though the parameter names overlap.
func (h *cgroupQoSHandler) HasParams(params map[string]string) bool {
	if h.volume.Mounter == rbdNbdMounter {
		return false
	}

	return hasCgroupQoSParams(params)
}

// Validate validates cgroup v2 QoS parameters.
func (h *cgroupQoSHandler) Validate(params map[string]string) error {
	return validateCgroupQoSParams(params)
}

// Apply saves cgroup v2 QoS parameters to RBD image metadata.
// The actual QoS is applied during NodePublishVolume via cgroup io.max.
func (h *cgroupQoSHandler) Apply(ctx context.Context, params map[string]string) error {
	return h.volume.saveCgroupQoS(ctx, params)
}

// Clear removes all cgroup v2 QoS metadata from the RBD image.
func (h *cgroupQoSHandler) Clear(ctx context.Context) error {
	// Pass empty params to trigger removal of all cgroup QoS metadata.
	return h.volume.saveCgroupQoS(ctx, map[string]string{})
}

// cgroupQoS holds cgroup v2 QoS parameters.
type cgroupQoS struct {
	// Device major:minor number.
	deviceID string
	// Max read IOPS.
	maxReadIops string
	// Max write IOPS.
	maxWriteIops string
	// Max read bytes per second.
	maxReadBps string
	// Max write bytes per second.
	maxWriteBps string
}

// parseCgroupQoSParams extracts cgroup v2 QoS parameters from VolumeAttributesClass.
func parseCgroupQoSParams(params map[string]string) *cgroupQoS {
	qos := &cgroupQoS{
		maxReadIops:  cgroupQoSMaxLimit,
		maxWriteIops: cgroupQoSMaxLimit,
		maxReadBps:   cgroupQoSMaxLimit,
		maxWriteBps:  cgroupQoSMaxLimit,
	}

	if val, ok := params[maxReadIops]; ok && val != "" {
		qos.maxReadIops = val
	}
	if val, ok := params[maxWriteIops]; ok && val != "" {
		qos.maxWriteIops = val
	}
	if val, ok := params[maxReadBps]; ok && val != "" {
		qos.maxReadBps = val
	}
	if val, ok := params[maxWriteBps]; ok && val != "" {
		qos.maxWriteBps = val
	}

	return qos
}

// hasCgroupQoSParams checks if any cgroup v2 QoS parameters are present.
func hasCgroupQoSParams(params map[string]string) bool {
	cgroupParams := []string{maxReadIops, maxWriteIops, maxReadBps, maxWriteBps}
	for _, param := range cgroupParams {
		if val, ok := params[param]; ok && val != "" {
			return true
		}
	}

	return false
}

// getDeviceID returns the device major:minor number for the given device path.
func getDeviceID(devicePath string) (string, error) {
	realPath, err := filepath.EvalSymlinks(devicePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlink for device %s: %w", devicePath, err)
	}

	var st unix.Stat_t
	if err := unix.Stat(realPath, &st); err != nil {
		return "", fmt.Errorf("failed to stat device %s: %w", realPath, err)
	}

	return fmt.Sprintf("%d:%d", unix.Major(st.Rdev), unix.Minor(st.Rdev)), nil
}

// formatIOMax formats the io.max line for cgroup v2.
// Format: <major>:<minor> rbps=<value> wbps=<value> riops=<value> wiops=<value>.
func (qos *cgroupQoS) formatIOMax() string {
	return fmt.Sprintf("%s rbps=%s wbps=%s riops=%s wiops=%s",
		qos.deviceID, qos.maxReadBps, qos.maxWriteBps, qos.maxReadIops, qos.maxWriteIops)
}

// writeIOMax writes a single device's QoS line to the cgroup io.max file.
// Per cgroup v2 conventions, each write targets one device — the kernel
// merges it with existing entries for other devices atomically.
func writeIOMax(ioMaxPath, ioMaxLine string) error {
	return os.WriteFile(ioMaxPath, []byte(ioMaxLine+"\n"), 0o600)
}

// podCgroupCandidates returns candidate cgroup paths for the given pod UID,
// covering both the systemd and cgroupfs cgroup drivers.
// Systemd paths use underscores in place of dashes; cgroupfs paths keep the
// original dashed UUID.
// Ordered: Guaranteed > Burstable > BestEffort within each driver, systemd first.
func podCgroupCandidates(podUID string) []string {
	uid := strings.ReplaceAll(podUID, "-", "_")

	return []string{
		// systemd cgroup driver
		filepath.Join(cgroupV2SystemdBase,
			"kubepods-pod"+uid+".slice"),
		filepath.Join(cgroupV2SystemdBase, "kubepods-burstable.slice",
			"kubepods-burstable-pod"+uid+".slice"),
		filepath.Join(cgroupV2SystemdBase, "kubepods-besteffort.slice",
			"kubepods-besteffort-pod"+uid+".slice"),

		// cgroupfs cgroup driver
		filepath.Join(cgroupV2CgroupfsBase, "pod"+podUID),
		filepath.Join(cgroupV2CgroupfsBase, "burstable", "pod"+podUID),
		filepath.Join(cgroupV2CgroupfsBase, "besteffort", "pod"+podUID),
	}
}

// findPodCgroupPath tries to find the pod's cgroup v2 path by probing
// candidates for both the systemd and cgroupfs cgroup drivers.
func findPodCgroupPath(ctx context.Context, podUID string) (string, error) {
	if podUID == "" {
		return "", errors.New("pod UID is empty")
	}

	for _, candidate := range podCgroupCandidates(podUID) {
		ioMaxPath := filepath.Join(candidate, ioMaxFile)
		if _, err := os.Stat(ioMaxPath); err == nil {
			log.DebugLog(ctx, "found pod cgroup path: %s", candidate)

			return candidate, nil
		}
	}

	return "", fmt.Errorf("pod cgroup path not found for pod UID: %s", podUID)
}

// applyCgroupQoS applies QoS limits at the pod cgroup level.
// QoS is applied to the pod's io.max file, which automatically applies to all containers.
func (qos *cgroupQoS) applyCgroupQoS(ctx context.Context, podUID string) error {
	if qos.deviceID == "" {
		return errors.New("device ID is not set for cgroup QoS")
	}

	// Find the pod's cgroup path.
	podCgroupPath, err := findPodCgroupPath(ctx, podUID)
	if err != nil {
		return fmt.Errorf("failed to find pod cgroup path for pod %s: %w", podUID, err)
	}

	// Apply io.max at the pod level.
	// Per cgroup v2 conventions, writing a single device line is atomic —
	// the kernel merges it with existing entries for other devices.
	ioMaxPath := filepath.Join(podCgroupPath, ioMaxFile)
	ioMaxValue := qos.formatIOMax()
	log.DebugLog(ctx, "applying QoS to pod cgroup: %s, io.max: %s", podCgroupPath, ioMaxValue)

	err = writeIOMax(ioMaxPath, ioMaxValue)
	if err != nil {
		return fmt.Errorf("failed to write io.max for device %s at %s: %w",
			qos.deviceID, ioMaxPath, err)
	}

	log.UsefulLog(ctx, "successfully applied cgroup QoS to pod %s at %s",
		podUID, ioMaxPath)

	return nil
}

// validateCgroupQoSParams validates cgroup QoS parameters.
func validateCgroupQoSParams(params map[string]string) error {
	for key := range qosParamToMetadataKey {
		if val, ok := params[key]; ok && val != "" {
			// "max" means unlimited (removes the limit).
			if val == cgroupQoSMaxLimit {
				continue
			}
			parsed, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %s, must be a non-negative integer or %q", key, val, cgroupQoSMaxLimit)
			}
			if parsed < 0 {
				return fmt.Errorf("invalid value for %s: %s, must be a non-negative integer or %q", key, val, cgroupQoSMaxLimit)
			}
		}
	}

	return nil
}

// saveCgroupQoS saves cgroup v2 QoS parameters to RBD image metadata.
// If params is empty or contains no cgroup QoS keys, existing QoS metadata is removed.
func (rv *rbdVolume) saveCgroupQoS(ctx context.Context, params map[string]string) error {
	// Check if we need to remove existing QoS (when VAC is removed or keys not present).
	if !hasCgroupQoSParams(params) {
		// Remove all cgroup QoS metadata.
		for _, metadataKey := range qosParamToMetadataKey {
			err := rv.RemoveMetadata(metadataKey)
			if err != nil && !errors.Is(err, librbd.ErrNotExist) {
				return fmt.Errorf("failed to remove cgroup QoS metadata %s: %w", metadataKey, err)
			}
			log.DebugLog(ctx, "removed cgroup QoS metadata %s", metadataKey)
		}

		return nil
	}

	err := validateCgroupQoSParams(params)
	if err != nil {
		return err
	}

	// Save or update cgroup QoS parameters.
	for param, metadataKey := range qosParamToMetadataKey {
		if val, ok := params[param]; ok && val != "" {
			err := rv.SetMetadata(metadataKey, val)
			if err != nil {
				return fmt.Errorf("failed to save cgroup QoS %s: %s, %w", param, val, err)
			}
			log.DebugLog(ctx, "saved cgroup QoS metadata %s: %s", metadataKey, val)
		} else {
			// Remove metadata if parameter not provided (allows partial updates).
			err := rv.RemoveMetadata(metadataKey)
			if err != nil && !errors.Is(err, librbd.ErrNotExist) {
				return fmt.Errorf("failed to remove cgroup QoS metadata %s: %w", metadataKey, err)
			}
			log.DebugLog(ctx, "removed cgroup QoS metadata %s (not in request)", metadataKey)
		}
	}

	return nil
}

// getCgroupQoS retrieves cgroup v2 QoS parameters from RBD image metadata.
func (rv *rbdVolume) getCgroupQoS(ctx context.Context) (map[string]string, error) {
	qosParams := make(map[string]string)
	for metadataKey, param := range qosMetadataToParamKey {
		val, err := rv.GetMetadata(metadataKey)
		if err != nil && !errors.Is(err, librbd.ErrNotFound) {
			return nil, fmt.Errorf("failed to get metadata %s: %w", metadataKey, err)
		}
		if val != "" {
			qosParams[param] = val
			log.DebugLog(ctx, "retrieved cgroup QoS metadata %s: %s", metadataKey, val)
		}
	}

	return qosParams, nil
}

// applyCgroupQoSForVolume applies cgroup v2 QoS limits to the pod mounting this volume.
func (rv *rbdVolume) applyCgroupQoSForVolume(ctx context.Context, devicePath, podUID string) error {
	if podUID == "" {
		log.DebugLog(ctx, "pod UID not available, skipping cgroup QoS (podInfoOnMount may not be enabled)")

		return nil
	}

	// Retrieve cgroup QoS parameters from image metadata.
	qosParams, err := rv.getCgroupQoS(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve cgroup QoS for volume %s: %w", rv.VolID, err)
	}

	// If no QoS parameters configured, nothing to do.
	if len(qosParams) == 0 {
		log.DebugLog(ctx, "no cgroup QoS configured for volume %s", rv.VolID)

		return nil
	}

	// Get device ID (major:minor).
	qos := parseCgroupQoSParams(qosParams)
	qos.deviceID, err = getDeviceID(devicePath)
	if err != nil {
		return fmt.Errorf("failed to get device ID for %s: %w", devicePath, err)
	}

	log.UsefulLog(ctx, "applying cgroup QoS for volume %s on device %s to pod %s",
		rv.VolID, qos.deviceID, podUID)

	// Apply QoS to all containers in the pod.
	err = qos.applyCgroupQoS(ctx, podUID)
	if err != nil {
		return fmt.Errorf("failed to apply cgroup QoS for volume %s: %w", rv.VolID, err)
	}

	log.UsefulLog(ctx, "successfully applied cgroup QoS for volume %s", rv.VolID)

	return nil
}
