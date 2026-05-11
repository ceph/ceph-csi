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
	"os"
	"path/filepath"
	"strconv"
	"strings"

	librbd "github.com/ceph/go-ceph/rbd"

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

	// cgroup v2 base path.
	cgroupV2BasePath = "/sys/fs/cgroup"

	// Kubernetes cgroup slices based on QoS class.
	kubepodsBestEffortSlice = "kubepods-besteffort.slice"
	kubepodsBurstableSlice  = "kubepods-burstable.slice"
	kubepodsGuaranteedSlice = "kubepods.slice"

	// io.max file for cgroup v2.
	ioMaxFile = "io.max"
)

// qosClassInfo holds pre-computed cgroup path information for each QoS class.
// Ordered by most common QoS class in production (Guaranteed > Burstable > BestEffort)
// to minimize stat() syscalls on average.
var qosClassInfo = [...]struct {
	name      string
	sliceName string
	podPrefix string
}{
	{"Guaranteed", kubepodsGuaranteedSlice, "kubepods-pod"},
	{"Burstable", kubepodsBurstableSlice, "kubepods-burstable-pod"},
	{"BestEffort", kubepodsBestEffortSlice, "kubepods-besteffort-pod"},
}

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
		maxReadIops:  "max",
		maxWriteIops: "max",
		maxReadBps:   "max",
		maxWriteBps:  "max",
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
func getDeviceID(ctx context.Context, devicePath string) (string, error) {
	// Get the real path if devicePath is a symlink.
	realPath, err := filepath.EvalSymlinks(devicePath)
	if err != nil {
		log.ErrorLog(ctx, "failed to resolve symlink for device %s: %v", devicePath, err)

		return "", err
	}

	// Read /proc/partitions to get major:minor for the device.
	data, err := os.ReadFile("/proc/partitions")
	if err != nil {
		log.ErrorLog(ctx, "failed to read /proc/partitions: %v", err)

		return "", err
	}

	deviceName := filepath.Base(realPath)
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[3] == deviceName {
			major := fields[0]
			minor := fields[1]

			return fmt.Sprintf("%s:%s", major, minor), nil
		}
	}

	return "", fmt.Errorf("device %s not found in /proc/partitions", deviceName)
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

// findPodCgroupPath tries to find the pod's cgroup path by attempting all QoS classes.
// Optimized to minimize allocations and stat() syscalls per lookup.
func findPodCgroupPath(ctx context.Context, podUID string) (string, error) {
	if podUID == "" {
		return "", errors.New("pod UID is empty")
	}

	// Normalize pod UID once: replace hyphens with underscores.
	normalizedUID := strings.ReplaceAll(podUID, "-", "_")

	// Try each QoS class path using pre-computed path prefixes.
	// Ordered by most common in production: Guaranteed → Burstable → BestEffort.
	// This minimizes average syscalls (1-2 stat() calls instead of always 3).
	for i := range qosClassInfo {
		qos := &qosClassInfo[i]
		// Construct pod slice name directly (e.g., "kubepods-guaranteed-pod<uid>.slice").
		// Using string concatenation instead of fmt.Sprintf reduces allocations.
		podSliceName := qos.podPrefix + normalizedUID + ".slice"
		podPath := filepath.Join(cgroupV2BasePath, qos.sliceName, podSliceName)

		if _, err := os.Stat(podPath); err == nil {
			log.DebugLog(ctx, "found pod cgroup path: %s for QoS class: %s", podPath, qos.name)

			return podPath, nil
		}
	}

	return "", fmt.Errorf("pod cgroup path not found for pod UID: %s", podUID)
}

// findContainerCgroups finds all container cgroup directories within a pod's cgroup.
func findContainerCgroups(ctx context.Context, podCgroupPath string) ([]string, error) {
	entries, err := os.ReadDir(podCgroupPath)
	if err != nil {
		log.ErrorLog(ctx, "failed to read pod cgroup directory %s: %v", podCgroupPath, err)

		return nil, err
	}

	var containerPaths []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Container scopes follow pattern: crio-<container-id>.scope or containerd-<container-id>.scope.
		if strings.HasPrefix(name, "crio-") || strings.HasPrefix(name, "containerd-") {
			containerPath := filepath.Join(podCgroupPath, name)
			containerPaths = append(containerPaths, containerPath)
			log.DebugLog(ctx, "found container cgroup: %s", containerPath)
		}
	}

	if len(containerPaths) == 0 {
		return nil, fmt.Errorf("no container cgroups found in pod path: %s", podCgroupPath)
	}

	return containerPaths, nil
}

// applyCgroupQoS applies QoS limits to all containers in a pod.
func (qos *cgroupQoS) applyCgroupQoS(ctx context.Context, podUID string) error {
	if qos.deviceID == "" {
		return errors.New("device ID is not set for cgroup QoS")
	}

	// Find the pod's cgroup path.
	podCgroupPath, err := findPodCgroupPath(ctx, podUID)
	if err != nil {
		return fmt.Errorf("failed to find pod cgroup path for pod %s: %w", podUID, err)
	}

	// Find all container cgroups within the pod.
	containerPaths, err := findContainerCgroups(ctx, podCgroupPath)
	if err != nil {
		log.ErrorLog(ctx, "failed to find container cgroups for pod %s: %v", podUID, err)

		return err
	}

	// Apply io.max to each container.
	// Per cgroup v2 conventions, writing a single device line is atomic —
	// the kernel merges it with existing entries for other devices.
	ioMaxValue := qos.formatIOMax()
	for _, containerPath := range containerPaths {
		ioMaxPath := filepath.Join(containerPath, ioMaxFile)
		log.DebugLog(ctx, "applying QoS to container: %s, io.max: %s", containerPath, ioMaxValue)

		err = writeIOMax(ioMaxPath, ioMaxValue)
		if err != nil {
			log.ErrorLog(ctx, "failed to write io.max for device %s at %s: %v",
				qos.deviceID, ioMaxPath, err)

			return err
		}

		log.DebugLog(ctx, "successfully applied QoS to container: %s for device %s",
			containerPath, qos.deviceID)
	}

	log.UsefulLog(ctx, "successfully applied cgroup QoS to %d containers in pod %s",
		len(containerPaths), podUID)

	return nil
}

// validateCgroupQoSParams validates cgroup QoS parameters.
func validateCgroupQoSParams(params map[string]string) error {
	for key := range qosParamToMetadataKey {
		if val, ok := params[key]; ok && val != "" {
			parsed, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %s, must be a positive integer", key, val)
			}
			if parsed <= 0 {
				return fmt.Errorf("invalid value for %s: %s, must be greater than 0", key, val)
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
