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

package util

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

// FindmntResult represents the output from findmnt -J command.
type FindmntResult struct {
	Filesystems []FindmntFilesystem `json:"filesystems"`
}

// FindmntFilesystem represents a single filesystem entry from findmnt.
type FindmntFilesystem struct {
	Source FindmntSource `json:"source"`
	Target string        `json:"target,omitempty"`
}

// FindmntSource represents a device source from findmnt JSON output.
// It automatically parses both filesystem volumes ("/dev/nvme0n2") and
// block volumes ("devtmpfs[/nvme0n1]") during JSON unmarshaling.
type FindmntSource string

// String returns the parsed device path.
func (f FindmntSource) String() string {
	return string(f)
}

// UnmarshalText implements encoding.TextUnmarshaler to parse device paths
// from findmnt output. It handles both filesystem and block volume formats.
func (f *FindmntSource) UnmarshalText(text []byte) error {
	*f = FindmntSource(parseNVMEDeviceFromRawSource(string(text)))

	return nil
}

// devtmpfsRegex matches NVMe devices in devtmpfs format: devtmpfs[/nvme0n1] or devtmpfs[nvme0n1].
var devtmpfsRegex = regexp.MustCompile(`devtmpfs\[/?(nvme\d+n\d+)\]`)

// parseNVMEDeviceFromRawSource parses findmnt source to extract NVMe device path.
// Handles both filesystem volumes ("/dev/nvme0n2") and block volumes ("devtmpfs[/nvme0n1]").
func parseNVMEDeviceFromRawSource(source string) string {
	// Case 1: devtmpfs[/nvme0n1] or devtmpfs[nvme0n1] format (block volumes)
	if matches := devtmpfsRegex.FindStringSubmatch(source); len(matches) > 1 {
		// Regex captures just "nvme0n1", so prepend /dev/
		return "/dev/" + matches[1]
	}

	// Case 2: Direct device path like /dev/nvme0n2 (filesystem volumes)
	if strings.HasPrefix(source, "/dev/nvme") {
		return source
	}

	return ""
}

// GetDeviceFromMountpoint returns the NVMe device path for a given mount point.
func GetDeviceFromMountpoint(ctx context.Context, mountpoint string) (string, error) {
	stdout, _, err := util.ExecCommandWithTimeout(ctx, 5*time.Second,
		"findmnt", "--mountpoint", mountpoint, "--output", "SOURCE",
		"--noheadings", "--first-only", "-J")
	if err != nil {
		log.DebugLog(ctx, "findmnt failed for %s: %v", mountpoint, err)

		return "", fmt.Errorf("findmnt failed: %w", err)
	}

	var result FindmntResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		return "", fmt.Errorf("failed to parse findmnt output: %w", err)
	}

	if len(result.Filesystems) == 0 {
		return "", nil
	}

	device := result.Filesystems[0].Source.String()
	if device != "" {
		log.DebugLog(ctx, "found device %s for mountpoint %s", device, mountpoint)
	}

	return device, nil
}

// GetAllNVMeMountedDevices returns a map of all currently mounted NVMe devices.
// Only checks staging paths to avoid counting the same device multiple times
// (staging, publish, and pod paths all point to the same device).
func GetAllNVMeMountedDevices(ctx context.Context) (map[string]bool, error) {
	stdout, _, err := util.ExecCommandWithTimeout(ctx, 5*time.Second,
		"findmnt", "-J", "--list", "--output", "SOURCE,TARGET")
	if err != nil {
		return nil, fmt.Errorf("failed to run findmnt: %w", err)
	}

	var result FindmntResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		return nil, fmt.Errorf("failed to parse findmnt output: %w", err)
	}

	mountedDevices := make(map[string]bool)
	for _, fs := range result.Filesystems {
		// Only check NVMe CSI staging paths
		isNVMeOFStagingFS := strings.Contains(fs.Target, "nvmeof.csi.ceph.com") &&
			strings.Contains(fs.Target, "/globalmount/")

		isNVMeOFStagingBlock := strings.Contains(fs.Target, "volumeDevices") &&
			strings.Contains(fs.Target, "/staging/")

		if !isNVMeOFStagingFS && !isNVMeOFStagingBlock {
			continue
		}

		device := fs.Source.String()
		// Only consider valid NVMe devices
		if device != "" && strings.HasPrefix(device, "/dev/nvme") {
			mountedDevices[device] = true
			log.DebugLog(ctx, "Found mounted NVMe device: %s at %s", device, fs.Target)
		}
	}

	log.DebugLog(ctx, "Found %d mounted NVMe devices total", len(mountedDevices))

	return mountedDevices, nil
}
