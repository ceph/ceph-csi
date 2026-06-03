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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	scv1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	adminUser = "admin"
)

// validateSubvolumegroup validates whether subvolumegroup is present.
func validateSubvolumegroup(f *framework.Framework, subvolgrp string) error {
	cmd := fmt.Sprintf("ceph fs subvolumegroup getpath %s %s", fileSystemName, subvolgrp)
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
	params map[string]string,
) error {
	scPath := fmt.Sprintf("%s/%s", cephFSExamplePath, "storageclass.yaml")
	sc, err := getStorageClass(scPath)
	if err != nil {
		return err
	}

	err = updateStorageClassParameters(&sc, params, enablePool, f)
	if err != nil {
		return err
	}

	return createStorageClass(c, &sc)
}

func createCephfsStorageClassWaitForFirstConsumer(c kubernetes.Interface, f *framework.Framework,
	enablePool bool,
	params map[string]string) error {
	scPath := fmt.Sprintf("%s/%s", cephFSExamplePath, "storageclass.yaml")
	sc, err := getStorageClass(scPath)
	if err != nil {
		return err
	}

	err = updateStorageClassParameters(&sc, params, enablePool, f)
	if err != nil {
		return err
	}

	// Set the volume binding mode to WaitForFirstConsumer
	value := scv1.VolumeBindingWaitForFirstConsumer
	sc.VolumeBindingMode = &value

	return createStorageClass(c, &sc)
}

func updateStorageClassParameters(sc *scv1.StorageClass, params map[string]string, enablePool bool, f *framework.Framework) error {
	if sc == nil {
		return fmt.Errorf("StorageClass is nil")
	}

	sc.Parameters["fsName"] = fileSystemName
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"] = cephFSProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-name"] = cephFSProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/controller-publish-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/controller-publish-secret-name"] = cephFSProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/node-stage-secret-name"] = cephFSNodePluginSecretName

	if enablePool {
		sc.Parameters["pool"] = "myfs-replicated"
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
		fsID, err := getClusterID(f)
		if err != nil {
			return fmt.Errorf("failed to get clusterID: %w", err)
		}
		sc.Parameters["clusterID"] = fsID
	}

	return nil
}

func createStorageClass(c kubernetes.Interface, sc *scv1.StorageClass) error {
	timeout := time.Duration(deployTimeout) * time.Minute
	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		_, err := c.StorageV1().StorageClasses().Create(ctx, sc, metav1.CreateOptions{})
		if err != nil {
			framework.Logf("error creating StorageClass %q: %v", sc.Name, err)
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to create StorageClass %q: %w", sc.Name, err)
		}

		return true, nil
	})
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
	sc.StringData["userID"] = userName
	sc.StringData["userKey"] = userKey
	sc.Namespace = cephCSINamespace
	_, err = f.ClientSet.CoreV1().Secrets(cephCSINamespace).Create(context.TODO(), &sc, metav1.CreateOptions{})

	return err
}

