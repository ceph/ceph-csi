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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	scv1 "k8s.io/api/storage/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"

	"github.com/ceph/ceph-csi/internal/util/cryptsetup"
	"github.com/ceph/ceph-csi/pkg/util/kernel"
)

//nolint:mnd // numbers specify Kernel versions.
var nbdResizeSupport = []kernel.KernelVersion{
	{
		Version:      5,
		PatchLevel:   3,
		SubLevel:     0,
		ExtraVersion: 0,
		Distribution: "",
		Backport:     false,
	}, // standard 5.3+ versions
}

//nolint:mnd // numbers specify Kernel versions.
var fastDiffSupport = []kernel.KernelVersion{
	{
		Version:      5,
		PatchLevel:   3,
		SubLevel:     0,
		ExtraVersion: 0,
		Distribution: "",
		Backport:     false,
	}, // standard 5.3+ versions
}

//nolint:mnd // numbers specify Kernel versions.
var deepFlattenSupport = []kernel.KernelVersion{
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
//nolint:mnd // numbers specify Kernel versions.
var nbdZeroIOtimeoutSupport = []kernel.KernelVersion{
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

// supportsVolumeAttributesClass returns true when both the Kubernetes cluster
// (>= 1.34) and the deployed ceph-csi (>= 3.17) support VolumeAttributesClass.
func supportsVolumeAttributesClass(c kubernetes.Interface, f *framework.Framework, daemonsetName string) bool {
	return k8sVersionGreaterEquals(c, 1, 34) &&
		cephcsiVersionGreaterEquals(f, daemonsetName, rbdContainerName, 3, 17)
}

func createRBDStorageClass(
	c kubernetes.Interface,
	f *framework.Framework,
	name string,
	scOptions, parameters map[string]string,
	policy v1.PersistentVolumeReclaimPolicy,
) error {
	scPath := rbdExamplePath + "/" + "storageclass.yaml"
	sc, err := getStorageClass(scPath)
	if err != nil {
		return fmt.Errorf("failed to get sc: %w", err)
	}
	if name != "" {
		sc.Name = name
	}
	// add pool only if topologyConstrainedPools is not present
	if _, ok := parameters["topologyConstrainedPools"]; !ok {
		sc.Parameters["pool"] = defaultRBDPool
	}
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"] = rbdProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-name"] = rbdProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/controller-publish-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/controller-publish-secret-name"] = rbdProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/node-stage-secret-name"] = rbdNodePluginSecretName

	sc.Parameters["csi.storage.k8s.io/node-publish-secret-namespace"] = cephCSINamespace
	sc.Parameters["csi.storage.k8s.io/node-publish-secret-name"] = rbdNodePluginSecretName

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

// createKRBDStorageClassWithModifySecret creates a StorageClass configured for
// krbd mounter (default) with controller-modify-secret for VolumeAttributesClass support.
// This is required for testing cgroup v2 QoS with VAC modification.
func createKRBDStorageClassWithModifySecret(f *framework.Framework, scName string) error {
	deletePolicy := v1.PersistentVolumeReclaimDelete

	parameters := map[string]string{
		// Empty mounter = krbd (default mounter)
		"mounter": "",
		// controller-modify-secret required for VAC modification
		"csi.storage.k8s.io/controller-modify-secret-namespace": cephCSINamespace,
		"csi.storage.k8s.io/controller-modify-secret-name":      rbdProvisionerSecretName,
	}

	return createRBDStorageClass(f.ClientSet, f, scName, nil, parameters, deletePolicy)
}

func createRadosNamespace(f *framework.Framework) error {
	stdOut, stdErr, err := execCommandInToolBoxPod(f,
		"rbd namespace ls --pool="+defaultRBDPool, rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("error listing rbd namespace %v", stdErr)
	}
	if !strings.Contains(stdOut, radosNamespace) {
		_, stdErr, err = execCommandInToolBoxPod(f,
			"rbd namespace create "+rbdOptions(defaultRBDPool), rookNamespace)
		if err != nil {
			return err
		}
		if stdErr != "" {
			return fmt.Errorf("error creating rbd namespace %v", stdErr)
		}
	}
	stdOut, stdErr, err = execCommandInToolBoxPod(f,
		"rbd namespace ls --pool="+rbdTopologyPool, rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("error listing rbd namespace %v", stdErr)
	}

	if !strings.Contains(stdOut, radosNamespace) {
		_, stdErr, err = execCommandInToolBoxPod(f,
			"rbd namespace create "+rbdOptions(rbdTopologyPool), rookNamespace)
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

type snapInfo struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Protected string `json:"protected"`
}

func (s snapInfo) String() string {
	return fmt.Sprintf("{id: %d, name: %s, protected: %s}", s.ID, s.Name, s.Protected)
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

	prefix := "csi-vol-"
	if pv.Spec.CSI.VolumeAttributes != nil {
		if val, ok := pv.Spec.CSI.VolumeAttributes["volumeNamePrefix"]; ok {
			prefix = val
		}
	}

	imageData = imageInfoFromPVC{
		imageID:         imageID,
		imageName:       prefix + imageID,
		csiVolumeHandle: pv.Spec.CSI.VolumeHandle,
		pvName:          pv.Name,
	}

	return imageData, nil
}

func getEncryptionOptionsFromPV(pvcNamespace, pvcName string, f *framework.Framework) (*cryptsetup.EncryptionOptions, error) {
	var options *cryptsetup.EncryptionOptions
	c := f.ClientSet.CoreV1()
	pvc, err := c.PersistentVolumeClaims(pvcNamespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pvc: %w", err)
	}
	pv, err := c.PersistentVolumes().Get(context.TODO(), pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pv: %w", err)
	}
	if pv.Spec.CSI == nil || pv.Spec.CSI.VolumeAttributes == nil {
		return nil, nil
	}
	initOptions := func() {
		// Only init options when key present
		if options == nil {
			options = &cryptsetup.EncryptionOptions{}
		}
	}
	for key, value := range pv.Spec.CSI.VolumeAttributes {
		switch key {
		case "encryptionCipher":
			initOptions()
			err = options.SetCipher(value)

		case "encryptionKeySize":
			initOptions()
			err = options.SetKeySize(value)

		case "integrityMode":
			initOptions()
			err = options.SetIntegrityMode(value)

		}
		if err != nil {
			return nil, fmt.Errorf("failed to set volume attribute %q: %w", key, err)
		}
	}

	return options, nil
}

func getLuksStatusFromMount(pvName, appName string, f *framework.Framework) (luksStatus *cryptsetup.LuksStatus, err error) {
	pod, err := f.ClientSet.CoreV1().Pods(f.UniqueName).Get(context.TODO(), appName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %q in namespace %q: %w", appName, f.UniqueName, err)
	}
	volumeMountPath := fmt.Sprintf(
		"/var/lib/kubelet/pods/%s/volumes/kubernetes.io~csi/%s/mount",
		pod.UID,
		pvName)
	selector, err := getDaemonSetLabelSelector(f, cephCSINamespace, rbdDeployment.getDaemonsetName())
	if err != nil {
		return nil, fmt.Errorf("failed to get labels: %w", err)
	}
	luksStatus, err = getLuksStatus(selector, volumeMountPath, f)
	if err != nil {
		return nil, err
	}

	return luksStatus, nil
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

func logErrors(uniqueName, msg string, wgErrs []error) int {
	failures := 0
	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			framework.Logf("%s (%s%d): %v", msg, uniqueName, i, err)
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
	uniqueName := uuid.NewString()
	wg.Add(totalCount)
	for i := range totalCount {
		go func(n int, s snapapi.VolumeSnapshot) {
			s.Name = fmt.Sprintf("%s-%d", uniqueName, n)
			wgErrs[n] = createSnapshot(&s, deployTimeout)
			wg.Done()
		}(i, snap)
	}
	wg.Wait()

	if failed := logErrors("failed to create snapshot", uniqueName, wgErrs); failed != 0 {
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
	pvcClone.Spec.DataSource.Name = fmt.Sprintf("%s-%d", uniqueName, 0)
	// create multiple PVCs from same snapshot
	wg.Add(totalCount)
	for i := range totalCount {
		go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s-%d", uniqueName, n)
			wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
			wg.Done()
		}(i, *pvcClone, *appClone)
	}
	wg.Wait()

	if failed := logErrors("failed to create PVC and application", uniqueName, wgErrs); failed != 0 {
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
	for i := range totalCount {
		go func(n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s-%d", uniqueName, n)
			p.Spec.DataSource.Name = name
			wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
			wg.Done()
		}(i, *pvcClone, *appClone)
	}
	wg.Wait()

	if failed := logErrors("failed to delete PVC and application", uniqueName, wgErrs); failed != 0 {
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
	for i := range totalCount {
		go func(n int, s snapapi.VolumeSnapshot) {
			s.Name = fmt.Sprintf("%s-%d", uniqueName, n)
			wgErrs[n] = deleteSnapshot(&s, deployTimeout)
			wg.Done()
		}(i, snap)
	}
	wg.Wait()

	if failed := logErrors("failed to delete snapshot", uniqueName, wgErrs); failed != 0 {
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
	options, err := getEncryptionOptionsFromPV(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return fmt.Errorf("failed to get encryption options for pv %q: %w",
			imageData.pvName, err)
	}
	// if options is nil then no encryption options have been set in the PV.
	if options != nil {
		luksStatus, err := getLuksStatusFromMount(imageData.pvName, app.Name, f)
		if err != nil {
			_ = deletePVCAndApp("", f, pvc, app)
			return fmt.Errorf("failed to get luks status for pv %q (app: %s): %w",
				imageData.pvName, app.Name, err)
		}
		if luksStatus == nil {
			return fmt.Errorf("state mismatch for pv %q: encryption options were set, but the volume is not encrypted (luks status is nil)",
				imageData.pvName)
		}
		isEqual, err := options.Equal(*luksStatus)
		if err != nil {
			return fmt.Errorf("comparison between EncryptionOptions %+v and luks status %+v failed", options, *luksStatus)
		}
		if !isEqual {
			return fmt.Errorf("encryption mismatch for pv %q: options do not match on-disk status. Desired: %+v, Actual: %+v",
				imageData.pvName, *options, *luksStatus)
		}
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

	headerSizeValue, err := getImageMeta(rbdImageSpec, "rbd.csi.ceph.com/luks2HeaderSize", f)
	if err != nil {
		return err
	}
	headerSize, parseErr := strconv.ParseUint(headerSizeValue, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("failed to parse luks2 header size for %s: %w", rbdImageSpec, parseErr)
	}
	if headerSize != cryptsetup.Luks2HeaderSize {
		return fmt.Errorf("luks2 header size for %s is %d, expected %d", rbdImageSpec, headerSize, cryptsetup.Luks2HeaderSize)
	}

	pod, err := f.ClientSet.CoreV1().Pods(f.UniqueName).Get(context.TODO(), appName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod %q in namespace %q: %w", appName, f.UniqueName, err)
	}
	volumeMountPath := fmt.Sprintf(
		"/var/lib/kubelet/pods/%s/volumes/kubernetes.io~csi/%s/mount",
		pod.UID,
		pvName)
	selector, err := getDaemonSetLabelSelector(f, cephCSINamespace, rbdDeployment.getDaemonsetName())
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

	selector, err := getDaemonSetLabelSelector(f, cephCSINamespace, rbdDeployment.getDaemonsetName())
	if err != nil {
		return fmt.Errorf("failed to get labels: %w", err)
	}
	opt := metav1.ListOptions{
		LabelSelector: selector,
	}
	cmd := fmt.Sprintf("lsattr -la %s | grep -E '%s/.\\s+Encrypted'", volumeMountPath, volumeMountPath)
	_, _, err = execCommandInContainer(f, cmd, cephCSINamespace, "csi-rbdplugin", &opt)
	if err != nil {
		cmd = "lsattr -lRa " + volumeMountPath
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

// librbdSupportsVolumeGroupSnapshot checks for the rbd_group_snap_get_info in
// librbd.so.* in a ceph-csi container. If this function is available,
// VolumeGroupSnapshot support is available.
func librbdSupportsVolumeGroupSnapshot(f *framework.Framework) (bool, error) {
	selector, err := getDaemonSetLabelSelector(f, cephCSINamespace, rbdDeployment.getDaemonsetName())
	if err != nil {
		return false, fmt.Errorf("failed to get labels: %w", err)
	}
	opt := metav1.ListOptions{
		LabelSelector: selector,
	}

	// run a shell command (to expand the * in the filename), return 0 on stdout when successful
	cmd := "sh -c 'grep -q rbd_group_snap_get_info /lib64/librbd.so.*; echo $?'"
	stdout, _, err := execCommandInContainer(f, cmd, cephCSINamespace, "csi-rbdplugin", &opt)
	if err != nil {
		return false, fmt.Errorf("error checking for rbd_group_snap_get_info in /lib64/librbd.so.*: %w", err)
	}

	return strings.TrimSpace(stdout) == "0", nil
}

func listRBDSnapshots(f *framework.Framework, pool, image string) ([]snapInfo, error) {
	var snapInfos []snapInfo
	command := fmt.Sprintf("rbd snap ls --format=json %s %s", rbdOptions(pool), image)
	stdout, stdErr, err := execCommandInToolBoxPod(f, command, rookNamespace)
	if err != nil {
		return snapInfos, err
	}
	if stdErr != "" {
		return snapInfos, fmt.Errorf("failed to list RBD snapshots %v", stdErr)
	}

	err = json.Unmarshal([]byte(stdout), &snapInfos)
	if err != nil {
		return snapInfos, err
	}

	return snapInfos, nil
}

func listRBDImages(f *framework.Framework, pool string) ([]string, error) {
	var imgInfos []string

	stdout, stdErr, err := execCommandInToolBoxPod(f,
		"rbd ls --format=json "+rbdOptions(pool), rookNamespace)
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
//nolint:unused // Unused code will be used in future.
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
//nolint:unused // Unused code will be used in future.
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
		cmds = append(cmds, "ceph fs fail "+name,
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
	// ceph osd pool create name
	cmd := fmt.Sprintf("ceph osd pool create %s %d --yes-i-really-mean-it", name, pgCount)
	_, _, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return err
	}
	// ceph osd pool set name size 1
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
		"rbd info "+imageSpec(pool, imageData.imageName), rookNamespace)
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
		"rbd trash ls --format=json "+rbdOptions(poolName), rookNamespace)
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
		framework.Logf("%v", errReason.Error())

		return false, nil
	})

	if wait.Interrupted(err) {
		err = errReason
	}

	return err
}

// imageInfo strongly typed JSON spec for image info.
type imageInfo struct {
	Name        string   `json:"name"`
	StripeUnit  int      `json:"stripe_unit"`
	StripeCount int      `json:"stripe_count"`
	ObjectSize  int      `json:"object_size"`
	DataPool    string   `json:"data_pool"`
	Features    []string `json:"features"`
}

// getImageInfo queries rbd about the given image and returns its metadata, and returns
// error if provided image is not found.
func getImageInfo(f *framework.Framework, imageName, poolName string) (string, error) {
	// rbd --format=json info [image-spec | snap-spec]
	stdOut, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf("rbd info %s %s --format json", rbdOptions(poolName), imageName),
		rookNamespace)
	if err != nil {
		return stdOut, fmt.Errorf("failed to get rbd info: %w", err)
	}
	if stdErr != "" {
		return stdOut, fmt.Errorf("failed to get rbd info: %v", stdErr)
	}

	return stdOut, nil
}

// getImageStatus queries rbd about the given image and returns its metadata, and returns
// error if provided image is not found.
func getImageStatus(f *framework.Framework, imageName, poolName string) (string, error) {
	// rbd --format=json status [image-spec | snap-spec]
	stdOut, stdErr, err := execCommandInToolBoxPod(
		f,
		fmt.Sprintf("rbd status %s %s --format json", rbdOptions(poolName), imageName),
		rookNamespace)
	if err != nil {
		return stdOut, fmt.Errorf("error retrieving rbd status: %w", err)
	}
	if stdErr != "" {
		return stdOut, fmt.Errorf("failed to get rbd info: %v", stdErr)
	}

	return stdOut, nil
}

// validateStripe validate the stripe count, stripe unit and object size of the
// image.
func validateStripe(f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	stripeUnit,
	stripeCount,
	objectSize int,
) error {
	var imgInfo imageInfo

	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	imgInfoStr, err := getImageInfo(f, imageData.imageName, defaultRBDPool)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(imgInfoStr), &imgInfo)
	if err != nil {
		return fmt.Errorf("unmarshal failed: %w. raw buffer response: %s", err, imgInfoStr)
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

// validateImageFeatures checks that the given RBD image has all the expected
// image features enabled.
func validateImageFeatures(
	f *framework.Framework,
	imageName, pool string,
	expectedFeatures []string,
) error {
	var imgInfo imageInfo

	imgInfoStr, err := getImageInfo(f, imageName, pool)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(imgInfoStr), &imgInfo)
	if err != nil {
		return fmt.Errorf("unmarshal failed: %w. raw buffer response: %s", err, imgInfoStr)
	}

	actualSet := make(map[string]bool, len(imgInfo.Features))
	for _, feat := range imgInfo.Features {
		actualSet[feat] = true
	}

	for _, want := range expectedFeatures {
		if !actualSet[want] {
			return fmt.Errorf("image %s missing expected feature %q, got features: %v",
				imageName, want, imgInfo.Features)
		}
	}

	return nil
}

func validateQOS(f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	wants map[string]string,
) error {
	metadataConfPrefix := "conf_"

	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	rbdImageSpec := imageSpec(defaultRBDPool, imageData.imageName)
	for k, v := range wants {
		qosVal, err := getImageMeta(rbdImageSpec, metadataConfPrefix+k, f)
		if err != nil {
			return err
		}
		if qosVal != v {
			return fmt.Errorf("%s: %s does not match expected %s", k, qosVal, v)
		}
	}

	return nil
}

// validateCgroupQoS validates cgroup v2 QoS parameters stored in RBD image metadata.
// The wants map uses VolumeAttributesClass parameter names as keys:
//   - "maxReadIops", "maxWriteIops"
//   - "maxReadBps", "maxWriteBps"
func validateCgroupQoS(f *framework.Framework,
	pvc *v1.PersistentVolumeClaim,
	wants map[string]string,
) error {
	// Cgroup QoS metadata keys are prefixed to prevent clone/snapshot propagation
	metadataPrefix := ".rbd.csi.ceph.com/cgroup_qos_"

	// Map VolumeAttributesClass parameter names to metadata keys
	paramToMetadataKey := map[string]string{
		"maxReadIops":  metadataPrefix + "max_read_iops",
		"maxWriteIops": metadataPrefix + "max_write_iops",
		"maxReadBps":   metadataPrefix + "max_read_bps",
		"maxWriteBps":  metadataPrefix + "max_write_bps",
	}

	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	rbdImageSpec := imageSpec(defaultRBDPool, imageData.imageName)
	for param, metadataKey := range paramToMetadataKey {
		expectedValue, shouldExist := wants[param]

		actualValue, err := getImageMeta(rbdImageSpec, metadataKey, f)
		if shouldExist {
			if err != nil {
				return fmt.Errorf("failed to get %s: %w", metadataKey, err)
			}
			if actualValue != expectedValue {
				return fmt.Errorf("%s: got %q, want %q", param, actualValue, expectedValue)
			}
		} else if err == nil {
			return fmt.Errorf("%s should be absent but found value %q", param, actualValue)
		}
	}

	return nil
}

// testIOEnforcement validates that I/O operations respect cgroup QoS limits.
// Uses dd with direct I/O to measure write throughput and compares against expected limits.
// Allows 20% tolerance margin to account for measurement variance and kernel overhead.
func testIOEnforcement(f *framework.Framework,
	pod *v1.Pod,
	volumePath string,
	isBlock bool,
	limits map[string]string,
) error {
	// Parse expected write bandwidth limit
	maxWriteBpsStr, ok := limits["maxWriteBps"]
	if !ok {
		return fmt.Errorf("maxWriteBps not specified in limits")
	}

	maxWriteBps, err := strconv.ParseInt(maxWriteBpsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse maxWriteBps: %w", err)
	}

	containerName := pod.Spec.Containers[0].Name

	// For block devices, write directly to the device path.
	// For filesystem volumes, create a test file under the mount path.
	target := volumePath
	if !isBlock {
		target = volumePath + "/qos_test_file"
	}

	// Run dd write test: 100MB with direct I/O.
	// Using direct I/O (oflag=direct) bypasses page cache and hits cgroup limits.
	// dd outputs: "N bytes copied, T s, R MB/s" — parse the rate and unit to compute bytes/sec.
	cmd := fmt.Sprintf(
		"dd if=/dev/zero of=%s bs=1M count=100 oflag=direct 2>&1 | "+
			"awk -F', ' '/copied/ {print $(NF)}' | "+
			"awk '{rate=$1; unit=$2; m=1; "+
			"if(unit==\"GB/s\") m=1e9; "+
			"else if(unit==\"MB/s\") m=1e6; "+
			"else if(unit==\"kB/s\") m=1e3; "+
			"else if(unit==\"GiB/s\") m=1073741824; "+
			"else if(unit==\"MiB/s\") m=1048576; "+
			"else if(unit==\"KiB/s\") m=1024; "+
			"printf \"%%.0f\\n\", rate*m}'",
		target)

	framework.Logf("running I/O enforcement test with command: %s", cmd)
	output, stdErr, err := execCommandInContainerByPodName(f, cmd, pod.Namespace, pod.Name, containerName)
	if err != nil {
		return fmt.Errorf("failed to run dd test: %w (stderr: %s)", err, stdErr)
	}

	// Parse throughput from dd output
	actualBpsStr := strings.TrimSpace(output)
	actualBps, err := strconv.ParseInt(actualBpsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse throughput from output %q: %w", output, err)
	}

	// Allow 20% tolerance for measurement variance
	toleranceMargin := 1.20
	maxAllowedBps := int64(float64(maxWriteBps) * toleranceMargin)

	framework.Logf("I/O enforcement test: actual=%d bps, limit=%d bps, max_allowed=%d bps (%.0f%% tolerance)",
		actualBps, maxWriteBps, maxAllowedBps, (toleranceMargin-1)*100)

	if actualBps > maxAllowedBps {
		return fmt.Errorf("write throughput %d bps exceeds limit %d bps (with %.0f%% tolerance margin)",
			actualBps, maxWriteBps, (toleranceMargin-1)*100)
	}

	framework.Logf("I/O enforcement validated: actual throughput within limits")

	return nil
}

// createMultiPVCPod creates a pod with multiple RBD volumes attached.
// Critical for testing updateIOMaxForDevice read-modify-write logic.
// When a pod has multiple volumes, each volume's QoS must be written to the same
// io.max file without overwriting others.
func createMultiPVCPod(f *framework.Framework,
	pvcs []*v1.PersistentVolumeClaim,
	podName string,
	isBlock bool,
) (*v1.Pod, error) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: f.UniqueName,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:            "app-container",
					Image:           "quay.io/centos/centos:latest",
					ImagePullPolicy: v1.PullIfNotPresent,
					Command:         []string{"/bin/sleep", "infinity"},
				},
			},
		},
	}

	// Add volumes and volume mounts/devices
	for i, pvc := range pvcs {
		volumeName := fmt.Sprintf("vol-%d", i)

		// Add volume source
		pod.Spec.Volumes = append(pod.Spec.Volumes, v1.Volume{
			Name: volumeName,
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.Name,
				},
			},
		})

		if isBlock {
			// Block mode - add volume device
			devicePath := fmt.Sprintf("/dev/xvd%c", 'a'+i)
			pod.Spec.Containers[0].VolumeDevices = append(pod.Spec.Containers[0].VolumeDevices,
				v1.VolumeDevice{
					Name:       volumeName,
					DevicePath: devicePath,
				})
		} else {
			// Filesystem mode - add volume mount
			mountPath := fmt.Sprintf("/mnt/vol-%d", i)
			pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts,
				v1.VolumeMount{
					Name:      volumeName,
					MountPath: mountPath,
				})
		}
	}

	err := createApp(f.ClientSet, pod, deployTimeout)
	if err != nil {
		return nil, err
	}

	return pod, nil
}

