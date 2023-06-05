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

package e2e

import (
	"context"
	"fmt"
	"time"

	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	snapclient "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned/typed/volumesnapshot/v1"
	. "github.com/onsi/gomega" //nolint:golint // e2e uses Expect() and other Gomega functions
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"
)

func getSnapshotClass(path string) snapapi.VolumeSnapshotClass {
	sc := snapapi.VolumeSnapshotClass{}
	err := unmarshal(path, &sc)
	Expect(err).ShouldNot(HaveOccurred())

	return sc
}

func getSnapshot(path string) snapapi.VolumeSnapshot {
	sc := snapapi.VolumeSnapshot{}
	err := unmarshal(path, &sc)
	Expect(err).ShouldNot(HaveOccurred())

	return sc
}

func newSnapshotClient() (*snapclient.SnapshotV1Client, error) {
	config, err := framework.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("error creating client: %w", err)
	}
	c, err := snapclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating snapshot client: %w", err)
	}

	return c, err
}

func createSnapshot(snap *snapapi.VolumeSnapshot, t int) error {
	sclient, err := newSnapshotClient()
	if err != nil {
		return err
	}

	ctx := context.TODO()
	_, err = sclient.
		VolumeSnapshots(snap.Namespace).
		Create(ctx, snap, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create volumesnapshot: %w", err)
	}
	framework.Logf("snapshot with name %v created in %v namespace", snap.Name, snap.Namespace)

	timeout := time.Duration(t) * time.Minute
	name := snap.Name
	start := time.Now()
	framework.Logf("waiting for %v to be in ready state", snap)

	return wait.PollUntilContextTimeout(ctx, poll, timeout, true, func(ctx context.Context) (bool, error) {
		framework.Logf("waiting for snapshot %s (%d seconds elapsed)", snap.Name, int(time.Since(start).Seconds()))
		snaps, err := sclient.
			VolumeSnapshots(snap.Namespace).
			Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			framework.Logf("Error getting snapshot in namespace: '%s': %v", snap.Namespace, err)
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to get volumesnapshot: %w", err)
		}
		if snaps.Status == nil || snaps.Status.ReadyToUse == nil {
			return false, nil
		}
		if *snaps.Status.ReadyToUse {
			return true, nil
		}
		framework.Logf("snapshot %s in %v state", snap.Name, *snaps.Status.ReadyToUse)

		return false, nil
	})
}

func deleteSnapshot(snap *snapapi.VolumeSnapshot, t int) error {
	sclient, err := newSnapshotClient()
	if err != nil {
		return err
	}

	ctx := context.TODO()
	err = sclient.
		VolumeSnapshots(snap.Namespace).
		Delete(ctx, snap.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete volumesnapshot: %w", err)
	}

	timeout := time.Duration(t) * time.Minute
	name := snap.Name
	start := time.Now()
	framework.Logf("Waiting up to %v to be deleted", snap)

	return wait.PollUntilContextTimeout(ctx, poll, timeout, true, func(ctx context.Context) (bool, error) {
		framework.Logf("deleting snapshot %s (%d seconds elapsed)", name, int(time.Since(start).Seconds()))
		_, err := sclient.
			VolumeSnapshots(snap.Namespace).
			Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			return false, nil
		}

		if isRetryableAPIError(err) {
			return false, nil
		}
		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf(
				"get on deleted snapshot %v failed : other than \"not found\": %w",
				name,
				err)
		}

		return true, nil
	})
}

func createRBDSnapshotClass(f *framework.Framework) error {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "snapshotclass.yaml")
	sc := getSnapshotClass(scPath)

	sc.Parameters["csi.storage.k8s.io/snapshotter-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/snapshotter-secret-name"] = rbdProvisionerSecretName

	fsID, err := getClusterID(f)
	if err != nil {
		return fmt.Errorf("failed to get clusterID: %w", err)
	}
	sc.Parameters["clusterID"] = fsID
	sclient, err := newSnapshotClient()
	if err != nil {
		return err
	}
	_, err = sclient.VolumeSnapshotClasses().Create(context.TODO(), &sc, metav1.CreateOptions{})

	return err
}

