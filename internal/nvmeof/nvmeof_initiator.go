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
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/kmod"
	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	// Command timeouts.
	connectTimeout    = 30 * time.Second
	listSubsysTimeout = 60 * time.Second
)

// nvmeCtrlLossTmo is the controller loss timeout passed to nvme connect -l flag.
// This defines how long (in seconds) the kernel will retry reconnecting to a
// failed controller before giving up.
const nvmeCtrlLossTmo = "1800"

// NVMeInitiator handles NVMe-oF initiator operations.
type NVMeInitiator interface {
	// LoadKernelModules ensures required kernel modules are loaded
	LoadKernelModules(ctx context.Context) error

	// ConnectSubsystem connects to an NVMe-oF subsystem
	ConnectSubsystem(ctx context.Context, req *ConnectRequest) (bool, error)

	// GetNamespaceDeviceByUUID returns the device path for a given namespace UUID
	GetNamespaceDeviceByUUID(ctx context.Context, uuid string) (string, error)

	// DisconnectIfLastMount disconnects from the NVMe controllers for the given device path,
	// all-or-nothing (skip disconnect for all controllers
	// if any controller has another mounted namespace).
	// Safe for multipath - disconnects each controller individually.
	//
	// Example: If device /dev/nvme0n1 is connected via controllers nvme0 and nvme1,
	// and both controllers have no other mounted namespaces, both will be disconnected.
	DisconnectIfLastMount(ctx context.Context, devPath string, mountedDevices map[string]string) error
}

// ConnectRequest represents a subsystem connection request.
type ConnectRequest struct {
	SubsystemNQN string
	Listeners    []ListenerDetails
	Transport    string // "tcp"
	HostNQN      string // Optional - empty means use system default
	// Optional - In-band authentication controller secret for bi-directional authentication.
	SubsystemDhchapKey string
	// Optional - In-band authentication secret for uni-directional authentication
	HostDhchapKey string
}

// nvmeInitiator implements NVMeInitiator interface.
type nvmeInitiator struct{}

// nvmePathAddress represents a parsed NVMe path address string.
type nvmePathAddress struct {
	Traddr  string
	Trsvcid string
	SrcAddr string
}

// nvmeHost represents the structure from nvme list-subsys output.
// Example JSON structure:
// [
//   {
//     "HostNQN":"nqn.2014-08.org.nvmexpress:gdidi-zwcq5-worker-2-7mppg",
//     "HostID":"d48fbfbf-1b0e-45ce-93a4-b919224aff2c",
//     "Subsystems":[
//       {
//         "Name":"nvme-subsys0",
//         "NQN":"nqn.2016-06.io.ceph:subsystem.test-integration",
//         "Paths":[
//           {
//             "Name":"nvme0",
//             "Transport":"tcp",
//             "Address":"traddr=10.131.0.225,trsvcid=4420,src_addr=10.131.0.2",
//             "State":"live"
//           }
//         ]
//       }
//     ]
//   }
// ]

type nvmeHost struct {
	HostNQN    string `json:"HostNQN"`
	Subsystems []struct {
		NQN   string `json:"NQN"`
		Paths []struct {
			Name    string          `json:"Name"`
			Address nvmePathAddress `json:"Address"`
			State   string          `json:"State"`
		} `json:"Paths"`
	} `json:"Subsystems"`
}

// nvmeHostConnections represents a collection of NVMe host connections.
type nvmeHostConnections []nvmeHost

// UnmarshalJSON implements custom JSON unmarshaling for nvmePathAddress.
func (na *nvmePathAddress) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Parse: "traddr=10.242.64.32,trsvcid=4420,src_addr=10.242.64.33"
	for part := range strings.SplitSeq(raw, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "traddr":
			na.Traddr = kv[1]
		case "trsvcid":
			na.Trsvcid = kv[1]
		case "src_addr":
			na.SrcAddr = kv[1]
		}
	}

	return nil
}

// NewNVMeInitiator creates a new NVMe-oF initiator.
func NewNVMeInitiator() NVMeInitiator {
	return &nvmeInitiator{}
}

