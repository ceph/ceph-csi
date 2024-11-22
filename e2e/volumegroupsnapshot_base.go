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
	"time"

	groupsnapapi "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1beta1"
	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	groupsnapclient "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/typed/volumegroupsnapshot/v1beta1"
	snapclient "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/typed/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"
)

// volumeGroupSnapshotter defines the common operations for handling
// volume group snapshots.
type volumeGroupSnapshotter interface {
	// CreateVolumeGroupSnapshotClass creates a volume group snapshot class.
	CreateVolumeGroupSnapshotClass(vgsc *groupsnapapi.VolumeGroupSnapshotClass) error
	// CreateVolumeGroupSnapshot creates a groupsnapshot with the specified name
	// namespace and volume group snapshot class.
	CreateVolumeGroupSnapshot(name,
		volumeGroupSnapshotClassName string,
		labels map[string]string) (*groupsnapapi.VolumeGroupSnapshot, error)
	// DeleteVolumeGroupSnapshot deletes the specified volume
	// group snapshot.
	DeleteVolumeGroupSnapshot(volumeGroupSnapshotName string) error
	// DeleteVolumeGroupSnapshotClass deletes the specified volume
	// group snapshot class.
	DeleteVolumeGroupSnapshotClass(snapshotClassName string) error
	// CreatePVCs creates PVCs with the specified namespace and labels.
	CreatePVCs(namespace string,
		labels map[string]string) ([]*v1.PersistentVolumeClaim, error)
	// DeletePVCs deletes the specified PVCs.
	DeletePVCs(pvcs []*v1.PersistentVolumeClaim) error
	// CreatePVCClones creates pvcs from all the snapshots in VolumeGroupSnapshot.
	CreatePVCClones(vgs *groupsnapapi.VolumeGroupSnapshot,
	) ([]*v1.PersistentVolumeClaim, error)
}

// VolumeGroupSnapshotter defines validation operations specific to each driver.
type VolumeGroupSnapshotter interface {
	// TestVolumeGroupSnapshot tests the volume group snapshot operations.
	TestVolumeGroupSnapshot() error
	// GetVolumeGroupSnapshotClass returns the volume group snapshot class.
	GetVolumeGroupSnapshotClass() (*groupsnapapi.VolumeGroupSnapshotClass, error)
	// ValidateResourcesForCreate validates the resources in the backend after
	// creating clones.
	ValidateResourcesForCreate(vgs *groupsnapapi.VolumeGroupSnapshot) error
	// ValidateSnapshotsDeleted checks if all resources are deleted in the
	// backend after all the resources are deleted.
	ValidateResourcesForDelete() error
}

type volumeGroupSnapshotterBase struct {
	timeout                   int
	framework                 *framework.Framework
	groupclient               *groupsnapclient.GroupsnapshotV1beta1Client
	snapClient                *snapclient.SnapshotV1Client
	storageClassName          string
	blockPVC                  bool
	totalPVCCount             int
	additionalVGSnapshotCount int
	namespace                 string
}

func newVolumeGroupSnapshotBase(f *framework.Framework, namespace,
	storageClass string,
	blockPVC bool,
	timeout, totalPVCCount, additionalVGSnapshotCount int,
) (*volumeGroupSnapshotterBase, error) {
	config, err := framework.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("error creating group snapshot client: %w", err)
	}
	c, err := groupsnapclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating group snapshot client: %w", err)
	}

	s, err := snapclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating snapshot client: %w", err)
	}

	return &volumeGroupSnapshotterBase{
		framework:                 f,
		groupclient:               c,
		snapClient:                s,
		namespace:                 namespace,
		storageClassName:          storageClass,
		blockPVC:                  blockPVC,
		timeout:                   timeout,
		totalPVCCount:             totalPVCCount,
		additionalVGSnapshotCount: additionalVGSnapshotCount,
	}, err
}

var _ volumeGroupSnapshotter = &volumeGroupSnapshotterBase{}

func (v *volumeGroupSnapshotterBase) CreatePVCs(namespace string,
	labels map[string]string,
) ([]*v1.PersistentVolumeClaim, error) {
	pvcs := make([]*v1.PersistentVolumeClaim, v.totalPVCCount)
	for i := range v.totalPVCCount {
		pvcs[i] = &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pvc-%d", i),
				Namespace: namespace,
			},
			Spec: v1.PersistentVolumeClaimSpec{
				Resources: v1.VolumeResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
				AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				StorageClassName: &v.storageClassName,
			},
		}
		if v.blockPVC {
			volumeMode := v1.PersistentVolumeBlock
			pvcs[i].Spec.VolumeMode = &volumeMode
		} else {
			volumeMode := v1.PersistentVolumeFilesystem
			pvcs[i].Spec.VolumeMode = &volumeMode
		}
		pvcs[i].Labels = labels
		err := createPVCAndvalidatePV(v.framework.ClientSet, pvcs[i], v.timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to create PVC: %w", err)
		}
	}

	return pvcs, nil
}

