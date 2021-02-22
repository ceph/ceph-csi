package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	snapapi "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	snapclient "github.com/kubernetes-csi/external-snapshotter/v2/pkg/client/clientset/versioned"
	. "github.com/onsi/gomega" // nolint
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

func getSnapshotClass(path string) snapapi.VolumeSnapshotClass {
	sc := snapapi.VolumeSnapshotClass{}
	err := unmarshal(path, &sc)
	Expect(err).Should(BeNil())
	return sc
}

func getSnapshot(path string) snapapi.VolumeSnapshot {
	sc := snapapi.VolumeSnapshot{}
	err := unmarshal(path, &sc)
	Expect(err).Should(BeNil())
	return sc
}

func newSnapshotClient() (*snapclient.Clientset, error) {
	config, err := framework.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("error creating client: %v", err.Error())
	}
	c, err := snapclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating snapshot client: %v", err.Error())
	}
	return c, err
}

func createSnapshot(snap *snapapi.VolumeSnapshot, t int) error {
	sclient, err := newSnapshotClient()
	if err != nil {
		return err
	}
	_, err = sclient.SnapshotV1beta1().VolumeSnapshots(snap.Namespace).Create(context.TODO(), snap, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	e2elog.Logf("snapshot with name %v created in %v namespace", snap.Name, snap.Namespace)

	timeout := time.Duration(t) * time.Minute
	name := snap.Name
	start := time.Now()
	e2elog.Logf("waiting for %v to be in ready state", snap)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		e2elog.Logf("waiting for snapshot %s (%d seconds elapsed)", snap.Name, int(time.Since(start).Seconds()))
		snaps, err := sclient.SnapshotV1beta1().VolumeSnapshots(snap.Namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			e2elog.Logf("Error getting snapshot in namespace: '%s': %v", snap.Namespace, err)
			if isRetryableAPIError(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if snaps.Status == nil || snaps.Status.ReadyToUse == nil {
			return false, nil
		}
		if *snaps.Status.ReadyToUse {
			return true, nil
		}
		e2elog.Logf("snapshot %s in %v state", snap.Name, *snaps.Status.ReadyToUse)
		return false, nil
	})
}

func deleteSnapshot(snap *snapapi.VolumeSnapshot, t int) error {
	sclient, err := newSnapshotClient()
	if err != nil {
		return err
	}
	err = sclient.SnapshotV1beta1().VolumeSnapshots(snap.Namespace).Delete(context.TODO(), snap.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	timeout := time.Duration(t) * time.Minute
	name := snap.Name
	start := time.Now()
	e2elog.Logf("Waiting up to %v to be deleted", snap)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		e2elog.Logf("deleting snapshot %s (%d seconds elapsed)", name, int(time.Since(start).Seconds()))
		_, err := sclient.SnapshotV1beta1().VolumeSnapshots(snap.Namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err == nil {
			return false, nil
		}

		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("get on deleted snapshot %v failed with error other than \"not found\": %v", name, err)
		}

		return true, nil
	})
}

func createRBDSnapshotClass(f *framework.Framework) error {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "snapshotclass.yaml")
	sc := getSnapshotClass(scPath)

	sc.Parameters["csi.storage.k8s.io/snapshotter-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/snapshotter-secret-name"] = rbdProvisionerSecretName

	fsID, stdErr, err := execCommandInToolBoxPod(f, "ceph fsid", rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to get fsid from ceph cluster %s", stdErr)
	}
	fsID = strings.Trim(fsID, "\n")
	sc.Parameters["clusterID"] = fsID
	sclient, err := newSnapshotClient()
	if err != nil {
		return err
	}
	_, err = sclient.SnapshotV1beta1().VolumeSnapshotClasses().Create(context.TODO(), &sc, metav1.CreateOptions{})
	return err
}

func createCephFSSnapshotClass(f *framework.Framework) error {
	scPath := fmt.Sprintf("%s/%s", cephfsExamplePath, "snapshotclass.yaml")
	sc := getSnapshotClass(scPath)
	sc.Parameters["csi.storage.k8s.io/snapshotter-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/snapshotter-secret-name"] = cephFSProvisionerSecretName
	fsID, stdErr, err := execCommandInToolBoxPod(f, "ceph fsid", rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to get fsid from ceph cluster %s", stdErr)
	}
	fsID = strings.Trim(fsID, "\n")
	sc.Parameters["clusterID"] = fsID
	sclient, err := newSnapshotClient()
	if err != nil {
		return err
	}
	_, err = sclient.SnapshotV1beta1().VolumeSnapshotClasses().Create(context.TODO(), &sc, metav1.CreateOptions{})
	return err
}
