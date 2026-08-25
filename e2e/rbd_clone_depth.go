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

package e2e

import (
	"fmt"

	"k8s.io/kubernetes/test/e2e/framework"
)

func validateCloneDepthFlattenWithTrashedParents(f *framework.Framework) {
	err := createRBDSnapshotClass(f)
	if err != nil {
		logAndFail("failed to create storageclass: %v", err)
	}
	defer func() {
		err = deleteRBDSnapshotClass()
		if err != nil {
			logAndFail("failed to delete VolumeSnapshotClass: %v", err)
		}
	}()

	pvc, err := loadPVC(pvcPath)
	if err != nil {
		logAndFail("failed to load PVC: %v", err)
	}
	pvc.Namespace = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		logAndFail("failed to create PVC: %v", err)
	}

	sourceImageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		logAndFail("failed to get ImageInfo from pvc: %v", err)
	}

	snap := getSnapshot(snapshotPath)
	snap.Name = fmt.Sprintf("%s-trash-depth-0", snap.Name)
	snap.Namespace = f.UniqueName
	snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
	err = createSnapshot(&snap, deployTimeout)
	if err != nil {
		logAndFail("failed to create snapshot: %v", err)
	}

	snapImageName, err := getRBDSnapshotImageName(snap.Namespace, snap.Name)
	if err != nil {
		logAndFail("failed to get snapshot backing image name: %v", err)
	}

	// Restore the first snapshot before deleting it. The restored PVC keeps the
	// first snapshot backing image in its parent chain.
	pvcClone, err := loadPVC(pvcClonePath)
	if err != nil {
		logAndFail("failed to load PVC: %v", err)
	}
	pvcClone.Name = fmt.Sprintf("%s-trash-depth-0", pvcClone.Name)
	pvcClone.Namespace = f.UniqueName
	pvcClone.Spec.DataSource.Name = snap.Name
	err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
	if err != nil {
		logAndFail("failed to create PVC from snapshot: %v", err)
	}

	// Move both the original source image and first snapshot backing image into
	// trash. The next snapshot restore must still count through these parents.
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		logAndFail("failed to delete source PVC: %v", err)
	}
	err = waitForRBDImageInTrash(f, defaultRBDPool, sourceImageData.imageName, deployTimeout)
	if err != nil {
		logAndFail("failed to validate source image in trash: %v", err)
	}
	err = deleteSnapshot(&snap, deployTimeout)
	if err != nil {
		logAndFail("failed to delete snapshot: %v", err)
	}
	err = waitForRBDImageInTrash(f, defaultRBDPool, snapImageName, deployTimeout)
	if err != nil {
		logAndFail("failed to validate snapshot backing image in trash: %v", err)
	}

	snap2 := getSnapshot(snapshotPath)
	snap2.Name = fmt.Sprintf("%s-trash-depth-1", snap2.Name)
	snap2.Namespace = f.UniqueName
	snap2.Spec.Source.PersistentVolumeClaimName = &pvcClone.Name
	err = createSnapshot(&snap2, deployTimeout)
	if err != nil {
		logAndFail("failed to create second snapshot: %v", err)
	}

	snap2ImageName, err := getRBDSnapshotImageName(snap2.Namespace, snap2.Name)
	if err != nil {
		logAndFail("failed to get second snapshot backing image name: %v", err)
	}

	// Restoring the second snapshot reaches the soft clone-depth limit only if
	// clone depth is counted through the parents that are already in trash.
	pvcClone2, err := loadPVC(pvcClonePath)
	if err != nil {
		logAndFail("failed to load second PVC clone: %v", err)
	}
	pvcClone2.Name = fmt.Sprintf("%s-trash-depth-1", pvcClone2.Name)
	pvcClone2.Namespace = f.UniqueName
	pvcClone2.Spec.DataSource.Name = snap2.Name
	err = createPVCAndvalidatePV(f.ClientSet, pvcClone2, deployTimeout)
	if err != nil {
		logAndFail("failed to create PVC from second snapshot: %v", err)
	}

	err = waitForRBDImageFlattened(f, defaultRBDPool, snap2ImageName, deployTimeout)
	if err != nil {
		logAndFail("failed to validate second snapshot backing image flatten: %v", err)
	}

	err = deleteSnapshot(&snap2, deployTimeout)
	if err != nil {
		logAndFail("failed to delete second snapshot: %v", err)
	}
	err = deletePVCAndValidatePV(f.ClientSet, pvcClone2, deployTimeout)
	if err != nil {
		logAndFail("failed to delete second PVC clone: %v", err)
	}
	err = deletePVCAndValidatePV(f.ClientSet, pvcClone, deployTimeout)
	if err != nil {
		logAndFail("failed to delete PVC clone: %v", err)
	}
	validateRBDImageCount(f, 0, defaultRBDPool)
	validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
	validateOmapCount(f, 0, rbdType, defaultRBDPool, snapsType)
}
