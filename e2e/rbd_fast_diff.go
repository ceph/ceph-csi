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
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"
)

// trashedParentReconcileTime is the time given to the PV reconciler to repair
// the trashed parent after it repaired the other parent. The PVs are
// reconciled one at a time in no fixed order.
const trashedParentReconcileTime = 30 * time.Second

var (
	// fastDiffFeatures are the image features required for efficient
	// DiffIterate.
	fastDiffFeatures = []string{"exclusive-lock", "object-map", "fast-diff"}
	// fastDiffInvalidFlags mark the object map of an image as unusable.
	fastDiffInvalidFlags = []string{"object map invalid", "fast diff invalid"}
)

// rbdImageParent is the parent of a cloned image as reported by `rbd info`.
type rbdImageParent struct {
	Image string `json:"image"`
	ID    string `json:"id"`
	Trash bool   `json:"trash"`
}

// rbdInfo is the part of `rbd info` used to validate fast-diff features.
type rbdInfo struct {
	Name     string          `json:"name"`
	ID       string          `json:"id"`
	Features []string        `json:"features"`
	Flags    []string        `json:"flags"`
	Parent   *rbdImageParent `json:"parent"`
}

// getRBDInfo returns `rbd info` of the image.
func getRBDInfo(f *framework.Framework, pool, image string) (rbdInfo, error) {
	var info rbdInfo

	stdOut, err := getImageInfo(f, image, pool)
	if err != nil {
		return info, err
	}
	err = json.Unmarshal([]byte(stdOut), &info)
	if err != nil {
		return info, fmt.Errorf("failed to parse rbd info of %s: %w ; output: %s", image, err, stdOut)
	}

	return info, nil
}

// fastDiffValid returns an error when the image is missing any of the
// fast-diff features, or its object map is flagged invalid.
func (i rbdInfo) fastDiffValid() error {
	for _, feature := range fastDiffFeatures {
		if !slices.Contains(i.Features, feature) {
			return fmt.Errorf("image %s is missing feature %s, got features: %v", i.Name, feature, i.Features)
		}
	}
	for _, flag := range fastDiffInvalidFlags {
		if slices.Contains(i.Flags, flag) {
			return fmt.Errorf("image %s has flag %q", i.Name, flag)
		}
	}

	return nil
}

// hasFastDiffFeature returns true when the image has any of the fast-diff
// features.
func (i rbdInfo) hasFastDiffFeature() bool {
	return slices.ContainsFunc(fastDiffFeatures, func(feature string) bool {
		return slices.Contains(i.Features, feature)
	})
}

// waitForFastDiffEnabled waits until the image has the fast-diff features and
// a valid object map.
func waitForFastDiffEnabled(f *framework.Framework, pool, image string, t int) error {
	var errReason error
	timeout := time.Duration(t) * time.Minute
	err := wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(_ context.Context) (bool, error) {
		info, err := getRBDInfo(f, pool, image)
		if err != nil {
			return false, err
		}
		errReason = info.fastDiffValid()
		if errReason != nil {
			framework.Logf("fast-diff not yet enabled: %v", errReason)

			return false, nil
		}

		return true, nil
	})
	if wait.Interrupted(err) {
		err = errReason
	}

	return err
}

// restoreRBDImageFromTrash restores the image with the given ID from the trash.
func restoreRBDImageFromTrash(f *framework.Framework, pool, imageID string) error {
	cmd := fmt.Sprintf("rbd trash restore %s %s", rbdOptions(pool), imageID)
	_, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return fmt.Errorf("failed to restore image %s from trash: %w ; stderr: %s", imageID, err, stdErr)
	}

	return nil
}

// removeRBDImage removes the image, retrying while snapshots or clones of it
// are still being removed.
func removeRBDImage(f *framework.Framework, pool, image string, t int) error {
	var errReason error
	cmd := fmt.Sprintf("rbd rm --no-progress %s %s", rbdOptions(pool), image)
	timeout := time.Duration(t) * time.Minute
	err := wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(_ context.Context) (bool, error) {
		_, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
		if err != nil {
			errReason = fmt.Errorf("failed to remove image %s: %w ; stderr: %s", image, err, stdErr)
			framework.Logf("%v", errReason)

			return false, nil
		}

		return true, nil
	})
	if wait.Interrupted(err) {
		err = errReason
	}

	return err
}