// unmountCephFSVolume unmounts a cephFS volume mounted on a pod.
func unmountCephFSVolume(f *framework.Framework, appName, pvcName string) error {
	pod, err := f.ClientSet.CoreV1().Pods(f.UniqueName).Get(context.TODO(), appName, metav1.GetOptions{})
	if err != nil {
		framework.Logf("Error occurred getting pod %s in namespace %s", appName, f.UniqueName)

		return fmt.Errorf("failed to get pod: %w", err)
	}
	pvc, err := getPersistentVolumeClaim(f.ClientSet, f.UniqueName, pvcName)
	if err != nil {
		framework.Logf("Error occurred getting PVC %s in namespace %s", pvcName, f.UniqueName)

		return fmt.Errorf("failed to get pvc: %w", err)
	}
	cmd := fmt.Sprintf(
		"umount /var/lib/kubelet/pods/%s/volumes/kubernetes.io~csi/%s/mount",
		pod.UID,
		pvc.Spec.VolumeName)
	stdErr, err := execCommandInDaemonsetPod(
		f,
		cmd,
		cephFSDeployment.getDaemonsetName(),
		pod.Spec.NodeName,
		cephFSContainerName,
		cephCSINamespace)
	if stdErr != "" {
		framework.Logf("StdErr occurred: %s", stdErr)
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

func cephfsOptions(pool string) string {
	if radosNamespace != "" {
		return "--pool=" + pool + " --namespace=" + radosNamespace
	}

	// default namespace is csi
	return "--pool=" + pool + " --namespace=csi"
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
		return subVols, fmt.Errorf("error listing subvolumes %v", stdErr)
	}

	err = json.Unmarshal([]byte(stdout), &subVols)
	if err != nil {
		return subVols, err
	}

	return subVols, nil
}

type cephfsSubvolumeMetadata struct {
	PVCNameKey      string `json:"csi.storage.k8s.io/pvc/name"`
	PVCNamespaceKey string `json:"csi.storage.k8s.io/pvc/namespace"`
	PVNameKey       string `json:"csi.storage.k8s.io/pv/name"`
	ClusterNameKey  string `json:"csi.ceph.com/cluster/name"`
}

func listCephFSSubvolumeMetadata(
	f *framework.Framework,
	filesystem,
	subvolume,
	groupname string,
) (*cephfsSubvolumeMetadata, error) {
	stdout, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf("ceph fs subvolume metadata ls %s %s --group_name=%s --format=json", filesystem, subvolume, groupname),
		rookNamespace)
	if err != nil {
		return nil, err
	}
	if stdErr != "" {
		return nil, fmt.Errorf("error listing subvolume metadata %v", stdErr)
	}

	metadata := &cephfsSubvolumeMetadata{}
	err = json.Unmarshal([]byte(stdout), metadata)
	if err != nil {
		return metadata, err
	}

	return metadata, nil
}

func getCephFSSubvolumeMetadata(
	f *framework.Framework,
	filesystem,
	subvolume,
	groupname,
	key string,
) (string, error) {
	stdout, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf("ceph fs subvolume metadata get %s %s --group_name=%s %s", filesystem, subvolume, groupname, key),
		rookNamespace)
	if err != nil {
		return "", err
	}
	if stdErr != "" {
		return "", fmt.Errorf("%s", stdErr)
	}

	return strings.TrimSpace(stdout), nil
}

type cephfsSnapshotMetadata struct {
	VolSnapNameKey        string `json:"csi.storage.k8s.io/volumesnapshot/name"`
	VolSnapNamespaceKey   string `json:"csi.storage.k8s.io/volumesnapshot/namespace"`
	VolSnapContentNameKey string `json:"csi.storage.k8s.io/volumesnapshotcontent/name"`
	ClusterNameKey        string `json:"csi.ceph.com/cluster/name"`
}

func listCephFSSnapshotMetadata(
	f *framework.Framework,
	filesystem,
	subvolume,
	snapname,
	groupname string,
) (*cephfsSnapshotMetadata, error) {
	stdout, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf("ceph fs subvolume snapshot metadata ls %s %s %s --group_name=%s --format=json",
			filesystem, subvolume, snapname, groupname),
		rookNamespace)
	if err != nil {
		return nil, err
	}
	if stdErr != "" {
		return nil, fmt.Errorf("error listing subvolume snapshots metadata %v", stdErr)
	}

	metadata := &cephfsSnapshotMetadata{}
	err = json.Unmarshal([]byte(stdout), metadata)
	if err != nil {
		return metadata, err
	}

	return metadata, nil
}

func getCephFSSnapshotMetadata(
	f *framework.Framework,
	filesystem,
	subvolume,
	snapshot,
	groupname,
	key string,
) (string, error) {
	stdout, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf("ceph fs subvolume snapshot metadata get %s %s %s %s --group_name=%s", filesystem, subvolume, snapshot, key, groupname),
		rookNamespace)
	if err != nil {
		return "", err
	}
	if stdErr != "" {
		return "", fmt.Errorf("%s", stdErr)
	}

	return strings.TrimSpace(stdout), nil
}

type cephfsSnapshot struct {
	Name string `json:"name"`
}

func listCephFSSnapshots(f *framework.Framework, filesystem, subvolume, groupname string) ([]cephfsSnapshot, error) {
	var snaps []cephfsSnapshot
	stdout, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf("ceph fs subvolume snapshot ls %s %s --group_name=%s --format=json", filesystem, subvolume, groupname),
		rookNamespace)
	if err != nil {
		return snaps, err
	}
	if stdErr != "" {
		return snaps, fmt.Errorf("error listing subolume snapshots %v", stdErr)
	}

	err = json.Unmarshal([]byte(stdout), &snaps)
	if err != nil {
		return snaps, err
	}

	return snaps, nil
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
	snapshotName := "csi-snap-" + snapID
	framework.Logf("snapshotName= %s", snapshotName)

	return snapshotName, nil
}