func validateDataPool(f *framework.Framework, imageName, poolName string, wants string) error {
	var imgInfo imageInfo
	imgInfoStr, err := getImageInfo(f, imageName, poolName)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(imgInfoStr), &imgInfo)
	if err != nil {
		return fmt.Errorf("unmarshal failed: %w. raw buffer response: %s", err, imgInfoStr)
	}

	if imgInfo.DataPool != wants {
		return fmt.Errorf("unexpected data_pool: got %q, want %q", imgInfo.DataPool, wants)
	}

	return nil
}

// setImageMeta sets a metadata key-value pair on an RBD image.
func setImageMeta(rbdImageSpec, metaKey, metaValue string, f *framework.Framework) error {
	cmd := fmt.Sprintf("rbd image-meta set %s %s %q", rbdImageSpec, metaKey, metaValue)
	_, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to set image metadata: %s", stdErr)
	}

	return nil
}

// removeImageMeta removes a metadata key from an RBD image.
func removeImageMeta(rbdImageSpec, metaKey string, f *framework.Framework) error {
	cmd := fmt.Sprintf("rbd image-meta remove %s %s", rbdImageSpec, metaKey)
	_, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to remove image metadata: %s", stdErr)
	}

	return nil
}

// validateServiceAccountVolumeRestriction tests that volume access can be
// restricted to a specific Kubernetes service account. It creates a PVC,
// sets the given saMetadataKey metadata on the backing RBD image, then
// verifies:
//   - A pod using the allowed service account can mount the volume.
//   - A pod using a different service account is rejected with PermissionDenied.
//
// The pool parameter specifies the RBD pool for the image. If scName is
// non-nil the storage class is set on the PVC.
func validateServiceAccountVolumeRestriction(
	pvcPath, appPath, saMetadataKey, pool string,
	scName *string,
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
	if scName != nil {
		pvc.Spec.StorageClassName = scName
	}
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

	// Get the RBD image info from the PVC.
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return fmt.Errorf("failed to get image info from PVC: %w", err)
	}
	rbdImageSpec := imageSpec(pool, imageData.imageName)

	// Set the service account restriction metadata on the RBD image.
	err = setImageMeta(rbdImageSpec, saMetadataKey, allowedSA, f)
	if err != nil {
		return fmt.Errorf("failed to set service account metadata: %w", err)
	}
	defer func() {
		// Clean up the metadata regardless of test outcome.
		delErr := removeImageMeta(rbdImageSpec, saMetadataKey, f)
		if delErr != nil {
			framework.Logf("failed to remove service account metadata: %v", delErr)
		}
	}()

	// Verify the metadata was set correctly.
	saValue, err := getImageMeta(rbdImageSpec, saMetadataKey, f)
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

	// Test 3: Update metadata to a comma-separated list of allowed service accounts and verify access.
	err = setImageMeta(rbdImageSpec, saMetadataKey, allowedSA+","+thirdSA, f)
	if err != nil {
		return fmt.Errorf("failed to set comma-separated service account metadata: %w", err)
	}

	// Verify the metadata was set correctly.
	saValue, err = getImageMeta(rbdImageSpec, saMetadataKey, f)
	if err != nil {
		return fmt.Errorf("failed to get service account metadata: %w", err)
	}
	if saValue != (allowedSA + "," + thirdSA) {
		return fmt.Errorf("expected service account metadata %q, got %q",
			(allowedSA + "," + thirdSA), saValue)
	}

	// Wait for volume to be fully detached before updating metadata
	// to ensure fresh ControllerPublishVolume call with new metadata.
	err = waitForPVCVolumeAttachmentsCleanup(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to wait for volume detachment: %w", err)
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

func createRBDVolumeAttributesClass(
	c kubernetes.Interface,
	name string,
	params map[string]string,
) error {
	if name == "" {
		return errors.New("name is required for VolumeAttributesClass")
	}
	// Create a clean VolumeAttributesClass instead of loading from template
	// to avoid mixing NBD QoS parameters from the template with cgroup QoS parameters.
	vac := scv1.VolumeAttributesClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		DriverName: "rbd.csi.ceph.com",
		Parameters: make(map[string]string),
	}

	// Add provided parameters
	if params != nil {
		maps.Copy(vac.Parameters, params)
	}

	timeout := time.Duration(deployTimeout) * time.Minute

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		_, createErr := c.StorageV1().VolumeAttributesClasses().Create(ctx, &vac, metav1.CreateOptions{})
		if createErr != nil {
			framework.Logf("error creating VolumeAttributesClass %q: %v", vac.Name, createErr)
			if apierrs.IsAlreadyExists(createErr) {
				return true, nil
			}
			if isRetryableAPIError(createErr) {
				return false, nil
			}

			return false, fmt.Errorf("failed to create VolumeAttributesClass %q: %w", vac.Name, createErr)
		}

		return true, nil
	})
}

