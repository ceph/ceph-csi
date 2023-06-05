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
	"sync"
	"time"

	"github.com/ceph/ceph-csi/internal/util"

	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	scv1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

//nolint:gomnd // numbers specify Kernel versions.
var nbdResizeSupport = []util.KernelVersion{
	{
		Version:      5,
		PatchLevel:   3,
		SubLevel:     0,
		ExtraVersion: 0,
		Distribution: "",
		Backport:     false,
	}, // standard 5.3+ versions
}

//nolint:gomnd // numbers specify Kernel versions.
var fastDiffSupport = []util.KernelVersion{
	{
		Version:      5,
		PatchLevel:   3,
		SubLevel:     0,
		ExtraVersion: 0,
		Distribution: "",
		Backport:     false,
	}, // standard 5.3+ versions
}

//nolint:gomnd // numbers specify Kernel versions.
var deepFlattenSupport = []util.KernelVersion{
	{
		Version:      5,
		PatchLevel:   1,
		SubLevel:     0,
		ExtraVersion: 0,
		Distribution: "",
		Backport:     false,
	}, // standard 5.1+ versions
}

// To use `io-timeout=0` we need
// www.mail-archive.com/linux-block@vger.kernel.org/msg38060.html
//
//nolint:gomnd // numbers specify Kernel versions.
var nbdZeroIOtimeoutSupport = []util.KernelVersion{
	{
		Version:      5,
		PatchLevel:   4,
		SubLevel:     0,
		ExtraVersion: 0,
		Distribution: "",
		Backport:     false,
	}, // standard 5.4+ versions
	{
		Version:      4,
		PatchLevel:   18,
		SubLevel:     0,
		ExtraVersion: 305,
		Distribution: ".el8",
		Backport:     true,
	}, // CentOS 8.4
}

func imageSpec(pool, image string) string {
	if radosNamespace != "" {
		return pool + "/" + radosNamespace + "/" + image
	}

	return pool + "/" + image
}

func rbdOptions(pool string) string {
	if radosNamespace != "" {
		return "--pool=" + pool + " --namespace " + radosNamespace
	}

	return "--pool=" + pool
}

func createRBDStorageClass(
	c kubernetes.Interface,
	f *framework.Framework,
	name string,
	scOptions, parameters map[string]string,
	policy v1.PersistentVolumeReclaimPolicy,
) error {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "storageclass.yaml")
	sc, err := getStorageClass(scPath)
	if err != nil {
		return fmt.Errorf("failed to get sc: %w", err)
	}
	if name != "" {
		sc.Name = name
	}
	sc.Parameters["pool"] = defaultRBDPool
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"] = rbdProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-name"] = rbdProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/node-stage-secret-name"] = rbdNodePluginSecretName

	fsID, err := getClusterID(f)
	if err != nil {
		return fmt.Errorf("failed to get clusterID: %w", err)
	}

	sc.Parameters["clusterID"] = fsID
	for k, v := range parameters {
		sc.Parameters[k] = v
		// if any values are empty remove it from the map
		if v == "" {
			delete(sc.Parameters, k)
		}
	}

	if scOptions["volumeBindingMode"] == "WaitForFirstConsumer" {
		value := scv1.VolumeBindingWaitForFirstConsumer
		sc.VolumeBindingMode = &value
	}

	// comma separated mount options
	if opt, ok := scOptions[rbdMountOptions]; ok {
		mOpt := strings.Split(opt, ",")
		sc.MountOptions = append(sc.MountOptions, mOpt...)
	}
	sc.ReclaimPolicy = &policy

	timeout := time.Duration(deployTimeout) * time.Minute

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		_, err = c.StorageV1().StorageClasses().Create(ctx, &sc, metav1.CreateOptions{})
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

func createRadosNamespace(f *framework.Framework) error {
	stdOut, stdErr, err := execCommandInToolBoxPod(f,
		fmt.Sprintf("rbd namespace ls --pool=%s", defaultRBDPool), rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("error listing rbd namespace %v", stdErr)
	}
	if !strings.Contains(stdOut, radosNamespace) {
		_, stdErr, err = execCommandInToolBoxPod(f,
			fmt.Sprintf("rbd namespace create %s", rbdOptions(defaultRBDPool)), rookNamespace)
		if err != nil {
			return err
		}
		if stdErr != "" {
			return fmt.Errorf("error creating rbd namespace %v", stdErr)
		}
	}
	stdOut, stdErr, err = execCommandInToolBoxPod(f,
		fmt.Sprintf("rbd namespace ls --pool=%s", rbdTopologyPool), rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("error listing rbd namespace %v", stdErr)
	}

	if !strings.Contains(stdOut, radosNamespace) {
		_, stdErr, err = execCommandInToolBoxPod(f,
			fmt.Sprintf("rbd namespace create %s", rbdOptions(rbdTopologyPool)), rookNamespace)
		if err != nil {
			return err
		}
		if stdErr != "" {
			return fmt.Errorf("error creating rbd namespace %v", stdErr)
		}
	}

	return nil
}

