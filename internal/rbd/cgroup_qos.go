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

	"github.com/ceph/ceph-csi/internal/util/log"
)

const (

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
	cgroupParams := []string{maxReadIops, maxWriteIops, maxReadBps, maxWriteBps}
	for _, key := range cgroupParams {
		if val, ok := params[key]; ok && val != "" {
			if _, err := strconv.ParseInt(val, 10, 64); err != nil {
				return fmt.Errorf("invalid value for %s: %s, must be a positive integer", key, val)
			}
		}
	}

	return nil
}
