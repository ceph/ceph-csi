package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	v1 "k8s.io/api/core/v1"
	scv1 "k8s.io/api/storage/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

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

func createRBDStorageClass(c kubernetes.Interface, f *framework.Framework, name string, scOptions, parameters map[string]string, policy v1.PersistentVolumeReclaimPolicy) error {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "storageclass.yaml")
	sc, err := getStorageClass(scPath)
	if err != nil {
		return nil
	}
	if name != "" {
		sc.Name = name
	}
	sc.Parameters["pool"] = defaultRBDPool
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"] = rookNamespace
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"] = rbdProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"] = rookNamespace
	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-name"] = rbdProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"] = rookNamespace
	sc.Parameters["csi.storage.k8s.io/node-stage-secret-name"] = rbdNodePluginSecretName

	fsID, stdErr, err := execCommandInToolBoxPod(f, "ceph fsid", rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("error getting fsid %v", stdErr)
	}
	// remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")

	sc.Parameters["clusterID"] = fsID
	for k, v := range parameters {
		sc.Parameters[k] = v
	}
	sc.Namespace = cephCSINamespace

	if scOptions["volumeBindingMode"] == "WaitForFirstConsumer" {
		value := scv1.VolumeBindingWaitForFirstConsumer
		sc.VolumeBindingMode = &value
	}

	// comma separated mount options
	if opt, ok := scOptions[rbdmountOptions]; ok {
		mOpt := strings.Split(opt, ",")
		sc.MountOptions = append(sc.MountOptions, mOpt...)
	}
	sc.ReclaimPolicy = &policy
	_, err = c.StorageV1().StorageClasses().Create(context.TODO(), &sc, metav1.CreateOptions{})
	return err
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

func createRBDSecret(c kubernetes.Interface, f *framework.Framework) error {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "secret.yaml")
	sc, err := getSecret(scPath)
	if err != nil {
		return err
	}
	adminKey, stdErr, err := execCommandInToolBoxPod(f, "ceph auth get-key client.admin", rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("error getting admin key %v", stdErr)
	}
	sc.StringData["userID"] = adminUser
	sc.StringData["userKey"] = adminKey
	sc.Namespace = cephCSINamespace
	_, err = c.CoreV1().Secrets(cephCSINamespace).Create(context.TODO(), &sc, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	err = updateSecretForEncryption(c)
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
		return imageData, err
	}

	pv, err := c.PersistentVolumes().Get(context.TODO(), pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return imageData, err
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
		return strings.TrimSpace(stdOut), fmt.Errorf(stdErr)
	}
	return strings.TrimSpace(stdOut), nil
}

// nolint:gocyclo // reduce complexity
func validateSnapshotInDifferentPool(f *framework.Framework, snapshotPool, cloneSc, destImagePool string) error {
	var wg sync.WaitGroup
	totalCount := 10
	wgErrs := make([]error, totalCount)
	wg.Add(totalCount)
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return fmt.Errorf("failed to load PVC with error %v", err)
	}

	pvc.Namespace = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create PVC with error %v", err)
	}
	validateRBDImageCount(f, 1, defaultRBDPool)
	snap := getSnapshot(snapshotPath)
	snap.Namespace = f.UniqueName
	snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
	// create snapshot
	for i := 0; i < totalCount; i++ {
		go func(w *sync.WaitGroup, n int, s v1beta1.VolumeSnapshot) {
			s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
			wgErrs[n] = createSnapshot(&s, deployTimeout)
			w.Done()
		}(&wg, i, snap)
	}
	wg.Wait()

	failed := 0
	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to create snapshot (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		return fmt.Errorf("creating snapshots failed, %d errors were logged", failed)
	}

	// delete parent pvc
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete PVC with error %v", err)
	}

	// validate the rbd images created for snapshots
	validateRBDImageCount(f, totalCount, snapshotPool)

	pvcClone, err := loadPVC(pvcClonePath)
	if err != nil {
		return fmt.Errorf("failed to load PVC with error %v", err)
	}
	appClone, err := loadApp(appClonePath)
	if err != nil {
		return fmt.Errorf("failed to load application with error %v", err)
	}
	pvcClone.Namespace = f.UniqueName
	// if request is to create clone with different sc
	if cloneSc != "" {
		pvcClone.Spec.StorageClassName = &cloneSc
	}
	appClone.Namespace = f.UniqueName
	pvcClone.Spec.DataSource.Name = fmt.Sprintf("%s%d", f.UniqueName, 0)

	// create multiple PVC from same snapshot
	wg.Add(totalCount)
	for i := 0; i < totalCount; i++ {
		go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
			w.Done()
		}(&wg, i, *pvcClone, *appClone)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to create PVC and application (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
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
		go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			p.Spec.DataSource.Name = name
			wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
			w.Done()
		}(&wg, i, *pvcClone, *appClone)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to delete PVC and application (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
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
		go func(w *sync.WaitGroup, n int, s v1beta1.VolumeSnapshot) {
			s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
			wgErrs[n] = deleteSnapshot(&s, deployTimeout)
			w.Done()
		}(&wg, i, snap)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to delete snapshot (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		return fmt.Errorf("deleting snapshots failed, %d errors were logged", failed)
	}

	// validate all pools are empty
	validateRBDImageCount(f, 0, snapshotPool)
	validateRBDImageCount(f, 0, defaultRBDPool)
	validateRBDImageCount(f, 0, destImagePool)
	return nil
}