func createRBDSecret(f *framework.Framework, secretName, userName, userKey string) error {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "secret.yaml")
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

type imageInfoFromPVC struct {
	imageID         string
	imageName       string
	csiVolumeHandle string
	pvName          string
}

// getImageInfoFromPVC reads volume handle of the bound PV to the passed in PVC,
// and returns imageInfoFromPVC or error.
func getImageInfoFromPVC(pvcNamespace, pvcName string, f *framework.Framework) (imageInfoFromPVC, error) {
	var imageData imageInfoFromPVC

	c := f.ClientSet.CoreV1()
	pvc, err := c.PersistentVolumeClaims(pvcNamespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
	if err != nil {
		return imageData, fmt.Errorf("failed to get pvc: %w", err)
	}

	pv, err := c.PersistentVolumes().Get(context.TODO(), pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return imageData, fmt.Errorf("failed to get pv: %w", err)
	}

	imageIDRegex := regexp.MustCompile(`(\w+\-?){5}$`)
	imageID := imageIDRegex.FindString(pv.Spec.CSI.VolumeHandle)

	imageData = imageInfoFromPVC{
		imageID:         imageID,
		imageName:       fmt.Sprintf("csi-vol-%s", imageID),
		csiVolumeHandle: pv.Spec.CSI.VolumeHandle,
		pvName:          pv.Name,
	}

	return imageData, nil
}

func getImageMeta(rbdImageSpec, metaKey string, f *framework.Framework) (string, error) {
	cmd := fmt.Sprintf("rbd image-meta get %s %s", rbdImageSpec, metaKey)
	stdOut, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return "", err
	}
	if stdErr != "" {
		return strings.TrimSpace(stdOut), fmt.Errorf("%s", stdErr)
	}

	return strings.TrimSpace(stdOut), nil
}

// validateImageOwner checks the "csi.volume.owner" key on the image journal
// and verifies that the owner is set to the namespace where the PVC is
// created.
func validateImageOwner(pvcPath string, f *framework.Framework) error {
	const ownerKey = "csi.volume.owner"

	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}
	pvc.Namespace = f.UniqueName
	pvc.Name = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return err
	}

	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	stdOut, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf(
			"rados %s getomapval csi.volume.%s %s",
			rbdOptions(defaultRBDPool),
			imageData.imageID,
			ownerKey),
		rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to getomapval %v", stdErr)
	}

	if radosNamespace != "" {
		framework.Logf(
			"found image journal %s in pool %s namespace %s",
			"csi.volume."+imageData.imageID,
			defaultRBDPool,
			radosNamespace)
	} else {
		framework.Logf("found image journal %s in pool %s", "csi.volume."+imageData.imageID, defaultRBDPool)
	}

	if !strings.Contains(stdOut, pvc.Namespace) {
		return fmt.Errorf("%q does not contain %q: %s", ownerKey, pvc.Namespace, stdOut)
	}

	return deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
}

func logErrors(f *framework.Framework, msg string, wgErrs []error) int {
	failures := 0
	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			framework.Logf("%s (%s%d): %v", msg, f.UniqueName, i, err)
			failures++
		}
	}

	return failures
}