func deleteRBDSnapshotClass() error {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "snapshotclass.yaml")
	sc := getSnapshotClass(scPath)

	sclient, err := newSnapshotClient()
	if err != nil {
		return err
	}

	return sclient.VolumeSnapshotClasses().Delete(context.TODO(), sc.Name, metav1.DeleteOptions{})
}

func createCephFSSnapshotClass(f *framework.Framework) error {
	scPath := fmt.Sprintf("%s/%s", cephFSExamplePath, "snapshotclass.yaml")
	sc := getSnapshotClass(scPath)
	sc.Parameters["csi.storage.k8s.io/snapshotter-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/snapshotter-secret-name"] = cephFSProvisionerSecretName

	fsID, err := getClusterID(f)
	if err != nil {
		return fmt.Errorf("failed to get clusterID: %w", err)
	}
	sc.Parameters["clusterID"] = fsID
	sclient, err := newSnapshotClient()
	if err != nil {
		return err
	}
	_, err = sclient.VolumeSnapshotClasses().Create(context.TODO(), &sc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create volumesnapshotclass: %w", err)
	}

	return err
}

func createNFSSnapshotClass(f *framework.Framework) error {
	scPath := fmt.Sprintf("%s/%s", nfsExamplePath, "snapshotclass.yaml")
	sc := getSnapshotClass(scPath)
	sc.Parameters["csi.storage.k8s.io/snapshotter-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/snapshotter-secret-name"] = cephFSProvisionerSecretName

	fsID, err := getClusterID(f)
	if err != nil {
		return fmt.Errorf("failed to get clusterID: %w", err)
	}
	sc.Parameters["clusterID"] = fsID
	sclient, err := newSnapshotClient()
	if err != nil {
		return err
	}

	timeout := time.Duration(deployTimeout) * time.Minute

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		_, err = sclient.VolumeSnapshotClasses().Create(ctx, &sc, metav1.CreateOptions{})
		if err != nil {
			framework.Logf("error creating SnapshotClass %q: %v", sc.Name, err)
			if apierrs.IsAlreadyExists(err) {
				return true, nil
			}
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to create SnapshotClass %q: %w", sc.Name, err)
		}

		return true, nil
	})
}

func deleteNFSSnapshotClass() error {
	scPath := fmt.Sprintf("%s/%s", nfsExamplePath, "snapshotclass.yaml")
	sc := getSnapshotClass(scPath)

	sclient, err := newSnapshotClient()
	if err != nil {
		return err
	}

	timeout := time.Duration(deployTimeout) * time.Minute

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		err = sclient.VolumeSnapshotClasses().Delete(ctx, sc.Name, metav1.DeleteOptions{})
		if err != nil {
			framework.Logf("error deleting SnapshotClass %q: %v", sc.Name, err)
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to delete SnapshotClass %q: %w", sc.Name, err)
		}

		return true, nil
	})
}

func getVolumeSnapshotContent(namespace, snapshotName string) (*snapapi.VolumeSnapshotContent, error) {
	sclient, err := newSnapshotClient()
	if err != nil {
		return nil, err
	}

	ctx := context.TODO()
	snapshot, err := sclient.
		VolumeSnapshots(namespace).
		Get(ctx, snapshotName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get volumesnapshot: %w", err)
	}

	volumeSnapshotContent, err := sclient.
		VolumeSnapshotContents().
		Get(ctx, *snapshot.Status.BoundVolumeSnapshotContentName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get volumesnapshotcontent: %w", err)
	}

	return volumeSnapshotContent, nil
}

