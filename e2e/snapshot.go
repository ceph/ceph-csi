package e2e

import (
	"encoding/json"
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
	testutils "k8s.io/kubernetes/test/utils"
)

type snapInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Timestamp string `json:"timestamp"`
}

func getSnapshotClass(path string) snapapi.VolumeSnapshotClass {
	sc := snapapi.VolumeSnapshotClass{}
	sc.Kind = "VolumeSnapshotClass"
	sc.APIVersion = "snapshot.storage.k8s.io/v1beta1"
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
	_, err = sclient.SnapshotV1beta1().VolumeSnapshots(snap.Namespace).Create(snap)
	if err != nil {
		return err
	}
	e2elog.Logf("snapshot with name %v created in %v namespace", snap.Name, snap.Namespace)

	timeout := time.Duration(t) * time.Minute
	name := snap.Name
	start := time.Now()
	e2elog.Logf("Waiting up to %v to be in Ready state", snap)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		e2elog.Logf("waiting for snapshot %s (%d seconds elapsed)", snap.Name, int(time.Since(start).Seconds()))
		snaps, err := sclient.SnapshotV1beta1().VolumeSnapshots(snap.Namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			e2elog.Logf("Error getting snapshot in namespace: '%s': %v", snap.Namespace, err)
			if testutils.IsRetryableAPIError(err) {
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
	err = sclient.SnapshotV1beta1().VolumeSnapshots(snap.Namespace).Delete(snap.Name, &metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	timeout := time.Duration(t) * time.Minute
	name := snap.Name
	start := time.Now()
	e2elog.Logf("Waiting up to %v to be deleted", snap)

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		e2elog.Logf("deleting snapshot %s (%d seconds elapsed)", name, int(time.Since(start).Seconds()))
		_, err := sclient.SnapshotV1beta1().VolumeSnapshots(snap.Namespace).Get(name, metav1.GetOptions{})
		if err == nil {
			return false, nil
		}

		if !apierrs.IsNotFound(err) {
			return false, fmt.Errorf("get on deleted snapshot %v failed with error other than \"not found\": %v", name, err)
		}

		return true, nil
	})
}

func listSnapshots(f *framework.Framework, pool, imageName string) ([]snapInfo, error) {
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}
	command := fmt.Sprintf("rbd snap ls %s/%s --format=json", pool, imageName)
	stdout, stdErr := execCommandInPod(f, command, rookNamespace, &opt)
	Expect(stdErr).Should(BeEmpty())

	var snapInfos []snapInfo

	err := json.Unmarshal([]byte(stdout), &snapInfos)
	return snapInfos, err
}

func createRBDSnapshotClass(f *framework.Framework) {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "snapshotclass.yaml")
	sc := getSnapshotClass(scPath)

	sc.Parameters["csi.storage.k8s.io/snapshotter-secret-namespace"] = cephCSINamespace

	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}
	fsID, stdErr := execCommandInPod(f, "ceph fsid", rookNamespace, &opt)
	Expect(stdErr).Should(BeEmpty())
	fsID = strings.Trim(fsID, "\n")
	sc.Parameters["clusterID"] = fsID
	sclient, err := newSnapshotClient()
	Expect(err).Should(BeNil())
	_, err = sclient.SnapshotV1beta1().VolumeSnapshotClasses().Create(&sc)
	Expect(err).Should(BeNil())
}