func validateCloneInDifferentPool(f *framework.Framework, snapshotPool, cloneSc, destImagePool string) error {
	var wg sync.WaitGroup
	totalCount := 10
	wgErrs := make([]error, totalCount)
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return fmt.Errorf("failed to load PVC: %w", err)
	}

	pvc.Namespace = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create PVC: %w", err)
	}
	validateRBDImageCount(f, 1, defaultRBDPool)
	snap := getSnapshot(snapshotPath)
	snap.Namespace = f.UniqueName
	snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
	// create snapshot
	wg.Add(totalCount)
	for i := 0; i < totalCount; i++ {
		go func(n int, s snapapi.VolumeSnapshot) {
			s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
			wgErrs[n] = createSnapshot(&s, deployTimeout)
			wg.Done()
		}(i, snap)
	}
	wg.Wait()

	if failed := logErrors(f, "failed to create snapshot", wgErrs); failed != 0 {
		return fmt.Errorf("creating snapshots failed, %d errors were logged", failed)
	}

	// delete parent pvc
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete PVC: %w", err)
	}

	// validate the rbd images created for snapshots
	validateRBDImageCount(f, totalCount, snapshotPool)

	pvcClone, err := loadPVC(pvcClonePath)
	if err != nil {
		return fmt.Errorf("failed to load PVC: %w", err)
	}
	appClone, err := loadApp(appClonePath)
	if err != nil {
		return fmt.Errorf("failed to load application: %w", err)
	}
	pvcClone.Namespace = f.UniqueName
	// if request is to create clone with different storage class
	if cloneSc != "" {
		pvcClone.Spec.StorageClassName = &cloneSc
	}
	appClone.Namespace = f.UniqueName
	pvcClone.Spec.DataSource.Name = fmt.Sprintf("%s%d", f.UniqueName, 0)
	// create multiple PVCs from same snapshot
	wg.Add(totalCount)
	for i := 0; i < totalCount; i++ {
		go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
			wg.Done()
		}(i, *pvcClone, *appClone)
	}
	wg.Wait()

	if failed := logErrors(f, "failed to create PVC and application", wgErrs); failed != 0 {
		return fmt.Errorf("creating PVCs and applications failed, %d errors were logged", failed)
	}

	// total images in pool is total snaps + total clones
	if destImagePool == snapshotPool {
		totalCloneCount := totalCount + totalCount
		validateRBDImageCount(f, totalCloneCount, snapshotPool)
	} else {
		// if clones are created in different pool we will have only rbd images of
		// count equal to totalCount
		validateRBDImageCount(f, totalCount, destImagePool)
	}
	wg.Add(totalCount)
	// delete clone and app
	for i := 0; i < totalCount; i++ {
		go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			p.Spec.DataSource.Name = name
			wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
			wg.Done()
		}(i, *pvcClone, *appClone)
	}
	wg.Wait()

	if failed := logErrors(f, "failed to delete PVC and application", wgErrs); failed != 0 {
		return fmt.Errorf("deleting PVCs and applications failed, %d errors were logged", failed)
	}

	if destImagePool == snapshotPool {
		// as we have deleted all clones total images in pool is total snaps
		validateRBDImageCount(f, totalCount, snapshotPool)
	} else {
		// we have deleted all clones
		validateRBDImageCount(f, 0, destImagePool)
	}

	wg.Add(totalCount)
	// delete snapshot
	for i := 0; i < totalCount; i++ {
		go func(n int, s snapapi.VolumeSnapshot) {
			s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
			wgErrs[n] = deleteSnapshot(&s, deployTimeout)
			wg.Done()
		}(i, snap)
	}
	wg.Wait()

	if failed := logErrors(f, "failed to delete snapshot", wgErrs); failed != 0 {
		return fmt.Errorf("deleting snapshots failed, %d errors were logged", failed)
	}
	// validate all pools are empty
	validateRBDImageCount(f, 0, snapshotPool)
	validateRBDImageCount(f, 0, defaultRBDPool)
	validateRBDImageCount(f, 0, destImagePool)

	return nil
}

type encryptionValidateFunc func(pvcPath, appPath string, kms kmsConfig, f *framework.Framework) error

func validateEncryptedPVCAndAppBinding(pvcPath, appPath string, kms kmsConfig, f *framework.Framework) error {
	pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
	if err != nil {
		return err
	}
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	rbdImageSpec := imageSpec(defaultRBDPool, imageData.imageName)
	err = validateEncryptedImage(f, rbdImageSpec, imageData.pvName, app.Name)
	if err != nil {
		return err
	}

	if kms != noKMS && kms.canGetPassphrase() {
		// check new passphrase created
		_, stdErr := kms.getPassphrase(f, imageData.csiVolumeHandle)
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
		stdOut, _ := kms.getPassphrase(f, imageData.csiVolumeHandle)
		if stdOut != "" {
			return fmt.Errorf("passphrase found in vault while should be deleted: %s", stdOut)
		}
	}

	if kms != noKMS && kms.canVerifyKeyDestroyed() {
		destroyed, msg := kms.verifyKeyDestroyed(f, imageData.csiVolumeHandle)
		if !destroyed {
			return fmt.Errorf("passphrased was not destroyed: %s", msg)
		} else if msg != "" {
			framework.Logf("passphrase destroyed, but message returned: %s", msg)
		}
	}

	return nil
}