// validateParentFastDiffRepair checks that the PV reconciler enables the
// fast-diff features on the intermediate parent image of a PVC-PVC clone (the
// temporary clone image) and of a restored image (the snapshot backing
// image, in the trash after the VolumeSnapshot is deleted). The volumes are
// created without the features while the omap generator, which runs the PV
// reconciler, is disabled, like volumes created by an older release.
func validateParentFastDiffRepair(f *framework.Framework, op *OperatorDeployment) {
	scName := "csi-rbd-sc-no-fast-diff"
	err := createRBDStorageClass(f.ClientSet, f, scName, nil,
		map[string]string{"imageFeatures": "layering"}, deletePolicy)
	if err != nil {
		logAndFail("failed to create StorageClass %s: %v", scName, err)
	}
	defer func() {
		err = f.ClientSet.StorageV1().StorageClasses().Delete(context.TODO(), scName, metav1.DeleteOptions{})
		if err != nil {
			logAndFail("failed to delete StorageClass %s: %v", scName, err)
		}
	}()
	err = createRBDSnapshotClass(f)
	if err != nil {
		logAndFail("failed to create VolumeSnapshotClass: %v", err)
	}
	defer func() {
		err = deleteRBDSnapshotClass()
		if err != nil {
			logAndFail("failed to delete VolumeSnapshotClass: %v", err)
		}
	}()

	// Registered before disabling, so a partially applied disable is undone too.
	omapEnabled := false
	defer func() {
		if !omapEnabled {
			err = op.setGenerateOMapInfo(true)
			if err != nil {
				logAndFail("failed to enable the omap generator: %v", err)
			}
		}
	}()
	err = op.setGenerateOMapInfo(false)
	if err != nil {
		logAndFail("failed to disable the omap generator: %v", err)
	}

	// newApp returns a pod that mounts the PVC, and the options to select it.
	newApp := func(pvcName string) (*v1.Pod, *metav1.ListOptions) {
		app, lErr := loadApp(appPath)
		if lErr != nil {
			logAndFail("failed to load application: %v", lErr)
		}
		app.Name = pvcName
		app.Namespace = f.UniqueName
		app.Labels = map[string]string{"app": pvcName}
		app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcName

		return app, &metav1.ListOptions{LabelSelector: "app=" + pvcName}
	}
	// volumeInfo returns `rbd info` of the image of the PVC.
	volumeInfo := func(pvc *v1.PersistentVolumeClaim) rbdInfo {
		imageData, lErr := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
		if lErr != nil {
			logAndFail("failed to get image info from PVC %s: %v", pvc.Name, lErr)
		}
		info, lErr := getRBDInfo(f, defaultRBDPool, imageData.imageName)
		if lErr != nil {
			logAndFail("failed to get rbd info of PVC %s: %v", pvc.Name, lErr)
		}
		if info.Parent == nil {
			logAndFail("image %s of PVC %s has no parent", info.Name, pvc.Name)
		}

		return info
	}
	// parentInfo returns `rbd info` of the parent of the volume, and validates
	// that it is the expected image and does not have the fast-diff features.
	parentInfo := func(volume rbdInfo, wantParent string) rbdInfo {
		if volume.Parent.Image != wantParent {
			logAndFail("parent of image %s is %s, expected %s", volume.Name, volume.Parent.Image, wantParent)
		}
		info, lErr := getRBDInfo(f, defaultRBDPool, wantParent)
		if lErr != nil {
			logAndFail("failed to get rbd info of parent %s: %v", wantParent, lErr)
		}
		if info.hasFastDiffFeature() {
			logAndFail("parent %s should not have any of %v, got features: %v",
				wantParent, fastDiffFeatures, info.Features)
		}

		return info
	}

	By("creating PVCs without fast-diff features while the omap generator is disabled")
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		logAndFail("failed to load PVC: %v", err)
	}
	pvc.Namespace = f.UniqueName
	pvc.Spec.StorageClassName = &scName
	app, optApp := newApp(pvc.Name)
	err = createPVCAndApp("", f, pvc, app, deployTimeout)
	if err != nil {
		logAndFail("failed to create PVC and application: %v", err)
	}
	filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/data"
	_, stdErr, err := execCommandInPod(f,
		fmt.Sprintf("dd if=/dev/urandom of=%s bs=1M count=16 conv=fsync status=none", filePath),
		app.Namespace, optApp)
	if err != nil || stdErr != "" {
		logAndFail("failed to write data on PVC %s: err %v, stderr %q", pvc.Name, err, stdErr)
	}
	sourceSum, err := calculateSHA512sum(f, app, filePath, optApp)
	if err != nil {
		logAndFail("failed to calculate checksum on PVC %s: %v", pvc.Name, err)
	}

	pvcSmartClone, err := loadPVC(pvcSmartClonePath)
	if err != nil {
		logAndFail("failed to load PVC: %v", err)
	}
	pvcSmartClone.Namespace = f.UniqueName
	pvcSmartClone.Spec.StorageClassName = &scName
	pvcSmartClone.Spec.DataSource.Name = pvc.Name
	err = createPVCAndvalidatePV(f.ClientSet, pvcSmartClone, deployTimeout)
	if err != nil {
		logAndFail("failed to create PVC-PVC clone: %v", err)
	}
	cloneVolume := volumeInfo(pvcSmartClone)
	tempParent := parentInfo(cloneVolume, cloneVolume.Name+"-temp")

	snap := getSnapshot(snapshotPath)
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
	pvcClone, err := loadPVC(pvcClonePath)
	if err != nil {
		logAndFail("failed to load PVC: %v", err)
	}
	pvcClone.Namespace = f.UniqueName
	pvcClone.Spec.StorageClassName = &scName
	pvcClone.Spec.DataSource.Name = snap.Name
	err = createPVCAndvalidatePV(f.ClientSet, pvcClone, deployTimeout)
	if err != nil {
		logAndFail("failed to restore snapshot: %v", err)
	}
	restoreVolume := volumeInfo(pvcClone)
	snapParent := parentInfo(restoreVolume, snapImageName)

	By("deleting the VolumeSnapshot to move the snapshot backing image to the trash")
	err = deleteSnapshot(&snap, deployTimeout)
	if err != nil {
		logAndFail("failed to delete snapshot: %v", err)
	}
	err = waitForRBDImageInTrash(f, defaultRBDPool, snapImageName, deployTimeout)
	if err != nil {
		logAndFail("failed to validate snapshot backing image in trash: %v", err)
	}

	By("enabling the omap generator to reconcile the PVs")
	err = op.setGenerateOMapInfo(true)
	if err != nil {
		logAndFail("failed to enable the omap generator: %v", err)
	}
	omapEnabled = true

	By("verifying the fast-diff features of the temporary clone image")
	err = waitForFastDiffEnabled(f, defaultRBDPool, tempParent.Name, deployTimeout)
	if err != nil {
		logAndFail("fast-diff features not enabled on parent of PVC %s: %v", pvcSmartClone.Name, err)
	}

	By("verifying the fast-diff features of the snapshot backing image repaired in the trash")
	// The trashed image can not be inspected by name, restore it after the
	// reconciler had the time to repair it while it was in the trash.
	time.Sleep(trashedParentReconcileTime)
	err = restoreRBDImageFromTrash(f, defaultRBDPool, snapParent.ID)
	if err != nil {
		logAndFail("failed to restore parent of PVC %s: %v", pvcClone.Name, err)
	}
	info, err := getRBDInfo(f, defaultRBDPool, snapParent.Name)
	if err != nil {
		logAndFail("failed to get rbd info of parent %s: %v", snapParent.Name, err)
	}
	err = info.fastDiffValid()
	if err != nil {
		logAndFail("fast-diff features not enabled on parent of PVC %s while in trash: %v", pvcClone.Name, err)
	}

	By("verifying the volumes are unchanged and the data read through the parents")
	for _, clone := range []struct {
		pvc *v1.PersistentVolumeClaim
		old rbdInfo
	}{{pvcSmartClone, cloneVolume}, {pvcClone, restoreVolume}} {
		volume := volumeInfo(clone.pvc)
		if volume.ID != clone.old.ID || !slices.Equal(volume.Features, clone.old.Features) ||
			volume.Parent.ID != clone.old.Parent.ID {
			logAndFail("image of PVC %s changed from %+v to %+v", clone.pvc.Name, clone.old, volume)
		}

		// reads of unallocated clone objects go to the parent, so a wrong
		// rebuilt object map of the parent shows up as wrong data
		cloneApp, optClone := newApp(clone.pvc.Name)
		err = createApp(f.ClientSet, cloneApp, deployTimeout)
		if err != nil {
			logAndFail("failed to create application for PVC %s: %v", clone.pvc.Name, err)
		}
		sum, cErr := calculateSHA512sum(f, cloneApp, filePath, optClone)
		if cErr != nil {
			logAndFail("failed to calculate checksum on PVC %s: %v", clone.pvc.Name, cErr)
		}
		if sum != sourceSum {
			logAndFail("checksum on PVC %s does not match: got %s, expected %s", clone.pvc.Name, sum, sourceSum)
		}
		err = deletePVCAndApp("", f, clone.pvc, cloneApp)
		if err != nil {
			logAndFail("failed to delete PVC %s and application: %v", clone.pvc.Name, err)
		}
	}

	// The restored snapshot backing image is no longer in the trash, where
	// ceph-csi queued its removal, so remove it here.
	err = removeRBDImage(f, defaultRBDPool, snapParent.Name, deployTimeout)
	if err != nil {
		logAndFail("failed to remove parent %s: %v", snapParent.Name, err)
	}
	err = deletePVCAndApp("", f, pvc, app)
	if err != nil {
		logAndFail("failed to delete PVC and application: %v", err)
	}
	validateRBDImageCount(f, 0, defaultRBDPool)
	validateOmapCount(f, 0, rbdType, defaultRBDPool, volumesType)
	validateOmapCount(f, 0, rbdType, defaultRBDPool, snapsType)
}
