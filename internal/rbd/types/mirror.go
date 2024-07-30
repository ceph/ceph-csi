/*
Copyright 2024 The Ceph-CSI Authors.

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

package types

import (
	"context"
	"time"

	"github.com/ceph/ceph-csi/internal/util"

	librbd "github.com/ceph/go-ceph/rbd"
	"github.com/ceph/go-ceph/rbd/admin"
)

// FlattenMode is used to indicate the flatten mode for an RBD image.
type FlattenMode string

const (
	// FlattenModeNever indicates that the image should never be flattened.
	FlattenModeNever FlattenMode = "never"
	// FlattenModeForce indicates that the image with the parent must be flattened.
	FlattenModeForce FlattenMode = "force"
)

// Mirror is the interface for managing mirroring on an RBD image or a group.
type Mirror interface {
	// EnableMirroring enables mirroring on the resource with the specified mode.
	EnableMirroring(ctx context.Context, mode librbd.ImageMirrorMode) error
	// DisableMirroring disables mirroring on the resource with the option to force the operation
	DisableMirroring(ctx context.Context, force bool) error
	// Promote promotes the resource to primary status with the option to force the operation
	Promote(ctx context.Context, force bool) error
	// ForcePromote promotes the resource to primary status with a timeout
	ForcePromote(ctx context.Context, cr *util.Credentials) error
	// Demote demotes the resource to secondary status
	Demote(ctx context.Context) error
	// Resync resynchronizes the resource
	Resync(ctx context.Context) error
	// GetMirroringInfo returns the mirroring information of the resource
	GetMirroringInfo(ctx context.Context) (MirrorInfo, error)
	// GetMirroringInfo returns the mirroring information of the resource
	GetGlobalMirroringStatus(ctx context.Context) (GlobalStatus, error)
	// AddSnapshotScheduling adds a snapshot scheduling to the resource
	AddSnapshotScheduling(interval admin.Interval, startTime admin.StartTime) error
}

// MirrorImage is the interface for managing mirroring on an RBD image or group of images.
// This will be used to get the state of resource and it is primary or secondary.
type MirrorInfo interface {
	// IsPrimary returns true if the resource is primary
	IsPrimary() bool
	// GetState returns the state of the resource
	GetState() string
}

// GlobalStatus is the interface for fetching the global status of the mirroring.
// This will be used to get the status of the local site and remote site or all sites.
type GlobalStatus interface {
	MirrorInfo
	// GetLocalSiteStatus returns the local site status
	GetLocalSiteStatus() (SiteStatus, error)
	// GetAllSitesStatus returns the status of all sites
	GetAllSitesStatus() []SiteStatus
	// GetRemoteSiteStatus returns the status of the remote site
	GetRemoteSiteStatus(ctx context.Context) (SiteStatus, error)
}

// SiteStatus is the interface for fetching the status of a site.
// This will be used to get the status of the local site and remote site.
type SiteStatus interface {
	// GetMirrorUUID returns the mirror UUID
	GetMirrorUUID() string
	// IsUP returns true if the site is up
	IsUP() bool
	// GetState returns the state of the site
	GetState() string
	// GetDescription returns the description of the site
	GetDescription() string
	// GetLastUpdate returns the last update time
	GetLastUpdate() time.Time
}