func validateEncryptedFilesystemAndAppBinding(pvcPath, appPath string, kms kmsConfig, f *framework.Framework) error {
	pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
	if err != nil {
		return err
	}
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	rbdImageSpec := imageSpec(defaultRBDPool, imageData.imageName)
	err = validateEncryptedFilesystem(f, rbdImageSpec, imageData.pvName, app.Name)
	if err != nil {
		return err
	}

	if kms != noKMS && kms.canGetPassphrase() {
		// check new passphrase created
		_, stdErr := kms.getPassphrase(f, imageData.csiVolumeHandle)
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
		stdOut, _ := kms.getPassphrase(f, imageData.csiVolumeHandle)
		if stdOut != "" {
			return fmt.Errorf("passphrase found in vault while should be deleted: %s", stdOut)
		}
	}

	if kms != noKMS && kms.canVerifyKeyDestroyed() {
		destroyed, msg := kms.verifyKeyDestroyed(f, imageData.csiVolumeHandle)
		if !destroyed {
			return fmt.Errorf("passphrased was not destroyed: %s", msg)
		} else if msg != "" {
			framework.Logf("passphrase destroyed, but message returned: %s", msg)
		}
	}

	return nil
}

type validateFunc func(f *framework.Framework, pvc *v1.PersistentVolumeClaim, app *v1.Pod) error

// noPVCValidation can be used to pass to validatePVCClone when no extra
// validation of the PVC is needed.
var noPVCValidation validateFunc

type imageValidateFunc func(f *framework.Framework, rbdImageSpec, pvName, appName string) error

func isEncryptedPVC(
	f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	app *v1.Pod,
	validateFunc imageValidateFunc,
) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}
	rbdImageSpec := imageSpec(defaultRBDPool, imageData.imageName)

	return validateFunc(f, rbdImageSpec, imageData.pvName, app.Name)
}

func isBlockEncryptedPVC(f *framework.Framework, pvc *v1.PersistentVolumeClaim, app *v1.Pod) error {
	return isEncryptedPVC(f, pvc, app, validateEncryptedImage)
}

func isFileEncryptedPVC(f *framework.Framework, pvc *v1.PersistentVolumeClaim, app *v1.Pod) error {
	return isEncryptedPVC(f, pvc, app, validateEncryptedFilesystem)
}

// validateEncryptedImage verifies that the RBD image is encrypted. The
// following checks are performed:
// - Metadata of the image should be set with the encryption state;
// - The pvc should be mounted by a pod, so the filesystem type can be fetched.
func validateEncryptedImage(f *framework.Framework, rbdImageSpec, pvName, appName string) error {
	encryptedState, err := getImageMeta(rbdImageSpec, "rbd.csi.ceph.com/encrypted", f)
	if err != nil {
		return err
	}
	if encryptedState != "encrypted" {
		return fmt.Errorf("%v not equal to encrypted", encryptedState)
	}

	pod, err := f.ClientSet.CoreV1().Pods(f.UniqueName).Get(context.TODO(), appName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod %q in namespace %q: %w", appName, f.UniqueName, err)
	}
	volumeMountPath := fmt.Sprintf(
		"/var/lib/kubelet/pods/%s/volumes/kubernetes.io~csi/%s/mount",
		pod.UID,
		pvName)
	selector, err := getDaemonSetLabelSelector(f, cephCSINamespace, rbdDaemonsetName)
	if err != nil {
		return fmt.Errorf("failed to get labels: %w", err)
	}
	mountType, err := getMountType(selector, volumeMountPath, f)
	if err != nil {
		return err
	}
	if mountType != "crypt" {
		return fmt.Errorf("%v not equal to crypt", mountType)
	}

	return nil
}

