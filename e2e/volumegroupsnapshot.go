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

package e2e

import (
	"context"
	"crypto/sha256"
	"fmt"

	groupsnapapi "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
)

type cephFSVolumeGroupSnapshot struct {
	*volumeGroupSnapshotterBase
}

var _ VolumeGroupSnapshotter = &cephFSVolumeGroupSnapshot{}

func newCephFSVolumeGroupSnapshot(f *framework.Framework, namespace,
	storageClass string,
	blockPVC bool,
	timeout, totalPVCCount, additionalVGSnapshotCount int,
) (VolumeGroupSnapshotter, error) {
	base, err := newVolumeGroupSnapshotBase(f, namespace, storageClass, blockPVC,
		timeout, totalPVCCount, additionalVGSnapshotCount)
	if err != nil {
		return nil, fmt.Errorf("failed to create volumeGroupSnapshotterBase: %w", err)
	}

	return &cephFSVolumeGroupSnapshot{
		volumeGroupSnapshotterBase: base,
	}, nil
}

func (c *cephFSVolumeGroupSnapshot) TestVolumeGroupSnapshot() error {
	return c.volumeGroupSnapshotterBase.testVolumeGroupSnapshot(c)
}

func (c *cephFSVolumeGroupSnapshot) GetVolumeGroupSnapshotClass() (*groupsnapapi.VolumeGroupSnapshotClass, error) {
	vgscPath := fmt.Sprintf("%s/%s", cephFSExamplePath, "groupsnapshotclass.yaml")
	vgsc := &groupsnapapi.VolumeGroupSnapshotClass{}
	err := unmarshal(vgscPath, vgsc)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal VolumeGroupSnapshotClass: %w", err)
	}

	vgsc.Parameters["csi.storage.k8s.io/group-snapshotter-secret-namespace"] = cephCSINamespace
	vgsc.Parameters["csi.storage.k8s.io/group-snapshotter-secret-name"] = cephFSProvisionerSecretName
	vgsc.Parameters["fsName"] = fileSystemName

	fsID, err := getClusterID(c.framework)
	if err != nil {
		return nil, fmt.Errorf("failed to get clusterID: %w", err)
	}
	vgsc.Parameters["clusterID"] = fsID

	return vgsc, nil
}

func (c *cephFSVolumeGroupSnapshot) ValidateResourcesForCreate(vgs *groupsnapapi.VolumeGroupSnapshot) error {
	ctx := context.TODO()
	metadataPool, err := getCephFSMetadataPoolName(c.framework, fileSystemName)
	if err != nil {
		return fmt.Errorf("failed getting cephFS metadata pool name: %w", err)
	}

	vgsc, err := c.groupclient.VolumeGroupSnapshotContents().Get(
		ctx,
		*vgs.Status.BoundVolumeGroupSnapshotContentName,
		metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get VolumeGroupSnapshotContent: %w", err)
	}

	sourcePVCCount := len(vgsc.Status.VolumeSnapshotHandlePairList)
	// we are creating clones for each source PVC
	clonePVCCount := len(vgsc.Status.VolumeSnapshotHandlePairList)
	totalPVCCount := sourcePVCCount + clonePVCCount
	validateSubvolumeCount(c.framework, totalPVCCount, fileSystemName, subvolumegroup)

	// we are creating 1 snapshot for each source PVC, validate the snapshot count
	for _, snapshot := range vgsc.Status.VolumeSnapshotHandlePairList {
		volumeHandle := snapshot.VolumeHandle
		volumeSnapshotName := fmt.Sprintf("snapshot-%x", sha256.Sum256([]byte(
			string(vgsc.UID)+volumeHandle)))
		volumeSnapshot, err := c.snapClient.VolumeSnapshots(vgs.Namespace).Get(ctx, volumeSnapshotName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get VolumeSnapshot: %w", err)
		}
		pvcName := *volumeSnapshot.Spec.Source.PersistentVolumeClaimName
		pvc, err := c.framework.ClientSet.CoreV1().PersistentVolumeClaims(vgs.Namespace).Get(ctx,
			pvcName,
			metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get PVC: %w", err)
		}
		pv := pvc.Spec.VolumeName
		pvObj, err := c.framework.ClientSet.CoreV1().PersistentVolumes().Get(ctx, pv, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get PV: %w", err)
		}
		validateCephFSSnapshotCount(c.framework, 1, subvolumegroup, pvObj)
	}
	validateOmapCount(c.framework, totalPVCCount, cephfsType, metadataPool, volumesType)
	validateOmapCount(c.framework, sourcePVCCount, cephfsType, metadataPool, snapsType)
	validateOmapCount(c.framework, 1, cephfsType, metadataPool, groupSnapsType)

	return nil
}