func deleteRBDVolumeAttributesClass(
	c kubernetes.Interface,
	f *framework.Framework,
	name string,
) error {
	vacPath := fmt.Sprintf("%s/%s", rbdExamplePath, "volumeattributesclass.yaml")
	vac, err := getVolumeAttributesClass(vacPath)
	if err != nil {
		return err
	}
	if name != "" {
		vac.Name = name
	}

	timeout := time.Duration(deployTimeout) * time.Minute

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		err = c.StorageV1().VolumeAttributesClasses().Delete(ctx, vac.Name, metav1.DeleteOptions{})
		if err != nil {
			framework.Logf("error deleting VolumeAttributesClass %q: %v", vac.Name, err)
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to delete VolumeAttributesClass %q: %w", vac.Name, err)
		}

		return true, nil
	})
}

func modifyPVCVolumeAttributesClass(
	c kubernetes.Interface,
	pvc *v1.PersistentVolumeClaim,
	vacName string,
) error {
	ctx := context.TODO()
	pvcName := pvc.Name
	pvcNamespace := pvc.Namespace
	updatedPVC, err := getPersistentVolumeClaim(c, pvcNamespace, pvcName)
	if err != nil {
		return fmt.Errorf("error fetching pvc %q with %w", pvcName, err)
	}

	timeout := time.Duration(deployTimeout) * time.Minute
	updatedPVC.Spec.VolumeAttributesClassName = &vacName
	_, err = c.CoreV1().
		PersistentVolumeClaims(updatedPVC.Namespace).
		Update(ctx, updatedPVC, metav1.UpdateOptions{})
	Expect(err).ShouldNot(HaveOccurred())

	return wait.PollUntilContextTimeout(ctx, poll, timeout, true, func(ctx context.Context) (bool, error) {
		updatedPVC, err = c.CoreV1().PersistentVolumeClaims(pvcNamespace).Get(ctx, pvcName, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}

			return false, fmt.Errorf("failed to get pvc: %w", err)
		}

		if updatedPVC.Status.CurrentVolumeAttributesClassName == nil ||
			*updatedPVC.Status.CurrentVolumeAttributesClassName != vacName {
			return false, nil
		}

		return true, nil
	})
}