func validateEncryptedFilesystem(f *framework.Framework, rbdImageSpec, pvName, appName string) error {
	pod, err := f.ClientSet.CoreV1().Pods(f.UniqueName).Get(context.TODO(), appName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod %q in namespace %q: %w", appName, f.UniqueName, err)
	}
	volumeMountPath := fmt.Sprintf(
		"/var/lib/kubelet/pods/%s/volumes/kubernetes.io~csi/%s/mount",
		pod.UID,
		pvName)

	selector, err := getDaemonSetLabelSelector(f, cephCSINamespace, rbdDaemonsetName)
	if err != nil {
		return fmt.Errorf("failed to get labels: %w", err)
	}
	opt := metav1.ListOptions{
		LabelSelector: selector,
	}
	cmd := fmt.Sprintf("lsattr -la %s | grep -E '%s/.\\s+Encrypted'", volumeMountPath, volumeMountPath)
	_, _, err = execCommandInContainer(f, cmd, cephCSINamespace, "csi-rbdplugin", &opt)
	if err != nil {
		cmd = fmt.Sprintf("lsattr -lRa %s", volumeMountPath)
		stdOut, stdErr, listErr := execCommandInContainer(f, cmd, cephCSINamespace, "csi-rbdplugin", &opt)
		if listErr == nil {
			return fmt.Errorf("error checking file encrypted attribute of %q. listing filesystem+attrs: %s %s",
				volumeMountPath, stdOut, stdErr)
		}

		return fmt.Errorf("error checking file encrypted attribute: %w", err)
	}

	mountType, err := getMountType(selector, volumeMountPath, f)
	if err != nil {
		return err
	}
	if mountType == "crypt" {
		return fmt.Errorf("mount type of %q is %v suggesting that the block device was encrypted,"+
			" when it must not have been", volumeMountPath, mountType)
	}

	return nil
}

func listRBDImages(f *framework.Framework, pool string) ([]string, error) {
	var imgInfos []string

	stdout, stdErr, err := execCommandInToolBoxPod(f,
		fmt.Sprintf("rbd ls --format=json %s", rbdOptions(pool)), rookNamespace)
	if err != nil {
		return imgInfos, err
	}
	if stdErr != "" {
		return imgInfos, fmt.Errorf("failed to list images %v", stdErr)
	}

	err = json.Unmarshal([]byte(stdout), &imgInfos)
	if err != nil {
		return imgInfos, err
	}

	return imgInfos, nil
}

func deleteBackingRBDImage(f *framework.Framework, pvc *v1.PersistentVolumeClaim) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("rbd rm %s %s", rbdOptions(defaultRBDPool), imageData.imageName)
	_, _, err = execCommandInToolBoxPod(f, cmd, rookNamespace)

	return err
}

// rbdDuImage contains the disk-usage statistics of an RBD image.
//
//nolint:unused // required for reclaimspace e2e.
type rbdDuImage struct {
	Name            string `json:"name"`
	ProvisionedSize uint64 `json:"provisioned_size"`
	UsedSize        uint64 `json:"used_size"`
}

// rbdDuImageList contains the list of images returned by 'rbd du'.
//
//nolint:unused // required for reclaimspace e2e.
type rbdDuImageList struct {
	Images []*rbdDuImage `json:"images"`
}

// getRbdDu runs 'rbd du' on the RBD image and returns a rbdDuImage struct with
// the result.
//
//nolint:deadcode,unused // required for reclaimspace e2e.
func getRbdDu(f *framework.Framework, pvc *v1.PersistentVolumeClaim) (*rbdDuImage, error) {
	rdil := rbdDuImageList{}

	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf("rbd du --format=json %s %s", rbdOptions(defaultRBDPool), imageData.imageName)
	stdout, _, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal([]byte(stdout), &rdil)
	if err != nil {
		return nil, err
	}

	for _, image := range rdil.Images {
		if image.Name == imageData.imageName {
			return image, nil
		}
	}

	return nil, fmt.Errorf("image %s not found", imageData.imageName)
}

// sparsifyBackingRBDImage runs `rbd sparsify` on the RBD image. Once done, all
// data blocks that contain zeros are discarded/trimmed/unmapped and do not
// take up any space anymore. This can be used to verify that an empty, but
// allocated (with zerofill) extents have been released.
//
//nolint:deadcode,unused // required for reclaimspace e2e.
func sparsifyBackingRBDImage(f *framework.Framework, pvc *v1.PersistentVolumeClaim) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("rbd sparsify %s %s", rbdOptions(defaultRBDPool), imageData.imageName)
	_, _, err = execCommandInToolBoxPod(f, cmd, rookNamespace)

	return err
}