func deleteBackingCephFSSubvolumeSnapshot(
	f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	snap *snapapi.VolumeSnapshot,
) error {
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

func validateEncryptedCephfs(f *framework.Framework, pvName, appName string) error {
	pod, err := f.ClientSet.CoreV1().Pods(f.UniqueName).Get(context.TODO(), appName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod %q in namespace %q: %w", appName, f.UniqueName, err)
	}
	volumeMountPath := fmt.Sprintf(
		"/var/lib/kubelet/pods/%s/volumes/kubernetes.io~csi/%s/mount",
		pod.UID,
		pvName)

	selector, err := getDaemonSetLabelSelector(f, cephCSINamespace, cephFSDeployment.getDaemonsetName())
	if err != nil {
		return fmt.Errorf("failed to get labels: %w", err)
	}
	opt := metav1.ListOptions{
		LabelSelector: selector,
	}

	cmd := "getfattr --name=ceph.fscrypt.auth --only-values " + volumeMountPath
	_, _, err = execCommandInContainer(f, cmd, cephCSINamespace, "csi-cephfsplugin", &opt)
	if err != nil {
		cmd = "getfattr --recursive --dump " + volumeMountPath
		stdOut, stdErr, listErr := execCommandInContainer(f, cmd, cephCSINamespace, "csi-cephfsplugin", &opt)
		if listErr == nil {
			return fmt.Errorf("error checking for cephfs fscrypt xattr on %q. listing: %s %s",
				volumeMountPath, stdOut, stdErr)
		}

		return fmt.Errorf("error checking file xattr: %w", err)
	}

	return nil
}

func getInfoFromPVC(pvcNamespace, pvcName string, f *framework.Framework) (string, string, error) {
	c := f.ClientSet.CoreV1()
	pvc, err := c.PersistentVolumeClaims(pvcNamespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("failed to get pvc: %w", err)
	}

	pv, err := c.PersistentVolumes().Get(context.TODO(), pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("failed to get pv: %w", err)
	}

	return pv.Name, pv.Spec.CSI.VolumeHandle, nil
}

func validateFscryptAndAppBinding(pvcPath, appPath string, kms kmsConfig, f *framework.Framework) error {
	pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
	if err != nil {
		return err
	}

	pvName, csiVolumeHandle, err := getInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}
	err = validateEncryptedCephfs(f, pvName, app.Name)
	if err != nil {
		return err
	}

	if kms != noKMS && kms.canGetPassphrase() {
		// check new passphrase created
		_, stdErr := kms.getPassphrase(f, csiVolumeHandle)
		if stdErr != "" {
			return fmt.Errorf("failed to read passphrase from vault: %s", stdErr)
		}
	}

	err = deletePVCAndApp("", f, pvc, app)
	if err != nil {
		return err
	}

	if kms != noKMS && kms.canGetPassphrase() {
		// check new passphrase created
		stdOut, _ := kms.getPassphrase(f, csiVolumeHandle)
		if stdOut != "" {
			return fmt.Errorf("passphrase found in vault while should be deleted: %s", stdOut)
		}
	}

	if kms != noKMS && kms.canVerifyKeyDestroyed() {
		destroyed, msg := kms.verifyKeyDestroyed(f, csiVolumeHandle)
		if !destroyed {
			return fmt.Errorf("passphrased was not destroyed: %s", msg)
		} else if msg != "" {
			framework.Logf("passphrase destroyed, but message returned: %s", msg)
		}
	}

	return nil
}