func (v *volumeGroupSnapshotterBase) DeletePVCs(pvcs []*v1.PersistentVolumeClaim) error {
	for _, pvc := range pvcs {
		err := deletePVCAndValidatePV(v.framework.ClientSet, pvc, v.timeout)
		if err != nil {
			return fmt.Errorf("failed to delete PVC: %w", err)
		}
	}

	return nil
}

func (v *volumeGroupSnapshotterBase) CreatePVCClones(
	vgs *groupsnapapi.VolumeGroupSnapshot,
) ([]*v1.PersistentVolumeClaim, error) {
	groupSnapshotContent, err := v.groupclient.VolumeGroupSnapshotContents().Get(
		context.TODO(),
		*vgs.Status.BoundVolumeGroupSnapshotContentName,
		metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get VolumeGroupSnapshotContent: %w", err)
	}
	namespace := vgs.Namespace
	ctx := context.TODO()
	pvcs := make([]*v1.PersistentVolumeClaim, len(groupSnapshotContent.Status.VolumeSnapshotHandlePairList))
	for i, snapshot := range groupSnapshotContent.Status.VolumeSnapshotHandlePairList {
		volumeHandle := snapshot.VolumeHandle
		volumeSnapshotName := fmt.Sprintf("snapshot-%x", sha256.Sum256([]byte(
			string(groupSnapshotContent.UID)+volumeHandle)))
		volumeSnapshot, err := v.snapClient.VolumeSnapshots(namespace).Get(ctx, volumeSnapshotName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get VolumeSnapshot: %w", err)
		}
		pvcName := *volumeSnapshot.Spec.Source.PersistentVolumeClaimName
		pvc, err := v.framework.ClientSet.CoreV1().PersistentVolumeClaims(namespace).Get(ctx,
			pvcName,
			metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get PVC: %w", err)
		}
		pvcs[i] = &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-clone-%d", pvc.Name, i),
				Namespace: pvc.Namespace,
			},
			Spec: *pvc.Spec.DeepCopy(),
		}

		apiGroup := snapapi.GroupName
		pvcs[i].Spec.DataSource = &v1.TypedLocalObjectReference{
			APIGroup: &apiGroup,
			Kind:     "VolumeSnapshot",
			Name:     volumeSnapshot.Name,
		}
		pvcs[i].Spec.StorageClassName = &v.storageClassName
		// cleanup the VolumeName as we are creating a new PVC
		pvcs[i].Spec.VolumeName = ""

		err = createPVCAndvalidatePV(v.framework.ClientSet, pvcs[i], v.timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to create PVC: %w", err)
		}
	}

	return pvcs, nil
}

func (v *volumeGroupSnapshotterBase) CreatePods(pvcs []*v1.PersistentVolumeClaim) ([]*v1.Pod, error) {
	pods := make([]*v1.Pod, len(pvcs))
	for i, p := range pvcs {
		pods[i] = &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: p.Namespace,
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    "container",
						Image:   "quay.io/centos/centos:latest",
						Command: []string{"/bin/sleep", "999999"},
					},
				},
			},
		}
		volName := "volume"
		if p.Spec.VolumeMode != nil && *p.Spec.VolumeMode == v1.PersistentVolumeBlock {
			pods[i].Spec.Containers[0].VolumeDevices = []v1.VolumeDevice{
				{
					Name:       volName,
					DevicePath: "/dev/xvda",
				},
			}
		} else {
			pods[i].Spec.Containers[0].VolumeMounts = []v1.VolumeMount{
				{
					Name:      volName,
					MountPath: "/mnt",
				},
			}
		}
		pods[i].Spec.Volumes = []v1.Volume{
			{
				Name: volName,
				VolumeSource: v1.VolumeSource{
					PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
						ClaimName: p.Name,
					},
				},
			},
		}
		err := createApp(v.framework.ClientSet, pods[i], v.timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to create pod: %w", err)
		}
	}

	return pods, nil
}

func (v *volumeGroupSnapshotterBase) DeletePods(pods []*v1.Pod) error {
	for _, pod := range pods {
		err := deletePod(pod.Name, pod.Namespace, v.framework.ClientSet, deployTimeout)
		if err != nil {
			return fmt.Errorf("failed to delete pod: %w", err)
		}
	}

	return nil
}

