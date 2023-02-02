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
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/ceph/ceph-csi/internal/util/log"

	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/cloud-provider/volume/helpers"
	mount "k8s.io/mount-utils"
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

	bytes /= helpers.MiB

	bytes = int64(math.Ceil(float64(bytes)/4) * 4)

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
	MetricsPath     string // path of prometheus endpoint where metrics will be available
	HistogramOption string // Histogram option for grpc metrics, should be comma separated value,
	// ex:= "0.5,2,6" where start=0.5 factor=2, count=6
	MetricsIP string // TCP port for liveness/ metrics requests

	// mount option related flags
	KernelMountOptions string // Comma separated string of mount options accepted by cephfs kernel mounter
	FuseMountOptions   string // Comma separated string of mount options accepted by ceph-fuse mounter

	PidLimit          int           // PID limit to configure through cgroups")
	MetricsPort       int           // TCP port for liveness/grpc metrics requests
	PollTime          time.Duration // time interval in seconds between each poll
	PoolTimeout       time.Duration // probe timeout in seconds
	EnableGRPCMetrics bool          // option to enable grpc metrics

	EnableProfiling    bool // flag to enable profiling
	IsControllerServer bool // if set to true start provisioner server
	IsNodeServer       bool // if set to true start node server
	Version            bool // cephcsi version

	// SkipForceFlatten is set to false if the kernel supports mounting of
	// rbd image or the image chain has the deep-flatten feature.
	SkipForceFlatten bool

	// cephfs related flags
	ForceKernelCephFS bool // force to use the ceph kernel client even if the kernel is < 4.17

	SetMetadata bool // set metadata on the volume

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

	// CSI-Addons endpoint
	CSIAddonsEndpoint string

	// Cluster name
	ClusterName string

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

// GetKernelVersion returns the version of the running Unix (like) system from the
// 'utsname' structs 'release' component.
func GetKernelVersion() (string, error) {
	utsname := unix.Utsname{}
	if err := unix.Uname(&utsname); err != nil {
		return "", err
	}

	return strings.TrimRight(string(utsname.Release[:]), "\x00"), nil
}

// KernelVersion holds kernel related information.
type KernelVersion struct {
	Version      int
	PatchLevel   int
	SubLevel     int
	ExtraVersion int    // prefix of the part after the first "-"
	Distribution string // component of full extraversion
	Backport     bool   // backport have a fixed version/patchlevel/sublevel
}

// parseKernelRelease parses a kernel release version string into:
// version, patch version, sub version and extra version.
func parseKernelRelease(release string) (int, int, int, int, error) {
	version := 0
	patchlevel := 0
	minVersions := 2

	extra := ""
	n, err := fmt.Sscanf(release, "%d.%d%s", &version, &patchlevel, &extra)
	if n < minVersions && err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to parse version and patchlevel from %s: %w", release, err)
	}

	sublevel := 0
	extraversion := 0
	if n > minVersions {
		n, err = fmt.Sscanf(extra, ".%d%s", &sublevel, &extra)
		if err != nil && n == 0 && len(extra) > 0 && extra[0] != '-' && extra[0] == '.' {
			return 0, 0, 0, 0, fmt.Errorf("failed to parse subversion from %s: %w", release, err)
		}

		extra = strings.TrimPrefix(extra, "-")
		// ignore errors, 1st component of extraversion does not need to be an int
		_, err = fmt.Sscanf(extra, "%d", &extraversion)
		if err != nil {
			// "go lint" wants err to be checked...
			extraversion = 0
		}
	}

	return version, patchlevel, sublevel, extraversion, nil
}

// CheckKernelSupport checks the running kernel and comparing it to known
// versions that have support for required features . Distributors of
// enterprise Linux have backport quota support to previous versions. This
// function checks if the running kernel is one of the versions that have the
// feature/fixes backport.
//
// `uname -r` (or Uname().Utsname.Release has a format like 1.2.3-rc.vendor
// This can be slit up in the following components: - version (1) - patchlevel
// (2) - sublevel (3) - optional, defaults to 0 - extraversion (rc) - optional,
// matching integers only - distribution (.vendor) - optional, match against
// whole `uname -r` string
//
// For matching multiple versions, the kernelSupport type contains a backport
// bool, which will cause matching
// version+patchlevel+sublevel+(>=extraversion)+(~distribution)
//
// In case the backport bool is false, a simple check for higher versions than
// version+patchlevel+sublevel is done.
func CheckKernelSupport(release string, supportedVersions []KernelVersion) bool {
	version, patchlevel, sublevel, extraversion, err := parseKernelRelease(release)
	if err != nil {
		log.ErrorLogMsg("%v", err)

		return false
	}

	// compare running kernel against known versions
	for _, kernel := range supportedVersions {
		if !kernel.Backport {
			// deal with the default case(s), find >= match for version, patchlevel, sublevel
			if version > kernel.Version || (version == kernel.Version && patchlevel > kernel.PatchLevel) ||
				(version == kernel.Version && patchlevel == kernel.PatchLevel && sublevel >= kernel.SubLevel) {
				return true
			}
		} else {
			// specific backport, match distribution initially
			if !strings.Contains(release, kernel.Distribution) {
				continue
			}

			// strict match version, patchlevel, sublevel, and >= match extraversion
			if version == kernel.Version && patchlevel == kernel.PatchLevel &&
				sublevel == kernel.SubLevel && extraversion >= kernel.ExtraVersion {
				return true
			}
		}
	}
	log.WarningLogMsg("kernel %s does not support required features", release)

	return false
}

// GenerateVolID generates a volume ID based on passed in parameters and version, to be returned
// to the CO system.
func GenerateVolID(
	ctx context.Context,
	monitors string,
	cr *Credentials,
	locationID int64,
	pool, clusterID, objUUID string,
	volIDVersion uint16,
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
		LocationID:      locationID,
		EncodingVersion: volIDVersion,
		ClusterID:       clusterID,
		ObjectUUID:      objUUID,
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

// IsMountPoint checks if the given path is mountpoint or not.
func IsMountPoint(mounter mount.Interface, p string) (bool, error) {
	notMnt, err := mounter.IsLikelyNotMountPoint(p)
	if err != nil {
		return false, err
	}

	return !notMnt, nil
}

// IsCorruptedMountError checks if the given error is a result of a corrupted
// mountpoint.
func IsCorruptedMountError(err error) bool {
	return mount.IsCorruptedMnt(err)
}

// ReadMountInfoForProc reads /proc/<PID>/mountpoint and marshals it into
// MountInfo structs.
func ReadMountInfoForProc(proc string) ([]mount.MountInfo, error) {
	return mount.ParseMountInfo(fmt.Sprintf("/proc/%s/mountinfo", proc))
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
		if opt != "" && !contains(newOpts, opt) {
			newOpts = append(newOpts, opt)
		}
	}

	return strings.Join(newOpts, ",")
}

func contains(s []string, key string) bool {
	for _, v := range s {
		if v == key {
			return true
		}
	}

	return false
}

// CallStack returns the stack of the calls in the current goroutine. Useful
// for debugging or reporting errors. This is a friendly alternative to
// assert() or panic().
func CallStack() string {
	stack := make([]byte, 2048)
	_ = runtime.Stack(stack, false)

	return string(stack)
}
