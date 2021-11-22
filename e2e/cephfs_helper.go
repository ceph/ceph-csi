package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

const (
	adminUser = "admin"
)

// validateSubvolumegroup validates whether subvolumegroup is present.
func validateSubvolumegroup(f *framework.Framework, subvolgrp string) error {
	cmd := fmt.Sprintf("ceph fs subvolumegroup getpath myfs %s", subvolgrp)
	stdOut, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return fmt.Errorf("failed to exec command in toolbox: %w", err)
	}
	if stdErr != "" {
		return fmt.Errorf("failed to getpath for subvolumegroup %s : %v", subvolgrp, stdErr)
	}
	expectedGrpPath := "/volumes/" + subvolgrp
	stdOut = strings.TrimSpace(stdOut)
	if stdOut != expectedGrpPath {
		return fmt.Errorf("error unexpected group path. Found: %s", stdOut)
	}

	return nil
}

func createCephfsStorageClass(
	c kubernetes.Interface,
	f *framework.Framework,
	enablePool bool,
	params map[string]string) error {
	scPath := fmt.Sprintf("%s/%s", cephFSExamplePath, "storageclass.yaml")
	sc, err := getStorageClass(scPath)
	if err != nil {
		return err
	}
	sc.Parameters["fsName"] = "myfs"
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"] = cephFSProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-name"] = cephFSProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/node-stage-secret-name"] = cephFSNodePluginSecretName

	if enablePool {
		sc.Parameters["pool"] = "myfs-data0"
	}

	// overload any parameters that were passed
	if params == nil {
		// create an empty params, so that params["clusterID"] below
		// does not panic
		params = map[string]string{}
	}
	for param, value := range params {
		sc.Parameters[param] = value
	}

	// fetch and set fsID from the cluster if not set in params
	if _, found := params["clusterID"]; !found {
		fsID, stdErr, failErr := execCommandInToolBoxPod(f, "ceph fsid", rookNamespace)
		if failErr != nil {
			return failErr
		}
		if stdErr != "" {
			return fmt.Errorf("error getting fsid %v", stdErr)
		}
		sc.Parameters["clusterID"] = strings.Trim(fsID, "\n")
	}
	sc.Namespace = cephCSINamespace
	_, err = c.StorageV1().StorageClasses().Create(context.TODO(), &sc, metav1.CreateOptions{})

	return err
}

func createCephfsSecret(f *framework.Framework, secretName, userName, userKey string) error {
	scPath := fmt.Sprintf("%s/%s", cephFSExamplePath, "secret.yaml")
	sc, err := getSecret(scPath)
	if err != nil {
		return err
	}
	if secretName != "" {
		sc.Name = secretName
	}
	sc.StringData["adminID"] = userName
	sc.StringData["adminKey"] = userKey
	delete(sc.StringData, "userID")
	delete(sc.StringData, "userKey")
	sc.Namespace = cephCSINamespace
	_, err = f.ClientSet.CoreV1().Secrets(cephCSINamespace).Create(context.TODO(), &sc, metav1.CreateOptions{})

	return err
}

// unmountCephFSVolume unmounts a cephFS volume mounted on a pod.
func unmountCephFSVolume(f *framework.Framework, appName, pvcName string) error {
	pod, err := f.ClientSet.CoreV1().Pods(f.UniqueName).Get(context.TODO(), appName, metav1.GetOptions{})
	if err != nil {
		e2elog.Logf("Error occurred getting pod %s in namespace %s", appName, f.UniqueName)

		return fmt.Errorf("failed to get pod: %w", err)
	}
	pvc, err := f.ClientSet.CoreV1().
		PersistentVolumeClaims(f.UniqueName).
		Get(context.TODO(), pvcName, metav1.GetOptions{})
	if err != nil {
		e2elog.Logf("Error occurred getting PVC %s in namespace %s", pvcName, f.UniqueName)

		return fmt.Errorf("failed to get pvc: %w", err)
	}
	cmd := fmt.Sprintf(
		"umount /var/lib/kubelet/pods/%s/volumes/kubernetes.io~csi/%s/mount",
		pod.UID,
		pvc.Spec.VolumeName)
	_, stdErr, err := execCommandInDaemonsetPod(
		f,
		cmd,
		cephFSDeamonSetName,
		pod.Spec.NodeName,
		cephFSContainerName,
		cephCSINamespace)
	if stdErr != "" {
		e2elog.Logf("StdErr occurred: %s", stdErr)
	}

	return err
}

func deleteBackingCephFSVolume(f *framework.Framework, pvc *v1.PersistentVolumeClaim) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("ceph fs subvolume rm %s %s %s", fileSystemName, imageData.imageName, subvolumegroup)
	_, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("error deleting backing volume %s %v", imageData.imageName, stdErr)
	}

	return nil
}

type cephfsSubVolume struct {
	Name string `json:"name"`
}

func listCephFSSubVolumes(f *framework.Framework, filesystem, groupname string) ([]cephfsSubVolume, error) {
	var subVols []cephfsSubVolume
	stdout, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf("ceph fs subvolume ls %s --group_name=%s --format=json", filesystem, groupname),
		rookNamespace)
	if err != nil {
		return subVols, err
	}
	if stdErr != "" {
		return subVols, fmt.Errorf("error listing subolumes %v", stdErr)
	}

	err = json.Unmarshal([]byte(stdout), &subVols)
	if err != nil {
		return subVols, err
	}

	return subVols, nil
}

// getSubvolumepath validates whether subvolumegroup is present.
func getSubvolumePath(f *framework.Framework, filesystem, subvolgrp, subvolume string) (string, error) {
	cmd := fmt.Sprintf("ceph fs subvolume getpath %s %s --group_name=%s", filesystem, subvolume, subvolgrp)
	stdOut, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return "", err
	}
	if stdErr != "" {
		return "", fmt.Errorf("failed to getpath for subvolume %s : %s", subvolume, stdErr)
	}

	return strings.TrimSpace(stdOut), nil
}

func getSnapName(snapNamespace, snapName string) (string, error) {
	sclient, err := newSnapshotClient()
	if err != nil {
		return "", err
	}
	snap, err := sclient.
		VolumeSnapshots(snapNamespace).
		Get(context.TODO(), snapName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get volumesnapshot: %w", err)
	}
	sc, err := sclient.
		VolumeSnapshotContents().
		Get(context.TODO(), *snap.Status.BoundVolumeSnapshotContentName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get volumesnapshotcontent: %w", err)
	}
	snapIDRegex := regexp.MustCompile(`(\w+\-?){5}$`)
	snapID := snapIDRegex.FindString(*sc.Status.SnapshotHandle)
	snapshotName := fmt.Sprintf("csi-snap-%s", snapID)
	e2elog.Logf("snapshotName= %s", snapshotName)

	return snapshotName, nil
}

func deleteBackingCephFSSubvolumeSnapshot(
	f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	snap *snapapi.VolumeSnapshot) error {
	snapshotName, err := getSnapName(snap.Namespace, snap.Name)
	if err != nil {
		return err
	}
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(
		"ceph fs subvolume snapshot rm %s %s %s %s",
		fileSystemName,
		imageData.imageName,
		snapshotName,
		subvolumegroup)
	_, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("error deleting backing snapshot %s %v", snapshotName, stdErr)
	}

	return nil
}