func (v *volumeGroupSnapshotterBase) CreateVolumeGroupSnapshotClass(
	groupSnapshotClass *groupsnapapi.VolumeGroupSnapshotClass,
) error {
	return wait.PollUntilContextTimeout(
		context.TODO(),
		poll,
		time.Duration(v.timeout)*time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			_, err := v.groupclient.VolumeGroupSnapshotClasses().Create(ctx, groupSnapshotClass, metav1.CreateOptions{})
			if err != nil {
				framework.Logf("error creating VolumeGroupSnapshotClass %q: %v", groupSnapshotClass.Name, err)
				if isRetryableAPIError(err) {
					return false, nil
				}

				return false, fmt.Errorf("failed to create VolumeGroupSnapshotClass %q: %w", groupSnapshotClass.Name, err)
			}

			return true, nil
		})
}

func (v *volumeGroupSnapshotterBase) CreateVolumeGroupSnapshot(name,
	volumeGroupSnapshotClassName string, labels map[string]string,
) (*groupsnapapi.VolumeGroupSnapshot, error) {
	namespace := v.namespace
	groupSnapshot := &groupsnapapi.VolumeGroupSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: groupsnapapi.VolumeGroupSnapshotSpec{
			Source: groupsnapapi.VolumeGroupSnapshotSource{
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
			},
			VolumeGroupSnapshotClassName: &volumeGroupSnapshotClassName,
		},
	}
	ctx := context.TODO()
	_, err := v.groupclient.VolumeGroupSnapshots(namespace).Create(ctx, groupSnapshot, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create VolumeGroupSnapshot %q: %w", name, err)
	}

	framework.Logf("VolumeGroupSnapshot with name %v created in %v namespace", name, namespace)

	timeout := time.Duration(v.timeout) * time.Minute
	start := time.Now()
	framework.Logf("waiting for %+v to be in ready state", groupSnapshot)

	err = wait.PollUntilContextTimeout(ctx, poll, timeout, true, func(ctx context.Context) (bool, error) {
		framework.Logf("waiting for VolumeGroupSnapshot %s (%d seconds elapsed)", name, int(time.Since(start).Seconds()))
		groupSnapshot, err = v.groupclient.VolumeGroupSnapshots(namespace).
			Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			framework.Logf("Error getting VolumeGroupSnapshot in namespace: '%s': %v", namespace, err)
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to get volumesnapshot: %w", err)
		}
		if groupSnapshot.Status == nil || groupSnapshot.Status.ReadyToUse == nil {
			return false, nil
		}

		if *groupSnapshot.Status.ReadyToUse {
			return true, nil
		}

		readyToUse := groupSnapshot.Status.ReadyToUse
		errMsg := ""
		if groupSnapshot.Status.Error != nil {
			errMsg = *groupSnapshot.Status.Error.Message
		}

		framework.Logf("current state of VolumeGroupSnapshot %s. ReadyToUse: %v, Error: %s", name, *readyToUse, errMsg)

		return false, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get VolumeGroupSnapshot %s: %w", name, err)
	}

	return groupSnapshot, nil
}

func (v *volumeGroupSnapshotterBase) DeleteVolumeGroupSnapshot(volumeGroupSnapshotName string) error {
	namespace := v.namespace
	ctx := context.TODO()
	err := v.groupclient.VolumeGroupSnapshots(namespace).Delete(
		ctx,
		volumeGroupSnapshotName,
		metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete VolumeGroupSnapshot: %w", err)
	}
	start := time.Now()
	framework.Logf("Waiting for VolumeGroupSnapshot %v to be deleted", volumeGroupSnapshotName)
	timeout := time.Duration(v.timeout) * time.Minute

	return wait.PollUntilContextTimeout(
		ctx,
		poll,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			_, err := v.groupclient.VolumeGroupSnapshots(namespace).Get(ctx, volumeGroupSnapshotName, metav1.GetOptions{})
			if err != nil {
				if isRetryableAPIError(err) {
					return false, nil
				}
				if apierrs.IsNotFound(err) {
					return true, nil
				}
				framework.Logf("%s VolumeGroupSnapshot to be deleted (%d seconds elapsed)",
					volumeGroupSnapshotName,
					int(time.Since(start).Seconds()))

				return false, fmt.Errorf("failed to get VolumeGroupSnapshot: %w", err)
			}

			return false, nil
		})
}