func (c *cephFSVolumeGroupSnapshot) ValidateResourcesForDelete() error {
	metadataPool, err := getCephFSMetadataPoolName(c.framework, fileSystemName)
	if err != nil {
		return fmt.Errorf("failed getting cephFS metadata pool name: %w", err)
	}
	validateOmapCount(c.framework, 0, cephfsType, metadataPool, volumesType)
	validateOmapCount(c.framework, 0, cephfsType, metadataPool, snapsType)
	validateOmapCount(c.framework, 0, cephfsType, metadataPool, groupSnapsType)

	return nil
}

type rbdVolumeGroupSnapshot struct {
	*volumeGroupSnapshotterBase
}

var _ VolumeGroupSnapshotter = &rbdVolumeGroupSnapshot{}

func newRBDVolumeGroupSnapshot(f *framework.Framework, namespace,
	storageClass string,
	blockPVC bool,
	timeout, totalPVCCount, additionalVGSnapshotCount int,
) (VolumeGroupSnapshotter, error) {
	base, err := newVolumeGroupSnapshotBase(f, namespace, storageClass, blockPVC,
		timeout, totalPVCCount, additionalVGSnapshotCount)
	if err != nil {
		return nil, fmt.Errorf("failed to create volumeGroupSnapshotterBase: %w", err)
	}

	return &rbdVolumeGroupSnapshot{
		volumeGroupSnapshotterBase: base,
	}, nil
}

func (rvgs *rbdVolumeGroupSnapshot) TestVolumeGroupSnapshot() error {
	return rvgs.volumeGroupSnapshotterBase.testVolumeGroupSnapshot(rvgs)
}

func (rvgs *rbdVolumeGroupSnapshot) GetVolumeGroupSnapshotClass() (*groupsnapapi.VolumeGroupSnapshotClass, error) {
	vgscPath := fmt.Sprintf("%s/%s", rbdExamplePath, "groupsnapshotclass.yaml")
	vgsc := &groupsnapapi.VolumeGroupSnapshotClass{}
	err := unmarshal(vgscPath, vgsc)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal VolumeGroupSnapshotClass: %w", err)
	}

	vgsc.Parameters["csi.storage.k8s.io/group-snapshotter-secret-namespace"] = cephCSINamespace
	vgsc.Parameters["csi.storage.k8s.io/group-snapshotter-secret-name"] = rbdProvisionerSecretName
	vgsc.Parameters["pool"] = defaultRBDPool

	fsID, err := getClusterID(rvgs.framework)
	if err != nil {
		return nil, fmt.Errorf("failed to get clusterID: %w", err)
	}
	vgsc.Parameters["clusterID"] = fsID

	return vgsc, nil
}

func (rvgs *rbdVolumeGroupSnapshot) ValidateResourcesForCreate(vgs *groupsnapapi.VolumeGroupSnapshot) error {
	vgsc, err := rvgs.groupclient.VolumeGroupSnapshotContents().Get(
		context.TODO(),
		*vgs.Status.BoundVolumeGroupSnapshotContentName,
		metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get VolumeGroupSnapshotContent: %w", err)
	}

	sourcePVCCount := len(vgsc.Status.VolumeSnapshotHandlePairList)
	clonePVCCount := len(vgsc.Status.VolumeSnapshotHandlePairList)
	totalPVCCount := sourcePVCCount + clonePVCCount

	validateOmapCount(rvgs.framework, totalPVCCount, rbdType, defaultRBDPool, volumesType)

	return nil
}

func (rvgs *rbdVolumeGroupSnapshot) ValidateResourcesForDelete() error {
	validateOmapCount(rvgs.framework, 0, rbdType, defaultRBDPool, volumesType)
	validateOmapCount(rvgs.framework, 0, rbdType, defaultRBDPool, snapsType)
	validateOmapCount(rvgs.framework, 0, rbdType, defaultRBDPool, groupSnapsType)
	validateRBDImageCount(rvgs.framework, 0, defaultRBDPool)

	err := waitToRemoveImagesFromTrash(rvgs.framework, defaultRBDPool, deployTimeout)
	if err != nil {
		return err
	}

	return nil
}