//nolint:gocyclo,cyclop // test function
func validateFscryptClone(
	pvcPath, appPath, pvcSmartClonePath, appSmartClonePath string,
	kms kmsConfig,
	f *framework.Framework,
) {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		logAndFail("failed to load PVC: %v", err)
	}

	pvc.Namespace = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		logAndFail("failed to create PVC: %v", err)
	}
	app, err := loadApp(appPath)
	if err != nil {
		logAndFail("failed to load application: %v", err)
	}
	label := make(map[string]string)
	label[appKey] = appLabel
	app.Namespace = f.UniqueName
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
	app.Labels = label
	opt := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
	}
	wErr := writeDataInPod(app, &opt, f)
	if wErr != nil {
		logAndFail("failed to write data from application %v", wErr)
	}

	pvcClone, err := loadPVC(pvcSmartClonePath)
	if err != nil {
		logAndFail("failed to load PVC: %v", err)
	}
	pvcClone.Spec.DataSource.Name = pvc.Name
	pvcClone.Namespace = f.UniqueName
	appClone, err := loadApp(appSmartClonePath)
	if err != nil {
		logAndFail("failed to load application: %v", err)
	}
	appClone.Namespace = f.UniqueName
	appClone.Labels = map[string]string{
		appKey: f.UniqueName,
	}

	err = createPVCAndApp(f.UniqueName, f, pvcClone, appClone, deployTimeout)
	if err != nil {
		logAndFail("failed to create PVC or application (%s): %v", f.UniqueName, err)
	}

	_, csiVolumeHandle, err := getInfoFromPVC(pvcClone.Namespace, pvcClone.Name, f)
	if err != nil {
		logAndFail("failed to get pvc info: %s", err)
	}

	if kms != noKMS && kms.canGetPassphrase() {
		// check new passphrase created
		stdOut, stdErr := kms.getPassphrase(f, csiVolumeHandle)
		if stdOut != "" {
			framework.Logf("successfully read the passphrase from vault: %s", stdOut)
		}
		if stdErr != "" {
			logAndFail("failed to read passphrase from vault: %s", stdErr)
		}
	}

	// delete parent pvc
	err = deletePVCAndApp("", f, pvc, app)
	if err != nil {
		logAndFail("failed to delete PVC or application: %v", err)
	}

	err = deletePVCAndApp(f.UniqueName, f, pvcClone, appClone)
	if err != nil {
		logAndFail("failed to delete PVC or application (%s): %v", f.UniqueName, err)
	}

	if kms != noKMS && kms.canGetPassphrase() {
		// check passphrase deleted
		stdOut, _ := kms.getPassphrase(f, csiVolumeHandle)
		if stdOut != "" {
			logAndFail("passphrase found in vault while should be deleted: %s", stdOut)
		}
	}

	if kms != noKMS && kms.canVerifyKeyDestroyed() {
		destroyed, msg := kms.verifyKeyDestroyed(f, csiVolumeHandle)
		if !destroyed {
			logAndFail("passphrased was not destroyed: %s", msg)
		} else if msg != "" {
			framework.Logf("passphrase destroyed, but message returned: %s", msg)
		}
	}
}

func verifyClientAddressMetadataSnapshotBacked(
	f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	pod *v1.Pod,
	subVolumeName, snapshotName string,
) error {
	nodeId := pod.Spec.NodeName
	_, pvObject, err := getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get PVC and PV: %w", err)
	}
	volumeHandle := pvObject.Spec.CSI.VolumeHandle

	metadataKey := fmt.Sprintf(".cephfs.csi.ceph.com/clientaddress/%s/%s", volumeHandle, nodeId)
	metadataValue, err := getCephFSSnapshotMetadata(
		f, fileSystemName, subVolumeName, snapshotName, subvolumegroup, metadataKey)
	if err != nil {
		return fmt.Errorf("failed to get subvolume snapshot metadata %s: %w", metadataKey, err)
	}

	if metadataValue == "" {
		return fmt.Errorf("client address metadata %s value is empty", metadataKey)
	}

	return nil
}

func setCephFSSubvolumeMetadata(
	f *framework.Framework,
	filesystem,
	subvolume,
	groupname,
	key,
	value string,
) error {
	_, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf("ceph fs subvolume metadata set %s %s --group_name=%s %s %q",
			filesystem, subvolume, groupname, key, value),
		rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("%s", stdErr)
	}

	return nil
}

func removeCephFSSubvolumeMetadata(
	f *framework.Framework,
	filesystem,
	subvolume,
	groupname,
	key string,
) error {
	_, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf("ceph fs subvolume metadata rm %s %s --group_name=%s %s",
			filesystem, subvolume, groupname, key),
		rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("%s", stdErr)
	}

	return nil
}