func validateEncryptedPVCAndAppBinding(pvcPath, appPath, kms string, f *framework.Framework) error {
	pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
	if err != nil {
		return err
	}
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}
	rbdImageSpec := imageSpec(defaultRBDPool, imageData.imageName)
	encryptedState, err := getImageMeta(rbdImageSpec, ".rbd.csi.ceph.com/encrypted", f)
	if err != nil {
		return err
	}
	if encryptedState != "encrypted" {
		return fmt.Errorf("%v not equal to encrypted", encryptedState)
	}

	volumeMountPath := app.Spec.Containers[0].VolumeMounts[0].MountPath
	mountType, err := getMountType(app.Name, app.Namespace, volumeMountPath, f)
	if err != nil {
		return err
	}
	if mountType != "crypt" {
		return fmt.Errorf("%v not equal to crypt", mountType)
	}

	if kms == "vault" {
		// check new passphrase created
		_, stdErr := readVaultSecret(imageData.csiVolumeHandle, f)
		if stdErr != "" {
			return fmt.Errorf("failed to read passphrase from vault: %s", stdErr)
		}
	}

	err = deletePVCAndApp("", f, pvc, app)
	if err != nil {
		return err
	}

	if kms == "vault" {
		// check new passphrase created
		stdOut, _ := readVaultSecret(imageData.csiVolumeHandle, f)
		if stdOut != "" {
			return fmt.Errorf("passphrase found in vault while should be deleted: %s", stdOut)
		}
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

func deletePool(name string, cephfs bool, f *framework.Framework) error {
	var cmds = []string{}
	if cephfs {
		// ceph fs fail
		// ceph fs rm myfs --yes-i-really-mean-it
		// ceph osd pool delete myfs-metadata myfs-metadata
		// --yes-i-really-mean-it
		// ceph osd pool delete myfs-data0 myfs-data0
		// --yes-i-really-mean-it
		cmds = append(cmds, fmt.Sprintf("ceph fs fail %s", name),
			fmt.Sprintf("ceph fs rm %s --yes-i-really-mean-it", name),
			fmt.Sprintf("ceph osd pool delete %s-metadata %s-metadata --yes-i-really-really-mean-it", name, name),
			fmt.Sprintf("ceph osd pool delete %s-data0 %s-data0 --yes-i-really-really-mean-it", name, name))
	} else {
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
	// ceph osd pool create replicapool
	cmd := fmt.Sprintf("ceph osd pool create %s", name)
	_, _, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
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
		e2elog.Logf("found image %s in pool %s namespace %s", imageData.imageName, pool, radosNamespace)
	} else {
		e2elog.Logf("found image %s in pool %s", imageData.imageName, pool)
	}

	return stdOut, nil
}

func checkPVCImageInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) error {
	_, err := getPVCImageInfoInPool(f, pvc, pool)

	return err
}

func checkPVCDataPoolForImageInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool, dataPool string) error {
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
		e2elog.Logf("found image journal %s in pool %s namespace %s", "csi.volume."+imageData.imageID, pool, radosNamespace)
	} else {
		e2elog.Logf("found image journal %s in pool %s", "csi.volume."+imageData.imageID, pool)
	}

	return nil
}

func checkPVCCSIJournalInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	_, stdErr, err := execCommandInToolBoxPod(f,
		fmt.Sprintf("rados getomapval %s csi.volumes.default csi.volume.%s", rbdOptions(pool), imageData.pvName), rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("error getting fsid %v", stdErr)
	}

	if radosNamespace != "" {
		e2elog.Logf("found CSI journal entry %s in pool %s namespace %s", "csi.volume."+imageData.pvName, pool, radosNamespace)
	} else {
		e2elog.Logf("found CSI journal entry %s in pool %s", "csi.volume."+imageData.pvName, pool)
	}

	return nil
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
		return fmt.Errorf("failed to remove omap %s csi.volume.%s with error %v", rbdOptions(pool), imageData.imageID, stdErr)
	}

	return nil
}

func deletePVCCSIJournalInPool(f *framework.Framework, pvc *v1.PersistentVolumeClaim, pool string) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	_, stdErr, err := execCommandInToolBoxPod(f,
		fmt.Sprintf("rados rmomapkey %s csi.volumes.default csi.volume.%s", rbdOptions(pool), imageData.pvName), rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to remove %s csi.volumes.default csi.volume.%s with error %v", rbdOptions(pool), imageData.imageID, stdErr)
	}

	return nil
}