//nolint:gocyclo,cyclop // reduce complexity
func validateBiggerPVCFromSnapshot(f *framework.Framework,
	pvcPath,
	appPath,
	snapPath,
	pvcClonePath,
	appClonePath string,
) error {
	const (
		size    = "1Gi"
		newSize = "2Gi"
	)
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return fmt.Errorf("failed to load PVC: %w", err)
	}
	label := make(map[string]string)
	pvc.Namespace = f.UniqueName
	pvc.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(size)
	app, err := loadApp(appPath)
	if err != nil {
		return fmt.Errorf("failed to load app: %w", err)
	}
	label[appKey] = appLabel
	app.Namespace = f.UniqueName
	app.Labels = label
	opt := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
	}
	err = createPVCAndApp("", f, pvc, app, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create pvc and application: %w", err)
	}

	snap := getSnapshot(snapPath)
	snap.Namespace = f.UniqueName
	snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
	err = createSnapshot(&snap, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}
	err = deletePVCAndApp("", f, pvc, app)
	if err != nil {
		return fmt.Errorf("failed to delete pvc and application: %w", err)
	}
	pvcClone, err := loadPVC(pvcClonePath)
	if err != nil {
		framework.Failf("failed to load PVC: %v", err)
	}
	pvcClone.Namespace = f.UniqueName
	pvcClone.Spec.DataSource.Name = snap.Name
	pvcClone.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(newSize)
	appClone, err := loadApp(appClonePath)
	if err != nil {
		framework.Failf("failed to load application: %v", err)
	}
	appClone.Namespace = f.UniqueName
	appClone.Labels = label
	err = createPVCAndApp("", f, pvcClone, appClone, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create pvc clone and application: %w", err)
	}
	err = deleteSnapshot(&snap, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}
	if pvcClone.Spec.VolumeMode == nil || *pvcClone.Spec.VolumeMode == v1.PersistentVolumeFilesystem {
		err = checkDirSize(appClone, f, &opt, newSize)
		if err != nil {
			return fmt.Errorf("failed to validate directory size: %w", err)
		}
	}

	if pvcClone.Spec.VolumeMode != nil && *pvcClone.Spec.VolumeMode == v1.PersistentVolumeBlock {
		err = checkDeviceSize(appClone, f, &opt, newSize)
		if err != nil {
			return fmt.Errorf("failed to validate device size: %w", err)
		}

		// make sure we had unset snapshot metadata on CreateVolume
		// from snapshot
		var (
			volSnapName        string
			volSnapNamespace   string
			volSnapContentName string
			stdErr             string
			imageList          []string
		)
		imageList, err = listRBDImages(f, defaultRBDPool)
		if err != nil {
			framework.Failf("failed to list rbd images: %v", err)
		}
		framework.Logf("list of rbd images: %v", imageList)
		volSnapName, stdErr, err = execCommandInToolBoxPod(f,
			formatImageMetaGetCmd(defaultRBDPool, imageList[0], volSnapNameKey),
			rookNamespace)
		if checkGetKeyError(err, stdErr) {
			framework.Failf("found volume snapshot name %s/%s %s=%s: err=%v stdErr=%q",
				rbdOptions(defaultRBDPool), imageList[0], volSnapNameKey, volSnapName, err, stdErr)
		}
		volSnapNamespace, stdErr, err = execCommandInToolBoxPod(f,
			formatImageMetaGetCmd(defaultRBDPool, imageList[0], volSnapNamespaceKey),
			rookNamespace)
		if checkGetKeyError(err, stdErr) {
			framework.Failf("found volume snapshot namespace %s/%s %s=%s: err=%v stdErr=%q",
				rbdOptions(defaultRBDPool), imageList[0], volSnapNamespaceKey, volSnapNamespace, err, stdErr)
		}
		volSnapContentName, stdErr, err = execCommandInToolBoxPod(f,
			formatImageMetaGetCmd(defaultRBDPool, imageList[0], volSnapContentNameKey),
			rookNamespace)
		if checkGetKeyError(err, stdErr) {
			framework.Failf("found snapshotcontent name %s/%s %s=%s: err=%v stdErr=%q",
				rbdOptions(defaultRBDPool), imageList[0], volSnapContentNameKey,
				volSnapContentName, err, stdErr)
		}
	}
	err = deletePVCAndApp("", f, pvcClone, appClone)
	if err != nil {
		return fmt.Errorf("failed to delete pvc and application: %w", err)
	}

	return nil
}