func deletePool(name string, cephFS bool, f *framework.Framework) error {
	cmds := []string{}
	if cephFS {
		//nolint:dupword // "ceph osd pool delete" requires the pool 2x
		//
		// ceph fs fail
		// ceph fs rm myfs --yes-i-really-mean-it
		// ceph osd pool delete myfs-metadata myfs-metadata
		// --yes-i-really-mean-it
		// ceph osd pool delete myfs-replicated myfs-replicated
		// --yes-i-really-mean-it
		cmds = append(cmds, fmt.Sprintf("ceph fs fail %s", name),
			fmt.Sprintf("ceph fs rm %s --yes-i-really-mean-it", name),
			fmt.Sprintf("ceph osd pool delete %s-metadata %s-metadata --yes-i-really-really-mean-it", name, name),
			fmt.Sprintf("ceph osd pool delete %s-replicated %s-replicated --yes-i-really-really-mean-it", name, name))
	} else {
		//nolint:dupword // "ceph osd pool delete" requires the pool 2x
		//
		// ceph osd pool delete replicapool replicapool
		// --yes-i-really-mean-it
		cmds = append(cmds, fmt.Sprintf("ceph osd pool delete %s %s --yes-i-really-really-mean-it", name, name))
	}

	for _, cmd := range cmds {
		// discard stdErr as some commands prints warning in strErr
		_, _, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
		if err != nil {
			return err
		}
	}

	return nil
}

func createPool(f *framework.Framework, name string) error {
	var (
		pgCount = 128
		size    = 1
	)
	// ceph osd pool create replicapool
	cmd := fmt.Sprintf("ceph osd pool create %s %d", name, pgCount)
	_, _, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return err
	}
	// ceph osd pool set replicapool size 1
	cmd = fmt.Sprintf("ceph osd pool set %s size %d --yes-i-really-mean-it", name, size)
	_, _, err = execCommandInToolBoxPod(f, cmd, rookNamespace)

	return err
}

func getPVCImageInfoInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) (string, error) {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return "", err
	}

	stdOut, stdErr, err := execCommandInToolBoxPod(f,
		fmt.Sprintf("rbd info %s", imageSpec(pool, imageData.imageName)), rookNamespace)
	if err != nil {
		return "", err
	}
	if stdErr != "" {
		return "", fmt.Errorf("failed to get rbd info %v", stdErr)
	}

	if radosNamespace != "" {
		framework.Logf("found image %s in pool %s namespace %s", imageData.imageName, pool, radosNamespace)
	} else {
		framework.Logf("found image %s in pool %s", imageData.imageName, pool)
	}

	return stdOut, nil
}

func checkPVCImageInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) error {
	_, err := getPVCImageInfoInPool(f, pvc, pool)

	return err
}

func checkPVCDataPoolForImageInPool(
	f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	pool, dataPool string,
) error {
	stdOut, err := getPVCImageInfoInPool(f, pvc, pool)
	if err != nil {
		return err
	}

	if !strings.Contains(stdOut, "data_pool: "+dataPool) {
		return fmt.Errorf("missing data pool value in image info, got info (%s)", stdOut)
	}

	return nil
}

func checkPVCImageJournalInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	_, stdErr, err := execCommandInToolBoxPod(f,
		fmt.Sprintf("rados listomapkeys %s csi.volume.%s", rbdOptions(pool), imageData.imageID), rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to listomapkeys %v", stdErr)
	}

	if radosNamespace != "" {
		framework.Logf(
			"found image journal %s in pool %s namespace %s",
			"csi.volume."+imageData.imageID,
			pool,
			radosNamespace)
	} else {
		framework.Logf("found image journal %s in pool %s", "csi.volume."+imageData.imageID, pool)
	}

	return nil
}

func checkPVCCSIJournalInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	_, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf(
			"rados getomapval %s csi.volumes.default csi.volume.%s",
			rbdOptions(pool),
			imageData.pvName,
		),
		rookNamespace,
	)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("error getting fsid %v", stdErr)
	}

	if radosNamespace != "" {
		framework.Logf(
			"found CSI journal entry %s in pool %s namespace %s",
			"csi.volume."+imageData.pvName,
			pool,
			radosNamespace)
	} else {
		framework.Logf("found CSI journal entry %s in pool %s", "csi.volume."+imageData.pvName, pool)
	}

	return nil
}

