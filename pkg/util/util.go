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
	"math"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/util/mount"
)

// remove this once kubernetes v1.14.0 release is done
// https://github.com/kubernetes/cloud-provider/blob/master/volume/helpers/rounding.go
const (
	// MiB - MebiByte size
	MiB = 1024 * 1024
	GiB = MiB * 1024
)

// RoundOffVolSize rounds up given quantity upto chunks of MiB/GiB
func RoundOffVolSize(size int64) int64 {
	size = RoundOffBytes(size)
	// convert size back to MiB for rbd CLI
	return size / MiB
}

// RoundOffBytes converts roundoff the size
// 1.1MiB will be round off to 2MiB same for GiB
// size less than 1MiB will be round off to 1MiB
func RoundOffBytes(bytes int64) int64 {
	var num int64
	floatBytes := float64(bytes)
	// round off the value if its in decimal
	if floatBytes < GiB {
		num = int64(math.Ceil(floatBytes / MiB))
		num *= MiB
	} else {
		num = int64(math.Ceil(floatBytes / GiB))
		num *= GiB
	}
	return num
}

// variables which will be set during the build time
var (
	// GitCommit tell the latest git commit image is built from
	GitCommit string
	// DriverVersion which will be driver version
	DriverVersion string
)

// Config holds the parameters list which can be configured
type Config struct {
	Vtype           string // driver type [rbd|cephfs|liveness]
	Endpoint        string // CSI endpoint
	DriverName      string // name of the driver
	NodeID          string // node id
	InstanceID      string // unique ID distinguishing this instance of Ceph CSI
	MetadataStorage string // metadata persistence method [node|k8s_configmap]
	PluginPath      string // location of cephcsi plugin

	// cephfs related flags
	MountCacheDir string // mount info cache save dir

	// metrics related flags
	MetricsPath       string        // path of prometheus endpoint where metrics will be available
	HistogramOption   string        // Histogram option for grpc metrics, should be comma separated value, ex:= "0.5,2,6" where start=0.5 factor=2, count=6
	MetricsIP         string        // TCP port for liveness/ metrics requests
	PidLimit          int           // PID limit to configure through cgroups")
	MetricsPort       int           // TCP port for liveness/grpc metrics requests
	PollTime          time.Duration // time interval in seconds between each poll
	PoolTimeout       time.Duration // probe timeout in seconds
	EnableGRPCMetrics bool          // option to enable grpc metrics

	IsControllerServer bool // if set to true start provisoner server
	IsNodeServer       bool // if set to true start node server
	// rbd related flag
	Containerized bool // whether run as containerized

}

// CreatePersistanceStorage creates storage path and initializes new cache
func CreatePersistanceStorage(sPath, metaDataStore, pluginPath string) (CachePersister, error) {
	var err error
	if err = CreateMountPoint(path.Join(sPath, "controller")); err != nil {
		klog.Errorf("failed to create persistent storage for controller: %v", err)
		return nil, err
	}

	if err = CreateMountPoint(path.Join(sPath, "node")); err != nil {
		klog.Errorf("failed to create persistent storage for node: %v", err)
		return nil, err
	}

	cp, err := NewCachePersister(metaDataStore, pluginPath)
	if err != nil {
		klog.Errorf("failed to define cache persistence method: %v", err)
		return nil, err
	}
	return cp, err
}

// ValidateDriverName validates the driver name
func ValidateDriverName(driverName string) error {
	if driverName == "" {
		return errors.New("driver name is empty")
	}

	if len(driverName) > 63 {
		return errors.New("driver name length should be less than 63 chars")
	}
	var err error
	for _, msg := range validation.IsDNS1123Subdomain(strings.ToLower(driverName)) {
		if err == nil {
			err = errors.New(msg)
			continue
		}
		err = errors.Wrap(err, msg)
	}
	return err
}

// GenerateVolID generates a volume ID based on passed in parameters and version, to be returned
// to the CO system
func GenerateVolID(ctx context.Context, monitors string, cr *Credentials, pool, clusterID, objUUID string, volIDVersion uint16) (string, error) {
	poolID, err := GetPoolID(ctx, monitors, cr, pool)
	if err != nil {
		return "", err
	}

	// generate the volume ID to return to the CO system
	vi := CSIIdentifier{
		LocationID:      poolID,
		EncodingVersion: volIDVersion,
		ClusterID:       clusterID,
		ObjectUUID:      objUUID,
	}

	volID, err := vi.ComposeCSIID()

	return volID, err
}

// CreateMountPoint creates the directory with given path
func CreateMountPoint(mountPath string) error {
	return os.MkdirAll(mountPath, 0750)
}

// checkDirExists checks directory  exists or not
func checkDirExists(p string) bool {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return false
	}
	return true
}

// IsMountPoint checks if the given path is mountpoint or not
func IsMountPoint(p string) (bool, error) {
	dummyMount := mount.New("")
	notMnt, err := dummyMount.IsLikelyNotMountPoint(p)
	if err != nil {
		return false, status.Error(codes.Internal, err.Error())
	}

	return !notMnt, nil
}

// Mount mounts the source to target path
func Mount(source, target, fstype string, options []string) error {
	dummyMount := mount.New("")
	return dummyMount.Mount(source, target, fstype, options)
}
