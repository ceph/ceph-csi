package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	adminUser = "admin"
)

// validateSubvolumegroup validates whether subvolumegroup is present.
func validateSubvolumegroup(f *framework.Framework, subvolgrp string) error {
	cmd := fmt.Sprintf("ceph fs subvolumegroup getpath myfs %s", subvolgrp)
	stdOut, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to getpath for subvolumegroup %s with error %v", subvolgrp, stdErr)
	}
	expectedGrpPath := "/volumes/" + subvolgrp
	stdOut = strings.TrimSpace(stdOut)
	if stdOut != expectedGrpPath {
		return fmt.Errorf("error unexpected group path. Found: %s", stdOut)
	}
	return nil
}

func createCephfsStorageClass(c kubernetes.Interface, f *framework.Framework, enablePool bool, clusterID string) error {
	scPath := fmt.Sprintf("%s/%s", cephfsExamplePath, "storageclass.yaml")
	sc, err := getStorageClass(scPath)
	if err != nil {
		return err
	}
	sc.Parameters["fsName"] = "myfs"
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-namespace"] = rookNamespace
	sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"] = cephfsProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-namespace"] = rookNamespace
	sc.Parameters["csi.storage.k8s.io/controller-expand-secret-name"] = cephfsProvisionerSecretName

	sc.Parameters["csi.storage.k8s.io/node-stage-secret-namespace"] = rookNamespace
	sc.Parameters["csi.storage.k8s.io/node-stage-secret-name"] = cephfsNodePluginSecretName

	if enablePool {
		sc.Parameters["pool"] = "myfs-data0"
	}
	fsID, stdErr, err := execCommandInToolBoxPod(f, "ceph fsid", rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("error getting fsid %v", stdErr)
	}
	// remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")
	if clusterID != "" {
		fsID = clusterID
	}
	sc.Namespace = cephCSINamespace
	sc.Parameters["clusterID"] = fsID
	_, err = c.StorageV1().StorageClasses().Create(context.TODO(), &sc, metav1.CreateOptions{})
	return err
}

func createCephfsSecret(c kubernetes.Interface, f *framework.Framework) error {
	scPath := fmt.Sprintf("%s/%s", cephfsExamplePath, "secret.yaml")
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
	sc.StringData["adminID"] = adminUser
	sc.StringData["adminKey"] = adminKey
	delete(sc.StringData, "userID")
	delete(sc.StringData, "userKey")
	sc.Namespace = cephCSINamespace
	_, err = c.CoreV1().Secrets(cephCSINamespace).Create(context.TODO(), &sc, metav1.CreateOptions{})
	return err
}

func deleteBackingCephFSVolume(f *framework.Framework, pvc *v1.PersistentVolumeClaim) error {
	imageData, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	if err != nil {
		return err
	}

	_, stdErr, err := execCommandInToolBoxPod(f, "ceph fs subvolume rm myfs "+imageData.imageName+" "+subvolumegroup, rookNamespace)
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
	stdout, stdErr, err := execCommandInToolBoxPod(f, fmt.Sprintf("ceph fs subvolume ls %s --group_name=%s --format=json", filesystem, groupname), rookNamespace)
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