func (v *volumeGroupSnapshotterBase) DeleteVolumeGroupSnapshotClass(groupSnapshotClassName string) error {
	ctx := context.TODO()
	err := v.groupclient.VolumeGroupSnapshotClasses().Delete(
		ctx, groupSnapshotClassName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete VolumeGroupSnapshotClass: %w", err)
	}
	start := time.Now()
	framework.Logf("Waiting for VolumeGroupSnapshotClass %v to be deleted", groupSnapshotClassName)
	timeout := time.Duration(v.timeout) * time.Minute

	return wait.PollUntilContextTimeout(
		ctx,
		poll,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			_, err := v.groupclient.VolumeGroupSnapshotClasses().Get(ctx, groupSnapshotClassName, metav1.GetOptions{})
			if err != nil {
				if isRetryableAPIError(err) {
					return false, nil
				}
				if apierrs.IsNotFound(err) {
					return true, nil
				}
				framework.Logf("%s VolumeGroupSnapshotClass to be deleted (%d seconds elapsed)",
					groupSnapshotClassName,
					int(time.Since(start).Seconds()))

				return false, fmt.Errorf("failed to get VolumeGroupSnapshotClass: %w", err)
			}

			return false, nil
		})
}

func (v *volumeGroupSnapshotterBase) testVolumeGroupSnapshot(vol VolumeGroupSnapshotter) error {
	pvcLabels := map[string]string{"pvc": "vgsc"}
	pvcs, err := v.CreatePVCs(v.namespace, pvcLabels)
	if err != nil {
		return fmt.Errorf("failed to create PVCs: %w", err)
	}

	vgsc, err := vol.GetVolumeGroupSnapshotClass()
	if err != nil {
		return fmt.Errorf("failed to get volume group snapshot class: %w", err)
	}
	// Create a volume group snapshot class
	vgscName := v.framework.Namespace.Name + "-vgsc"
	vgsc.Name = vgscName
	err = v.CreateVolumeGroupSnapshotClass(vgsc)
	if err != nil {
		return fmt.Errorf("failed to create volume group snapshot: %w", err)
	}
	vgsName := v.framework.Namespace.Name + "-vgs"
	// Create a volume group snapshot
	volumeGroupSnapshot, err := v.CreateVolumeGroupSnapshot(vgsName, vgscName, pvcLabels)
	if err != nil {
		return fmt.Errorf("failed to create volume group snapshot: %w", err)
	}

	// Create and delete additional group snapshots.
	for i := range v.additionalVGSnapshotCount {
		newVGSName := fmt.Sprintf("%s-%d", vgsName, i)
		_, err = v.CreateVolumeGroupSnapshot(newVGSName, vgscName, pvcLabels)
		if err != nil {
			return fmt.Errorf("failed to create volume group snapshot %q: %w", newVGSName, err)
		}
	}
	for i := range v.additionalVGSnapshotCount {
		newVGSName := fmt.Sprintf("%s-%d", vgsName, i)
		err = v.DeleteVolumeGroupSnapshot(newVGSName)
		if err != nil {
			return fmt.Errorf("failed to delete volume group snapshot %q: %w", newVGSName, err)
		}
	}

	clonePVCs, err := v.CreatePVCClones(volumeGroupSnapshot)
	if err != nil {
		return fmt.Errorf("failed to create clones: %w", err)
	}
	// create pods using the cloned PVCs
	pods, err := v.CreatePods(clonePVCs)
	if err != nil {
		return fmt.Errorf("failed to create pods: %w", err)
	}
	// validate the resources in the backend
	err = vol.ValidateResourcesForCreate(volumeGroupSnapshot)
	if err != nil {
		return fmt.Errorf("failed to validate resources for create: %w", err)
	}
	// Delete the pods
	err = v.DeletePods(pods)
	if err != nil {
		return fmt.Errorf("failed to delete pods: %w", err)
	}
	// Delete the clones
	err = v.DeletePVCs(clonePVCs)
	if err != nil {
		return fmt.Errorf("failed to delete clones: %w", err)
	}
	// Delete the PVCs
	err = v.DeletePVCs(pvcs)
	if err != nil {
		return fmt.Errorf("failed to delete PVCs: %w", err)
	}
	// Delete the volume group snapshot
	err = v.DeleteVolumeGroupSnapshot(volumeGroupSnapshot.Name)
	if err != nil {
		return fmt.Errorf("failed to delete volume group snapshot: %w", err)
	}
	// validate the resources in the backend after deleting the resources
	err = vol.ValidateResourcesForDelete()
	if err != nil {
		return fmt.Errorf("failed to validate resources for delete: %w", err)
	}
	// Delete the volume group snapshot class
	err = v.DeleteVolumeGroupSnapshotClass(vgscName)
	if err != nil {
		return fmt.Errorf("failed to delete volume group snapshot class: %w", err)
	}

	return nil
}
