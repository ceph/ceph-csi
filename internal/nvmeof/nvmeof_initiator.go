/*
Copyright 2025 The Ceph-CSI Authors.

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

package nvmeof

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/kmod"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/avast/retry-go/v4"
)

const (
	// Command timeouts.
	connectTimeout    = 30 * time.Second
	disconnectTimeout = 15 * time.Second
)

// NVMeInitiator handles NVMe-oF initiator operations.
type NVMeInitiator interface {
	// LoadKernelModules ensures required kernel modules are loaded
	LoadKernelModules(ctx context.Context) error

	// ConnectSubsystem connects to an NVMe-oF subsystem
	ConnectSubsystem(ctx context.Context, req *ConnectRequest) (bool, error)

	// GetNamespaceDeviceByUUID returns the device path for a given namespace UUID
	GetNamespaceDeviceByUUID(ctx context.Context, uuid string) (string, error)
}

// ConnectRequest represents a subsystem connection request.
type ConnectRequest struct {
	SubsystemNQN string
	Listeners    []GatewayAddress
	Transport    string // "tcp"
	HostNQN      string // Optional - empty means use system default
}

// nvmeInitiator implements NVMeInitiator interface.
type nvmeInitiator struct{}

// NewNVMeInitiator creates a new NVMe-oF initiator.
func NewNVMeInitiator() NVMeInitiator {
	return &nvmeInitiator{}
}

// LoadKernelModules ensures required kernel modules are loaded.
func (ni *nvmeInitiator) LoadKernelModules(ctx context.Context) error {
	modules := []string{
		"nvme_tcp",
		"nvme_fabrics",
	}
	log.DebugLog(ctx, "Loading NVMe-oF kernel modules: %s, and %s", modules[0], modules[1])

	for _, module := range modules {
		err := kmod.Modprobe(ctx, module)
		if err != nil {
			return fmt.Errorf("failed to load kernel module %q: %w", module, err)
		}
	}

	log.DebugLog(ctx, "All NVMe-oF kernel modules: %s, and %s, loaded successfully", modules[0], modules[1])

	return nil
}

// ConnectSubsystem connects to an NVMe-oF subsystem.
func (ni *nvmeInitiator) ConnectSubsystem(ctx context.Context, req *ConnectRequest) (bool, error) {
	// Try connecting to each address until one succeeds
	var success bool
	for _, listener := range req.Listeners {
		log.DebugLog(ctx, "Connecting to NVMe-oF subsystem %s at %v:%v",
			req.SubsystemNQN, listener.Address, listener.Port)
		portStr := strconv.FormatUint(uint64(listener.Port), 10)
		// Build nvme connect command for this address
		args := []string{
			"connect",
			"-t", req.Transport,
			"-n", req.SubsystemNQN,
			"-a", listener.Address,
			"-s", portStr,
			"-l", "1800", // TODO - known value for connection timeout.move to be const.
		}

		// Add HostNQN only if specified
		if req.HostNQN != "" {
			args = append(args, "--hostnqn", req.HostNQN)
		}

		stdout, stderr, err := util.ExecCommandWithTimeout(ctx, connectTimeout, "nvme", args...)
		// Execute connection
		if err != nil {
			log.WarningLog(ctx, "Failed to connect to %s - stdout: %s, stderr: %s", listener, stdout, stderr)

			continue
		}
		success = true
		log.DebugLog(ctx, "Successfully connected to subsystem %s via %s",
			req.SubsystemNQN, listener)
	}
	if !success {
		return false, fmt.Errorf("failed to connect to any gateway address for subsystem %s", req.SubsystemNQN)
	}

	return true, nil
}

// GetNamespaceDeviceByUUID tries to find the path of the block device for the
// namespace. While attaching there can be a delay, this function retries a few
// times with a short delay.
func (ni *nvmeInitiator) GetNamespaceDeviceByUUID(ctx context.Context, uuid string) (string, error) {
	return retry.DoWithData(
		func() (string, error) {
			uuids := []string{
				formatUUID(uuid), // with dashes is most common
				uuid,
			}

			for _, pathUUID := range uuids {
				expectedPath := "/dev/disk/by-id/nvme-uuid." + pathUUID
				if _, err := os.Stat(expectedPath); err == nil {
					// Verify it's a symlink and readable
					if _, err := os.Readlink(expectedPath); err == nil {
						return expectedPath, nil
					}
				}
			}

			return "", fmt.Errorf("device path with uuid: %s not found", uuid)
		},
		// BackOffDelay is the default, starts at 100ms
		retry.Attempts(4), // defaults to 10 delays, too many
	)
}