// LoadKernelModules ensures required kernel modules are loaded.
func (ni *nvmeInitiator) LoadKernelModules(ctx context.Context) error {
	module := "nvme_tcp"
	log.DebugLog(ctx, "Loading NVMe-oF kernel module: %s", module)

	err := kmod.Modprobe(ctx, module)
	if err != nil {
		return fmt.Errorf("failed to load kernel module %q: %w", module, err)
	}
	log.DebugLog(ctx, "NVMe-oF kernel module: %s, is loaded successfully", module)
	// verify nvme_fabrics is functional by checking its device node.
	// On immutable Operating Systems like Talos Linux (CONFIG_NVME_FABRICS=y), the module
	// is baked into the kernel and /sys/module/nvme_fabrics may not exist,
	// but /dev/nvme-fabrics is always created by the kernel on init if the
	// fabrics framework is operational.
	if _, err := os.Stat("/dev/nvme-fabrics"); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("nvme_fabrics is not functional, /dev/nvme-fabrics not found: %w", err)
		}

		return fmt.Errorf("nvme_fabrics is not functional, failed to stat /dev/nvme-fabrics: %w", err)
	}
	log.DebugLog(ctx, "NVMe-oF fabrics framework is functional with /dev/nvme-fabrics present")

	return nil
}

// ConnectSubsystem connects to an NVMe-oF subsystem.
func (ni *nvmeInitiator) ConnectSubsystem(ctx context.Context, req *ConnectRequest) (bool, error) {
	// Get existing subsystem connections once to avoid repeated nvme list-subsys calls
	var existingConnections nvmeHostConnections
	if req.HostNQN != "" {
		connections, err := listSubsystems(ctx)
		if err != nil {
			log.WarningLog(ctx, "Failed to list existing subsystems: %v (continuing anyway)", err)
		} else {
			existingConnections = connections
		}
	}
	// Try connecting to each address until one succeeds
	var success bool
	for _, listener := range req.Listeners {
		portStr := strconv.FormatUint(uint64(listener.Port), 10)

		// Check if already connected to this specific gateway
		if req.HostNQN != "" && existingConnections != nil {
			if existingConnections.hasPathToGateway(
				req.SubsystemNQN, req.HostNQN, listener.Address, portStr) {
				log.DebugLog(ctx, "Already connected to subsystem %s via %s:%s with HostNQN %s",
					req.SubsystemNQN, listener.Address, portStr, req.HostNQN)
				success = true

				continue
			}
		}

		log.DebugLog(ctx, "Connecting to NVMe-oF subsystem %s at %v:%s",
			req.SubsystemNQN, listener.Address, portStr)

		// Build nvme connect command for this address
		args := []string{
			"connect",
			"-t", req.Transport,
			"-n", req.SubsystemNQN,
			"-a", listener.Address,
			"-s", portStr,
			"-l", nvmeCtrlLossTmo,
		}

		// Add HostNQN only if specified
		if req.HostNQN != "" {
			args = append(args, "--hostnqn", req.HostNQN)
		}
		// if Host DH-CHAP key is provided, add it to the command
		if req.HostDhchapKey != "" {
			args = append(args, "--dhchap-secret", req.HostDhchapKey)
		}
		// if Subsystem DH-CHAP key is provided, add it to the command (for bi-directional auth)
		if req.SubsystemDhchapKey != "" {
			args = append(args, "--dhchap-ctrl-secret", req.SubsystemDhchapKey)
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

// DisconnectIfLastMount disconnects from ALL NVMe controllers for the given device path,
// but only if NO other mounted namespace is served by any of those controllers.
//
// In NVMe-oF multipath, all namespaces in a subsystem are accessible through all controllers.
// This means if any namespace is mounted, it's using ALL controllers. Therefore, we use an
// all-or-nothing approach: either disconnect all controllers or none.
//
// params:
// - devPath: the device path of the namespace being unstaged (e.g. /dev/nvme0n1)
// - mountedDevices: a map of currently MOUNTED device paths for quick lookup
//
// Algorithm:
//  1. Resolve the controllers that serve devPath.
//  2. For every OTHER mounted device, resolve its controllers and check for overlap.
//  3. If any other mounted device shares a controller, return early (don't disconnect anything).
//  4. Otherwise, disconnect ALL controllers of devPath.
//
// Note on device naming: with NVMe-oF native multipath the namespace block device is named
// after the subsystem head instance (e.g. /dev/nvme0n1), which differs from the fabric
// controller instance (e.g. /dev/nvme1). Comparing actual mounted devices via list-subsys
// avoids reconstructing device names from controller names + nsid, which would never match a
// head device and would wrongly disconnect a controller still in use by other volumes.
//
// Example: Devices /dev/nvme0n1 and /dev/nvme0n2 are both served by controller nvme1.
//
// Case 1 - Other namespace mounted:
//   - Unstaging /dev/nvme0n1 while /dev/nvme0n2 is still mounted.
//   - /dev/nvme0n2 is served by nvme1 -> Don't disconnect ANY controller.
//
// Case 2 - No other namespaces mounted:
//   - Unstaging /dev/nvme0n1 and no other mounted device is served by nvme1.
//   - Decision: No other namespaces using the controllers -> Disconnect nvme1.
func (ni *nvmeInitiator) DisconnectIfLastMount(ctx context.Context, devPath string,
	mountedDevices map[string]string,
) error {
	return disconnectIfLastMount(ctx, devPath, mountedDevices, getControllersForDevice, disconnectController)
}

// disconnectIfLastMount holds the decision logic for DisconnectIfLastMount with its system
// dependencies injected (controllersFor resolves a device's controllers, disconnect tears a
// controller down), so the full flow can be exercised without invoking the nvme binary.
func disconnectIfLastMount(ctx context.Context, devPath string,
	mountedDevices map[string]string,
	controllersFor func(context.Context, string) ([]string, error),
	disconnect func(context.Context, string) error,
) error {
	controllers, err := controllersFor(ctx, devPath)
	if err != nil {
		return fmt.Errorf("failed to get controllers for %s: %w", devPath, err)
	}
	if len(controllers) == 0 {
		// Device already disconnected, nothing to do (idempotent).
		return nil
	}

	ctrlSet := make(map[string]struct{}, len(controllers))
	for _, ctrl := range controllers {
		ctrlSet[ctrl] = struct{}{}
	}

	shared, err := hasSharedControllers(ctx, devPath, ctrlSet, mountedDevices, controllersFor)
	if err != nil {
		log.WarningLog(ctx, "failed to check controllers of other mounted devices for %s: %v (skipping disconnect)",
			devPath, err)

		return nil // Safe: don't disconnect if we can't verify
	}
	if shared {
		log.DebugLog(ctx, "controllers of %s still serve other mounted namespaces, skipping disconnect", devPath)

		return nil // EXIT early, don't disconnect ANYTHING
	}

	log.DebugLog(ctx, "No other mounted namespaces found, disconnecting all controllers")

	var disconnectErrors []string
	for _, ctrl := range controllers {
		log.DebugLog(ctx, "Disconnecting controller %s", ctrl)
		if err := disconnect(ctx, ctrl); err != nil {
			disconnectErrors = append(disconnectErrors, fmt.Sprintf("%s: %v", ctrl, err))

			continue
		}
		log.DebugLog(ctx, "Successfully disconnected controller %s", ctrl)
	}

	if len(disconnectErrors) > 0 {
		return fmt.Errorf("failed to disconnect some controllers: %s",
			strings.Join(disconnectErrors, "; "))
	}

	return nil
}

// disconnectController tears down a single NVMe controller via the nvme binary.
func disconnectController(ctx context.Context, controllerName string) error {
	_, _, err := util.ExecCommandWithTimeout(ctx, connectTimeout,
		"nvme", "disconnect", "-d", "/dev/"+controllerName)

	return err
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

// listSubsystems retrieves current NVMe subsystem connections.
func listSubsystems(ctx context.Context) (nvmeHostConnections, error) {
	stdout, _, err := util.ExecCommandWithTimeout(ctx, listSubsysTimeout, "nvme", "list-subsys", "-o", "json")
	if err != nil {
		return nil, err
	}

	var hosts nvmeHostConnections
	if err := json.Unmarshal([]byte(stdout), &hosts); err != nil {
		return nil, err
	}

	return hosts, nil
}

// hasPathToGateway checks if a path exists to the specified gateway.
func (nhc nvmeHostConnections) hasPathToGateway(subsystemNQN, hostNQN,
	gatewayIP, gatewayPort string,
) bool {
	for _, host := range nhc {
		if host.HostNQN != hostNQN {
			continue
		}

		for _, subsys := range host.Subsystems {
			if subsys.NQN != subsystemNQN {
				continue
			}

			// loop through paths to find matching path
			for _, path := range subsys.Paths {
				// Check if the path matches the gateway IP and port
				// and is in a usable state:
				// - "live": connection is active and working
				// - "connecting": kernel is actively trying to (re)connect
				//
				// The "connecting" state occurs when:
				// 1. Initial connection is being established
				// 2. Connection lost and kernel is retrying (ctrl_loss_tmo in effect)
				// 3. Subsystem was deleted/recreated on the gateway
				//
				// In all cases, the kernel's retry mechanism handles reconnection
				// for up to ctrl_loss_tmo seconds, so we should not attempt another
				// connection which would fail with "already connected" error.
				if path.Address.Traddr == gatewayIP &&
					path.Address.Trsvcid == gatewayPort &&
					(path.State == "live" ||
						path.State == "connecting") {
					return true
				}
			}
		}
	}

	return false
}

// ResolveListeners resolves listener IP addresses from hostnames and returns only valid listeners.
// Returns error only if all listeners fail to resolve.
func ResolveListeners(ctx context.Context, listeners []ListenerDetails) ([]ListenerDetails, error) {
	var resolveErrors []string
	var validListeners []ListenerDetails

	for i := range listeners {
		// if the address was empty, and the controller assigned it to default 0.0.0.0,
		// resolve the IP address from hostname for the node to connect to the subsystem
		if listeners[i].Address == "0.0.0.0" {
			addrs, err := ResolveIPAddress(listeners[i].Hostname)
			if err != nil {
				errMsg := fmt.Sprintf("listener %d (%s): %v", i, listeners[i].Hostname, err)
				log.WarningLog(ctx, "%s", errMsg)
				resolveErrors = append(resolveErrors, errMsg)

				continue // Skip this listener
			}
			listeners[i].Address = addrs
			log.DebugLog(ctx, "Resolved %s to %s", listeners[i].Hostname, listeners[i].Address)
		}

		// Add to valid listeners (either resolved or already had an address)
		validListeners = append(validListeners, listeners[i])
	}

	// If no listeners succeeded, return error
	if len(validListeners) == 0 {
		return nil, fmt.Errorf("failed to resolve any listener hostnames: %v", strings.Join(resolveErrors, "; "))
	}

	// If some failed, log warning but continue
	if len(resolveErrors) > 0 {
		log.WarningLog(ctx, "Some listeners failed to resolve (using %d valid listeners): %v",
			len(validListeners), strings.Join(resolveErrors, "; "))
	}

	log.DebugLog(ctx, "Successfully resolved %d listener(s)", len(validListeners))

	return validListeners, nil
}

// getControllersForDevice returns all controller names for a given namespace
// device (e.g. /dev/nvme0n1 -> ["nvme0", "nvme1"]).
// Uses nvme list-subsys <device> which is multipath-aware.
func getControllersForDevice(ctx context.Context, devPath string) ([]string, error) {
	stdout, _, err := util.ExecCommandWithTimeout(ctx, listSubsysTimeout,
		"nvme", "list-subsys", devPath, "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("list-subsys failed for %s: %w", devPath, err)
	}

	var hosts nvmeHostConnections
	if err := json.Unmarshal([]byte(stdout), &hosts); err != nil {
		log.DebugLog(ctx, "Failed to parse list-subsys output for %s: %v (output: %s)", devPath, err, stdout)

		return nil, fmt.Errorf("failed to parse list-subsys output: %w", err)
	}

	var controllers []string
	for _, host := range hosts {
		for _, subsys := range host.Subsystems {
			for _, path := range subsys.Paths {
				if path.Name != "" {
					controllers = append(controllers, path.Name)
				}
			}
		}
	}
	// This makes it idempotent - device may have been disconnected already
	if len(controllers) == 0 {
		log.DebugLog(ctx, "no controllers found for device %s (may be already disconnected)", devPath)
	}

	return controllers, nil
}

// hasSharedControllers reports whether any mounted device OTHER than currentDevPath is
// served by one of the controllers in ctrlSet (the controllers of the device being unstaged).
//
// It resolves the controllers of each other mounted device via controllersFor (injectable
// for testing) and checks for overlap with ctrlSet. This compares real mounted head devices
// against real controllers, so it works regardless of the head-vs-controller instance naming
// difference in NVMe-oF native multipath.
//
// mountedDevices is expected to hold currently mounted namespaces, so every other device must
// resolve to at least one controller. An empty controller list is therefore treated as a
// verification failure (returning an error), not as "no overlap" - the caller then fails safe
// and skips the disconnect rather than risk tearing down a controller still in use.
func hasSharedControllers(ctx context.Context, currentDevPath string,
	ctrlSet map[string]struct{}, mountedDevices map[string]string,
	controllersFor func(context.Context, string) ([]string, error),
) (bool, error) {
	for dev := range mountedDevices {
		// Skip the device we're about to unmount.
		if dev == currentDevPath {
			continue
		}

		ctrls, err := controllersFor(ctx, dev)
		if err != nil {
			return false, fmt.Errorf("failed to get controllers for %s: %w", dev, err)
		}
		if len(ctrls) == 0 {
			return false, fmt.Errorf("no controllers found for mounted device %s", dev)
		}

		for _, ctrl := range ctrls {
			if _, ok := ctrlSet[ctrl]; ok {
				log.DebugLog(ctx, "mounted device %s shares controller %s with %s",
					dev, ctrl, currentDevPath)

				return true, nil
			}
		}
	}

	return false, nil
}