// validateIOMax reads the io.max file from the pod's cgroup on the node
// and validates that it contains the expected QoS values for the volume's device.
//
// The function:
//  1. Finds the csi-rbdplugin pod on the same node as the app pod.
//  2. Discovers the RBD device major:minor from the node.
//  3. Locates the pod's cgroup path (probing systemd and cgroupfs layouts).
//  4. Reads io.max and verifies the device's line matches the expected QoS values.
func validateIOMax(
	f *framework.Framework,
	appPod *v1.Pod,
	pvc *v1.PersistentVolumeClaim,
	wants map[string]string,
	daemonsetName string,
) error {
	podUID := string(appPod.UID)
	nodeName := appPod.Spec.NodeName

	pluginPodName, err := getDaemonsetPodOnNode(f, daemonsetName, nodeName, cephCSINamespace)
	if err != nil {
		return fmt.Errorf("failed to find csi-rbdplugin pod on node %s: %w", nodeName, err)
	}

	// Get the PV to find the volume handle for the staging path.
	_, pv, err := getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get PV for PVC %s: %w", pvc.Name, err)
	}

	deviceID, err := getDeviceIDOnNode(f, pluginPodName, pv)
	if err != nil {
		return fmt.Errorf("failed to get device ID: %w", err)
	}

	framework.Logf("device ID for volume %s: %s", pv.Name, deviceID)

	ioMaxContent, err := readPodCgroupIOMax(f, pluginPodName, podUID)
	if err != nil {
		return fmt.Errorf("failed to read io.max for pod %s: %w", appPod.Name, err)
	}

	framework.Logf("io.max content for pod %s:\n%s", appPod.Name, ioMaxContent)

	return verifyIOMaxContent(ioMaxContent, deviceID, wants)
}

