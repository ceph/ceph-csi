package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

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

func createRBDStorageClass(c kubernetes.Interface, f *framework.Framework, scOptions, parameters map[string]string, policy v1.PersistentVolumeReclaimPolicy) error {
	scPath := fmt.Sprintf("%s/%s", rbdExamplePath, "storageclass.yaml")
	sc, err := getStorageClass(scPath)
	if err != nil {
		return nil
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

	stdOut, stdErr, err := execCommandInToolBoxPod(f,
		fmt.Sprintf("rados %s getomapval csi.volume.%s %s", rbdOptions(defaultRBDPool), imageData.imageID, ownerKey), rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to getomapval %v", stdErr)
	}

	if radosNamespace != "" {
		e2elog.Logf("found image journal %s in pool %s namespace %s", "csi.volume."+imageData.imageID, defaultRBDPool, radosNamespace)
	} else {
		e2elog.Logf("found image journal %s in pool %s", "csi.volume."+imageData.imageID, defaultRBDPool)
	}

	if !strings.Contains(stdOut, pvc.Namespace) {
		return fmt.Errorf("%q does not contain %q: %s", ownerKey, pvc.Namespace, stdOut)
	}

	return deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
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

func listRBDImages(f *framework.Framework) ([]string, error) {
	var imgInfos []string

	stdout, stdErr, err := execCommandInToolBoxPod(f,
		fmt.Sprintf("rbd ls --format=json %s", rbdOptions(defaultRBDPool)), rookNamespace)
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