// validateCephFSServiceAccountVolumeRestriction tests that CephFS volume access
// can be restricted to a specific Kubernetes service account. It creates a PVC,
// sets the given saMetadataKey metadata on the backing CephFS subvolume, then
// verifies:
//   - A pod using the allowed service account can mount the volume.
//   - A pod using a different service account is rejected with PermissionDenied.
func validateCephFSServiceAccountVolumeRestriction(
	pvcPath, appPath, saMetadataKey string,
	f *framework.Framework,
) error {
	allowedSA := "allowed-sa-" + f.UniqueName
	deniedSA := "denied-sa-" + f.UniqueName
	thirdSA := "third-sa-" + f.UniqueName

	// Create service accounts.
	_, err := f.ClientSet.CoreV1().ServiceAccounts(f.UniqueName).Create(
		context.TODO(),
		&v1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: allowedSA},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to create allowed ServiceAccount: %w", err)
	}
	defer func() {
		delErr := f.ClientSet.CoreV1().ServiceAccounts(f.UniqueName).Delete(
			context.TODO(), allowedSA, metav1.DeleteOptions{})
		if delErr != nil {
			framework.Logf("failed to delete ServiceAccount %s: %v", allowedSA, delErr)
		}
	}()

	_, err = f.ClientSet.CoreV1().ServiceAccounts(f.UniqueName).Create(
		context.TODO(),
		&v1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: deniedSA},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to create denied ServiceAccount: %w", err)
	}
	defer func() {
		delErr := f.ClientSet.CoreV1().ServiceAccounts(f.UniqueName).Delete(
			context.TODO(), deniedSA, metav1.DeleteOptions{})
		if delErr != nil {
			framework.Logf("failed to delete ServiceAccount %s: %v", deniedSA, delErr)
		}
	}()

	_, err = f.ClientSet.CoreV1().ServiceAccounts(f.UniqueName).Create(
		context.TODO(),
		&v1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: thirdSA},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to create third ServiceAccount: %w", err)
	}
	defer func() {
		delErr := f.ClientSet.CoreV1().ServiceAccounts(f.UniqueName).Delete(
			context.TODO(), thirdSA, metav1.DeleteOptions{})
		if delErr != nil {
			framework.Logf("failed to delete ServiceAccount %s: %v", thirdSA, delErr)
		}
	}()

	// Create PVC and wait for it to be bound.
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return fmt.Errorf("failed to load PVC: %w", err)
	}
	pvc.Namespace = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create PVC: %w", err)
	}

	defer func() {
		delErr := deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
		if delErr != nil {
			framework.Logf("failed to delete PVC: %v", delErr)
		}
	}()

	// Get the subvolume name from the PVC.
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return fmt.Errorf("failed to get image info from PVC: %w", err)
	}

	// Set the service account restriction metadata on the subvolume.
	err = setCephFSSubvolumeMetadata(f, fileSystemName, imageData.imageName, subvolumegroup,
		saMetadataKey, allowedSA)
	if err != nil {
		return fmt.Errorf("failed to set service account metadata: %w", err)
	}
	defer func() {
		delErr := removeCephFSSubvolumeMetadata(f, fileSystemName, imageData.imageName, subvolumegroup,
			saMetadataKey)
		if delErr != nil {
			framework.Logf("failed to remove service account metadata: %v", delErr)
		}
	}()

	// Verify the metadata was set correctly.
	saValue, err := getCephFSSubvolumeMetadata(f, fileSystemName, imageData.imageName,
		subvolumegroup, saMetadataKey)
	if err != nil {
		return fmt.Errorf("failed to get service account metadata: %w", err)
	}
	if saValue != allowedSA {
		return fmt.Errorf("expected service account metadata %q, got %q", allowedSA, saValue)
	}

	// Test 1: Pod with the allowed service account should succeed.
	app, err := loadApp(appPath)
	if err != nil {
		return fmt.Errorf("failed to load app: %w", err)
	}
	app.Namespace = f.UniqueName
	app.Name = "sa-allowed-pod"
	app.Spec.ServiceAccountName = allowedSA
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return fmt.Errorf("pod with allowed service account %q should have started but failed: %w", allowedSA, err)
	}
	framework.Logf("pod with allowed service account %q started successfully", allowedSA)
	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete allowed pod: %w", err)
	}

	// Test 2: Pod with a denied service account should fail with PermissionDenied.
	app, err = loadApp(appPath)
	if err != nil {
		return fmt.Errorf("failed to load app for denied test: %w", err)
	}
	app.Namespace = f.UniqueName
	app.Name = "sa-denied-pod"
	app.Spec.ServiceAccountName = deniedSA
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
	err = createAppErr(
		f.ClientSet, app, deployTimeout,
		[]string{"PermissionDenied", "is restricted to service account"},
	)
	if err != nil {
		return fmt.Errorf("pod with denied service account should have failed with PermissionDenied: %w", err)
	}
	framework.Logf("pod with denied service account %q was correctly rejected", deniedSA)
	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete denied pod: %w", err)
	}

	// Wait for volume to be fully detached before updating metadata
	// to ensure fresh ControllerPublishVolume call with new metadata.
	err = waitForPVCVolumeAttachmentsCleanup(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to wait for volume detachment: %w", err)
	}

	// Test 3: Update metadata to a comma-separated list of allowed service accounts and verify access.
	err = setCephFSSubvolumeMetadata(
		f, fileSystemName, imageData.imageName, subvolumegroup, saMetadataKey, allowedSA+","+thirdSA)
	if err != nil {
		return fmt.Errorf("failed to set comma-separated service account metadata: %w", err)
	}

	// Verify the metadata was set correctly.
	saValue, err = getCephFSSubvolumeMetadata(f, fileSystemName, imageData.imageName,
		subvolumegroup, saMetadataKey)
	if err != nil {
		return fmt.Errorf("failed to get service account metadata: %w", err)
	}
	if saValue != (allowedSA + "," + thirdSA) {
		return fmt.Errorf("expected service account metadata %q, got %q", allowedSA+","+thirdSA, saValue)
	}
	// Pod with thirdSA should succeed (in the list).
	app, err = loadApp(appPath)
	if err != nil {
		return fmt.Errorf("failed to load app for multi-SA test: %w", err)
	}
	app.Namespace = f.UniqueName
	app.Name = "sa-third-pod"
	app.Spec.ServiceAccountName = thirdSA
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return fmt.Errorf(
			"pod with third service account %q should have started but failed: %w", thirdSA, err)
	}
	framework.Logf("pod with third service account %q started successfully (comma-separated list)", thirdSA)
	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete third pod: %w", err)
	}

	// Pod with deniedSA should still be rejected.
	app, err = loadApp(appPath)
	if err != nil {
		return fmt.Errorf("failed to load app for multi-SA denied test: %w", err)
	}
	app.Namespace = f.UniqueName
	app.Name = "sa-denied-multi-pod"
	app.Spec.ServiceAccountName = deniedSA
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
	err = createAppErr(
		f.ClientSet, app, deployTimeout,
		[]string{"PermissionDenied", "is restricted to service account"},
	)
	if err != nil {
		return fmt.Errorf(
			"pod with denied SA should have failed with PermissionDenied (comma-separated list): %w", err)
	}
	framework.Logf("pod with denied service account %q was correctly rejected (comma-separated list)", deniedSA)
	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete denied multi pod: %w", err)
	}

	return nil
}

func verifyUserIdMappingMetadataSnapshotBacked(
	f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	pod *v1.Pod,
	subVolumeName, snapshotName string,
) error {
	nodeId := pod.Spec.NodeName
	_, pvObject, err := getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get PVC and PV: %w", err)
	}
	volumeHandle := pvObject.Spec.CSI.VolumeHandle

	expectedValue := keyringCephFSNodePluginUsername
	metadataKey := fmt.Sprintf(".cephfs.csi.ceph.com/userid/%s/%s", volumeHandle, nodeId)
	metadataValue, err := getCephFSSnapshotMetadata(
		f, fileSystemName, subVolumeName, snapshotName, subvolumegroup, metadataKey)
	if err != nil {
		return fmt.Errorf("failed to get subvolume snapshot metadata %s: %w", metadataKey, err)
	}

	if metadataValue != expectedValue {
		return fmt.Errorf("userId mapping metadata %s has unexpected value %s, expected %s",
			metadataKey, metadataValue, expectedValue)
	}

	return nil
}