// getDeviceIDOnNode gets the major:minor device ID for the RBD volume on the node
// by reading the stash file from the staging path in the csi-rbdplugin container.
func getDeviceIDOnNode(
	f *framework.Framework,
	pluginPodName string,
	pv *v1.PersistentVolume,
) (string, error) {
	volumeHandle := pv.Spec.CSI.VolumeHandle

	// The kubelet staging directory layout differs for filesystem and block volumes:
	//   Filesystem: /var/lib/kubelet/plugins/kubernetes.io/csi/rbd.csi.ceph.com/<sha256>/globalmount/
	//   Block:      /var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/staging/<pv-name>/
	hash := sha256.Sum256([]byte(volumeHandle))
	dirName := hex.EncodeToString(hash[:])
	candidates := []string{
		fmt.Sprintf(
			"/var/lib/kubelet/plugins/kubernetes.io/csi/rbd.csi.ceph.com/%s/globalmount/image-meta.json",
			dirName),
		fmt.Sprintf(
			"/var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/staging/%s/image-meta.json",
			pv.Name),
	}

	var stashContent string
	var stashPath string
	for _, candidate := range candidates {
		cmd := "cat " + candidate + " 2>/dev/null"
		content, _, err := execCommandInContainerByPodName(
			f, cmd, cephCSINamespace, pluginPodName, rbdContainerName)
		if err == nil && strings.TrimSpace(content) != "" {
			stashContent = content
			stashPath = candidate

			break
		}
	}

	if stashContent == "" {
		return "", fmt.Errorf("stash file not found for volume %s (tried %v)", volumeHandle, candidates)
	}

	framework.Logf("found stash file at %s", stashPath)

	var stash struct {
		Device string `json:"device"`
	}
	if err := json.Unmarshal([]byte(stashContent), &stash); err != nil {
		return "", fmt.Errorf("failed to parse stash file: %w", err)
	}

	if stash.Device == "" {
		return "", fmt.Errorf("device path not found in stash file %s", stashPath)
	}

	// stat the device to get major:minor
	cmd := fmt.Sprintf("stat -c '%%t:%%T' %s", stash.Device)
	hexID, _, err := execCommandInContainerByPodName(
		f, cmd, cephCSINamespace, pluginPodName, rbdContainerName)
	if err != nil {
		return "", fmt.Errorf("failed to stat device %s: %w", stash.Device, err)
	}

	hexID = strings.TrimSpace(hexID)
	parts := strings.SplitN(hexID, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("unexpected stat output for device %s: %q", stash.Device, hexID)
	}

	major, err := strconv.ParseInt(parts[0], 16, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse major number %q: %w", parts[0], err)
	}

	minor, err := strconv.ParseInt(parts[1], 16, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse minor number %q: %w", parts[1], err)
	}

	return fmt.Sprintf("%d:%d", major, minor), nil
}