// deleteJournalInfoInPool deletes all omap data regarding pvc.
func deleteJournalInfoInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) error {
	if err := deletePVCImageJournalInPool(f, pvc, pool); err != nil {
		return err
	}

	return deletePVCCSIJournalInPool(f, pvc, pool)
}

func deletePVCImageJournalInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	_, stdErr, err := execCommandInToolBoxPod(f,
		fmt.Sprintf("rados rm %s csi.volume.%s", rbdOptions(pool), imageData.imageID), rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf(
			"failed to remove omap %s csi.volume.%s: %v",
			rbdOptions(pool),
			imageData.imageID,
			stdErr)
	}

	return nil
}

func deletePVCCSIJournalInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	_, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf(
			"rados rmomapkey %s csi.volumes.default csi.volume.%s",
			rbdOptions(pool),
			imageData.pvName),
		rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf(
			"failed to remove %s csi.volumes.default csi.volume.%s: %v",
			rbdOptions(pool),
			imageData.imageID,
			stdErr)
	}

	return nil
}

// trashInfo contains the image details in trash.
type trashInfo struct {
	Name string `json:"name"`
}

// listRBDImagesInTrash lists images in the trash.
func listRBDImagesInTrash(f *framework.Framework, poolName string) ([]trashInfo, error) {
	var trashInfos []trashInfo

	stdout, stdErr, err := execCommandInToolBoxPod(f,
		fmt.Sprintf("rbd trash ls --format=json %s", rbdOptions(poolName)), rookNamespace)
	if err != nil {
		return trashInfos, err
	}
	if stdErr != "" {
		return trashInfos, fmt.Errorf("failed to list images in trash %v", stdErr)
	}

	err = json.Unmarshal([]byte(stdout), &trashInfos)
	if err != nil {
		return trashInfos, err
	}

	return trashInfos, nil
}

func waitToRemoveImagesFromTrash(f *framework.Framework, poolName string, t int) error {
	var errReason error
	timeout := time.Duration(t) * time.Minute
	err := wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(_ context.Context) (bool, error) {
		imagesInTrash, err := listRBDImagesInTrash(f, poolName)
		if err != nil {
			return false, err
		}
		if len(imagesInTrash) == 0 {
			return true, nil
		}
		errReason = fmt.Errorf("found %d images found in trash. Image details %v", len(imagesInTrash), imagesInTrash)
		framework.Logf(errReason.Error())

		return false, nil
	})

	if wait.Interrupted(err) {
		err = errReason
	}

	return err
}

// imageInfo strongly typed JSON spec for image info.
type imageInfo struct {
	Name        string `json:"name"`
	StripeUnit  int    `json:"stripe_unit"`
	StripeCount int    `json:"stripe_count"`
	ObjectSize  int    `json:"object_size"`
}

// getImageInfo queries rbd about the given image and returns its metadata, and returns
// error if provided image is not found.
func getImageInfo(f *framework.Framework, imageName, poolName string) (imageInfo, error) {
	// rbd --format=json info [image-spec | snap-spec]
	var imgInfo imageInfo

	stdOut, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf("rbd info %s %s --format json", rbdOptions(poolName), imageName),
		rookNamespace)
	if err != nil {
		return imgInfo, fmt.Errorf("failed to get rbd info: %w", err)
	}
	if stdErr != "" {
		return imgInfo, fmt.Errorf("failed to get rbd info: %v", stdErr)
	}
	err = json.Unmarshal([]byte(stdOut), &imgInfo)
	if err != nil {
		return imgInfo, fmt.Errorf("unmarshal failed: %w. raw buffer response: %s",
			err, stdOut)
	}

	return imgInfo, nil
}

// validateStripe validate the stripe count, stripe unit and object size of the
// image.
func validateStripe(f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	stripeUnit,
	stripeCount,
	objectSize int,
) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	imgInfo, err := getImageInfo(f, imageData.imageName, defaultRBDPool)
	if err != nil {
		return err
	}

	if imgInfo.ObjectSize != objectSize {
		return fmt.Errorf("objectSize %d does not match expected %d", imgInfo.ObjectSize, objectSize)
	}

	if imgInfo.StripeUnit != stripeUnit {
		return fmt.Errorf("stripeUnit %d does not match expected %d", imgInfo.StripeUnit, stripeUnit)
	}

	if imgInfo.StripeCount != stripeCount {
		return fmt.Errorf("stripeCount %d does not match expected %d", imgInfo.StripeCount, stripeCount)
	}

	return nil
}
