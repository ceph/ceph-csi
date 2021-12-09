/*
Copyright 2021 The Ceph-CSI Authors.

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
	"fmt"

	"github.com/ceph/ceph-csi/internal/journal"
)

const (
	// volIDVersion is the version number of volume ID encoding scheme.
	volIDVersion uint16 = 1
)

var (
	// CSIInstanceID is the instance ID that is unique to an instance of CSI, used when sharing
	// ceph clusters across CSI instances, to differentiate omap names per CSI instance.
	CSIInstanceID = "default"

	// volJournal and snapJournal are used to maintain RADOS based journals for CO generated
	// VolumeName to backing RBD images.
	volJournal  *journal.Config
	snapJournal *journal.Config
	// rbdHardMaxCloneDepth is the hard limit for maximum number of nested volume clones that are taken before flatten
	// occurs.
	rbdHardMaxCloneDepth uint

	// rbdSoftMaxCloneDepth is the soft limit for maximum number of nested volume clones that are taken before flatten
	// occurs.
	rbdSoftMaxCloneDepth              uint
	maxSnapshotsOnImage               uint
	minSnapshotsOnImageToStartFlatten uint
	skipForceFlatten                  bool

	// krbd features supported by the loaded driver.
	krbdFeatures uint
)

// SetGlobalInt provides a way for the rbd-driver to configure global variables
// in the rbd package.
//
// TODO: these global variables should be set in the ControllerService and
// NodeService where appropriate. Using global variables limits the ability to
// configure these options based on the Ceph cluster or StorageClass.
func SetGlobalInt(name string, value uint) {
	switch name {
	case "rbdHardMaxCloneDepth":
		rbdHardMaxCloneDepth = value
	case "rbdSoftMaxCloneDepth":
		rbdSoftMaxCloneDepth = value
	case "maxSnapshotsOnImage":
		maxSnapshotsOnImage = value
	case "minSnapshotsOnImageToStartFlatten":
		minSnapshotsOnImageToStartFlatten = value
	case "krbdFeatures":
		krbdFeatures = value
	default:
		panic(fmt.Sprintf("BUG: can not set unknown variable %q", name))
	}
}

// SetGlobalBool provides a way for the rbd-driver to configure global
// variables in the rbd package.
//
// TODO: these global variables should be set in the ControllerService and
// NodeService where appropriate. Using global variables limits the ability to
// configure these options based on the Ceph cluster or StorageClass.
func SetGlobalBool(name string, value bool) {
	switch name {
	case "skipForceFlatten":
		skipForceFlatten = value
	default:
		panic(fmt.Sprintf("BUG: can not set unknown variable %q", name))
	}
}

// InitJournals initializes the global journals that are used by the rbd
// package. This is called from the rbd-driver on startup.
//
// TODO: these global journals should be set in the ControllerService and
// NodeService where appropriate. Using global journals limits the ability to
// configure these options based on the Ceph cluster or StorageClass.
func InitJournals(instance string) {
	// Use passed in instance ID, if provided for omap suffix naming
	if instance != "" {
		CSIInstanceID = instance
	}

	volJournal = journal.NewCSIVolumeJournal(CSIInstanceID)
	snapJournal = journal.NewCSISnapshotJournal(CSIInstanceID)
}