// readPodCgroupIOMax reads the io.max file from the pod's cgroup path on the node
// by probing both systemd and cgroupfs cgroup driver layouts.
func readPodCgroupIOMax(
	f *framework.Framework,
	pluginPodName string,
	podUID string,
) (string, error) {
	uidUnderscore := strings.ReplaceAll(podUID, "-", "_")

	// Candidate cgroup paths matching production code (podCgroupCandidates).
	candidates := []string{
		// systemd cgroup driver
		"/sys/fs/cgroup/kubepods.slice/kubepods-pod" + uidUnderscore + ".slice",
		"/sys/fs/cgroup/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod" + uidUnderscore + ".slice",
		"/sys/fs/cgroup/kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-pod" + uidUnderscore + ".slice",
		// cgroupfs cgroup driver
		"/sys/fs/cgroup/kubepods/pod" + podUID,
		"/sys/fs/cgroup/kubepods/burstable/pod" + podUID,
		"/sys/fs/cgroup/kubepods/besteffort/pod" + podUID,
	}

	for _, candidate := range candidates {
		ioMaxPath := candidate + "/io.max"
		cmd := "cat " + ioMaxPath + " 2>/dev/null"

		content, _, err := execCommandInContainerByPodName(
			f, cmd, cephCSINamespace, pluginPodName, rbdContainerName)
		if err == nil && strings.TrimSpace(content) != "" {
			framework.Logf("found io.max at %s", ioMaxPath)

			return strings.TrimSpace(content), nil
		}
	}

	return "", fmt.Errorf("io.max not found for pod UID %s in any candidate cgroup path", podUID)
}

