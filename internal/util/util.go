/*
Copyright 2019 The Ceph-CSI Authors.

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
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/cloud-provider/volume/helpers"
	mount "k8s.io/mount-utils"

	"github.com/ceph/ceph-csi/internal/util/k8s"
)

// Driver types to identify type of driver running.
const (
	RBDType        = "rbd"
	CephFsType     = "cephfs"
	NFSType        = "nfs"
	NVMeoFType     = "nvmeof"
	LivenessType   = "liveness"
	ControllerType = "controller"
)

// RoundOffVolSize rounds up given quantity up to chunks of MiB/GiB.
func RoundOffVolSize(size int64) int64 {
	size = RoundOffBytes(size)
	// convert size back to MiB for rbd CLI
	return size / helpers.MiB
}

// RoundOffBytes converts roundoff the size
// 1.1Mib will be round off to 2Mib same for GiB
// size less than 1MiB will be round off to 1MiB.
func RoundOffBytes(bytes int64) int64 {
	var num int64
	// round off the value if its in decimal
	if floatBytes := float64(bytes); floatBytes < helpers.GiB {
		num = int64(math.Ceil(floatBytes / helpers.MiB))
		num *= helpers.MiB
	} else {
		num = int64(math.Ceil(floatBytes / helpers.GiB))
		num *= helpers.GiB
	}

	return num
}

// RoundOffCephFSVolSize rounds up the bytes to 4MiB if the request is less
// than 4MiB or if its greater it rounds up to multiple of 4MiB.
func RoundOffCephFSVolSize(bytes int64) int64 {
	// Minimum supported size is 1MiB in CephCSI, if the request is <4MiB,
	// round off to 4MiB.
	if bytes < helpers.MiB {
		return 4 * helpers.MiB
	}

	bytesInFloat := float64(bytes) / helpers.MiB

	bytes = int64(math.Ceil(bytesInFloat/4) * 4)

	return RoundOffBytes(bytes * helpers.MiB)
}

// variables which will be set during the build time.
var (
	// GitCommit tell the latest git commit image is built from.
	GitCommit string
	// DriverVersion which will be driver version.
	DriverVersion string
)

// Config holds the parameters list which can be configured.
type Config struct {
	Vtype           string // driver type [rbd|cephfs|liveness|controller]
	Endpoint        string // CSI endpoint
	DriverName      string // name of the driver
	DriverNamespace string // namespace in which driver is deployed
	NodeID          string // node id
	InstanceID      string // unique ID distinguishing this instance of Ceph CSI
	PluginPath      string // location of cephcsi plugin
	StagingPath     string // location of cephcsi staging path
	DomainLabels    string // list of domain labels to read from the node
	// metrics related flags
	MetricsPath string // path of prometheus endpoint where metrics will be available
	MetricsIP   string // TCP port for liveness/ metrics requests

	// CSI-Addons endpoint
	CSIAddonsEndpoint string

	// Cluster name
	ClusterName string

	// mount option related flags
	KernelMountOptions string // Comma separated string of mount options accepted by cephfs kernel mounter
	FuseMountOptions   string // Comma separated string of mount options accepted by ceph-fuse mounter

	// RbdHardMaxCloneDepth is the hard limit for maximum number of nested volume clones that are taken before a flatten
	// occurs
	RbdHardMaxCloneDepth uint

	// RbdSoftMaxCloneDepth is the soft limit for maximum number of nested volume clones that are taken before a flatten
	// occurs
	RbdSoftMaxCloneDepth uint

	// MaxSnapshotsOnImage represents the maximum number of snapshots allowed
	// on rbd image without flattening, once the limit is reached cephcsi will
	// start flattening the older rbd images to allow more snapshots
	MaxSnapshotsOnImage uint

	// MinSnapshotsOnImage represents the soft limit for maximum number of
	// snapshots allowed on rbd image without flattening, once the soft limit is
	// reached cephcsi will start flattening the older rbd images.
	MinSnapshotsOnImage uint

	PidLimit    int           // PID limit to configure through cgroups")
	MetricsPort int           // TCP port for liveness/grpc metrics requests
	PollTime    time.Duration // time interval in seconds between each poll
	PoolTimeout time.Duration // probe timeout in seconds
	// Log interval for slow GRPC calls. Calls that outlive their context deadline
	// are considered slow.
	LogSlowOpInterval time.Duration

	AutoMaxProcs bool // configure GOMAXPROCS with automaxprocs

	EnableProfiling    bool // flag to enable profiling
	IsControllerServer bool // if set to true start provisioner server
	IsNodeServer       bool // if set to true start node server
	Version            bool // cephcsi version

	// SkipForceFlatten is set to false if the kernel supports mounting of
	// rbd image or the image chain has the deep-flatten feature.
	SkipForceFlatten bool

	// enable fencing of nodes during non-graceful shutdowns.
	EnableFencing bool

	// cephfs related flags
	ForceKernelCephFS    bool   // force to use the ceph kernel client even if the kernel is < 4.17
	RadosNamespaceCephFS string // RadosNamespace used to store CSI specific objects and keys
	SetMetadata          bool   // set metadata on the volume

	// Read affinity related options
	EnableReadAffinity  bool   // enable OSD read affinity.
	CrushLocationLabels string // list of CRUSH location labels to read from the node.
}

// ValidateDriverName validates the driver name.
func ValidateDriverName(driverName string) error {
	if driverName == "" {
		return errors.New("driver name is empty")
	}

	const reqDriverNameLen = 63
	if len(driverName) > reqDriverNameLen {
		return errors.New("driver name length should be less than 63 chars")
	}
	var err error
	for _, msg := range validation.IsDNS1123Subdomain(strings.ToLower(driverName)) {
		if err == nil {
			err = errors.New(msg)

			continue
		}
		err = fmt.Errorf("%s: %w", msg, err)
	}

	return err
}

// GenerateVolID generates a volume ID based on passed in parameters and version, to be returned
// to the CO system.
func GenerateVolID(
	ctx context.Context,
	monitors string,
	cr *Credentials,
	locationID int64,
	pool, clusterID, objUUID string,
) (string, error) {
	var err error

	if locationID == InvalidPoolID {
		locationID, err = GetPoolID(monitors, cr, pool)
		if err != nil {
			return "", err
		}
	}

	// generate the volume ID to return to the CO system
	vi := CSIIdentifier{
		LocationID: locationID,
		ClusterID:  clusterID,
		ObjectUUID: objUUID,
	}

	volID, err := vi.ComposeCSIID()

	return volID, err
}

// CreateMountPoint creates the directory with given path.
func CreateMountPoint(mountPath string) error {
	return os.MkdirAll(mountPath, 0o750)
}

// checkDirExists checks directory  exists or not.
func checkDirExists(p string) bool {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return false
	}

	return true
}

// IsCorruptedMountError checks if the given error is a result of a corrupted
// mountpoint.
func IsCorruptedMountError(err error) bool {
	return mount.IsCorruptedMnt(err)
}

// Mount mounts the source to target path.
func Mount(mounter mount.Interface, source, target, fstype string, options []string) error {
	return mounter.MountSensitiveWithoutSystemd(source, target, fstype, options, nil)
}

// MountOptionsAdd adds the `add` mount options to the `options` and returns a
// new string. In case `add` is already present in the `options`, `add` is not
// added again.
func MountOptionsAdd(options string, add ...string) string {
	opts := strings.Split(options, ",")
	newOpts := []string{}
	// clean original options from empty strings
	for _, opt := range opts {
		if opt != "" {
			newOpts = append(newOpts, opt)
		}
	}

	for _, opt := range add {
		if opt != "" && !slices.Contains(newOpts, opt) {
			newOpts = append(newOpts, opt)
		}
	}

	return strings.Join(newOpts, ",")
}

// CallStack returns the stack of the calls in the current goroutine. Useful
// for debugging or reporting errors. This is a friendly alternative to
// assert() or panic().
func CallStack() string {
	stack := make([]byte, 2048)
	_ = runtime.Stack(stack, false)

	return string(stack)
}

// GetVolumeContext filters out parameters that are not required in volume context.
func GetVolumeContext(parameters map[string]string) map[string]string {
	volumeContext := map[string]string{}

	// parameters that are not required in the volume context
	notRequiredParams := []string{
		topologyPoolsParam,
	}
	for k, v := range parameters {
		if !slices.Contains(notRequiredParams, k) {
			volumeContext[k] = v
		}
	}

	// remove kubernetes csi prefixed parameters.
	volumeContext = k8s.RemoveCSIPrefixedParameters(volumeContext)

	return volumeContext
}

// ParseClientIP extracts and normalizes an IP address from a address string.
// Handles both IPv4 and IPv6 addresses.
// Example: "10.244.0.1:0/2686266785" returns "10.244.0.1".
func ParseClientIP(addr string) (string, error) {
	versionPrefixRe := regexp.MustCompile(`(?:v\d+:)`)
	for hostPair := range strings.SplitSeq(addr, " ") {
		hostPair = versionPrefixRe.ReplaceAllString(hostPair, "")

		host, _, err := net.SplitHostPort(hostPair)
		if err != nil {
			// We might fail if there is no port specified, in that case continue
			// as if it's just an IP address and try to parse it.
			host = hostPair
		}

		if ip := net.ParseIP(host); ip != nil {
			return ip.String(), nil
		}
	}

	return "", fmt.Errorf("failed to extract IP address, incorrect format: %s", addr)
}

// GetControllerPublishSecretRef retrieves the controller publish secret from ceph-csi-config ConfigMap
// for a given clusterID. Fetches the secret from Kubernetes, and returns it as a map of key-value pairs.
func GetControllerPublishSecretRef(volumeId, driverType string) (string, string, error) {
	var (
		vi              CSIIdentifier
		secretName      string
		secretNamespace string
	)
	err := vi.DecomposeCSIID(volumeId)
	if err != nil {
		return secretName, secretNamespace, fmt.Errorf("failed to decode volume ID (%s): %w", volumeId, err)
	}

	secretName, secretNamespace, err = getControllerPublishSecretRef(vi.ClusterID, driverType)
	if err != nil && !errors.Is(err, ErrConfigNotFound) {
		return secretName, secretNamespace,
			fmt.Errorf("failed to get controller publish secret details from csi config file: %w", err)
	}

	if secretName != "" && secretNamespace != "" {
		return secretName, secretNamespace, nil
	}

	// Check clusterID mapping exists
	mapping, mErr := GetClusterMappingInfo(vi.ClusterID)
	if mErr != nil {
		return secretName, secretNamespace, mErr
	}
	if mapping != nil {
		for _, cm := range *mapping {
			for key, val := range cm.ClusterIDMapping {
				mappedClusterID := GetMappedID(key, val, vi.ClusterID)
				if mappedClusterID == "" {
					continue
				}

				secretName, secretNamespace, err := getControllerPublishSecretRef(mappedClusterID, driverType)
				if err != nil && !errors.Is(err, ErrConfigNotFound) {
					return secretName, secretNamespace,
						fmt.Errorf("failed to get controller publish secret details from csi config file: %w", err)
				}
				if secretName != "" && secretNamespace != "" {
					return secretName, secretNamespace, nil
				}
			}
		}
	}

	if secretName == "" || secretNamespace == "" {
		return secretName, secretNamespace, fmt.Errorf("controller publish secret name or namespace is empty"+
			" in csi config file for cluster %s", vi.ClusterID)
	}

	return secretName, secretNamespace, nil
}

func getControllerPublishSecretRef(clusterId, driverType string) (string, string, error) {
	var (
		err              error
		secretName       string
		secretNamespace  string
		getSecretRefFunc func(string, string) (string, string, error)
	)

	switch driverType {
	case RBDType:
		getSecretRefFunc = GetRBDControllerPublishSecretRef
	case CephFsType:
		getSecretRefFunc = GetCephFSControllerPublishSecretRef
	default:
		return secretName, secretNamespace, fmt.Errorf("unsupported driver type: %s", driverType)
	}

	secretName, secretNamespace, err = getSecretRefFunc(CsiConfigFile, clusterId)

	return secretName, secretNamespace, err
}