// verifyIOMaxContent parses io.max content and verifies that the line for the
// given device ID contains the expected QoS values.
func verifyIOMaxContent(content, deviceID string, wants map[string]string) error {
	// Map VAC parameter names to io.max field names.
	paramToField := map[string]string{
		"maxReadIops":  "riops",
		"maxWriteIops": "wiops",
		"maxReadBps":   "rbps",
		"maxWriteBps":  "wbps",
	}

	// Find the line for our device.
	var deviceLine string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, deviceID+" ") || strings.HasPrefix(line, deviceID+"\t") {
			deviceLine = line

			break
		}
	}

	if deviceLine == "" {
		return fmt.Errorf("device %s not found in io.max content:\n%s", deviceID, content)
	}

	// Parse key=value pairs from the device line.
	actual := make(map[string]string)
	fields := strings.Fields(deviceLine)
	for _, field := range fields[1:] {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) == 2 {
			actual[parts[0]] = parts[1]
		}
	}

	// Verify each expected value.
	for param, expected := range wants {
		fieldName, ok := paramToField[param]
		if !ok {
			continue
		}

		got, exists := actual[fieldName]
		if !exists {
			return fmt.Errorf("io.max field %s (%s) not found for device %s", fieldName, param, deviceID)
		}

		if got != expected {
			return fmt.Errorf("io.max %s (%s) for device %s: got %q, want %q",
				fieldName, param, deviceID, got, expected)
		}
	}

	return nil
}
