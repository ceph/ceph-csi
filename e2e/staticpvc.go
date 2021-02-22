package e2e

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
)

func getStaticPV(name, volName, size, secretName, secretNS, sc, driverName string, blockPV bool, options map[string]string) *v1.PersistentVolume {
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimRetain,
			Capacity: v1.ResourceList{
				v1.ResourceStorage: resource.MustParse(size),
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:           driverName,
					VolumeHandle:     volName,
					ReadOnly:         false,
					VolumeAttributes: options,
					NodeStageSecretRef: &v1.SecretReference{
						Name:      secretName,
						Namespace: secretNS,
					},
				},
			},
			StorageClassName: sc,
			AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
		},
	}

	if blockPV {
		volumeMode := v1.PersistentVolumeBlock
		pv.Spec.VolumeMode = &volumeMode
	} else {
		volumeMode := v1.PersistentVolumeFilesystem
		pv.Spec.VolumeMode = &volumeMode
	}

	return pv
}

func getStaticPVC(name, pvName, size, ns, sc string, blockPVC bool) *v1.PersistentVolumeClaim {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse(size),
				},
			},
			AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			VolumeName:       pvName,
			StorageClassName: &sc,
		},
	}
	if blockPVC {
		volumeMode := v1.PersistentVolumeBlock
		pvc.Spec.VolumeMode = &volumeMode
	} else {
		volumeMode := v1.PersistentVolumeFilesystem
		pvc.Spec.VolumeMode = &volumeMode
	}

	return pvc
}

func validateRBDStaticPV(f *framework.Framework, appPath string, isBlock bool) error {
	opt := make(map[string]string)
	var (
		rbdImageName = "test-static-pv"
		pvName       = "pv-name"
		pvcName      = "pvc-name"
		namespace    = f.UniqueName
		// minikube creates default class in cluster, we need to set dummy
		// storageclass on PV and PVC to avoid storageclass name mismatch
		sc = "storage-class"
	)

	c := f.ClientSet

	fsID, e, err := execCommandInToolBoxPod(f, "ceph fsid", rookNamespace)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to get fsid from ceph cluster %s", e)
	}
	// remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")
	size := "4Gi"
	// create rbd image
	cmd := fmt.Sprintf("rbd create %s --size=%d --image-feature=layering %s", rbdImageName, 4096, rbdOptions(defaultRBDPool))

	_, e, err = execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to create rbd image %s", e)
	}
	opt["clusterID"] = fsID
	opt["imageFeatures"] = "layering"
	opt["pool"] = defaultRBDPool
	opt["staticVolume"] = "true"
	if radosNamespace != "" {
		opt["radosNamespace"] = radosNamespace
	}

	pv := getStaticPV(pvName, rbdImageName, size, rbdNodePluginSecretName, cephCSINamespace, sc, "rbd.csi.ceph.com", isBlock, opt)

	_, err = c.CoreV1().PersistentVolumes().Create(context.TODO(), pv, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("PV Create API error: %w", err)
	}

	pvc := getStaticPVC(pvcName, pvName, size, namespace, sc, isBlock)

	_, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("PVC Create API error: %w", err)
	}
	// bind pvc to app
	app, err := loadApp(appPath)
	if err != nil {
		return err
	}

	app.Namespace = namespace
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcName
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}

	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return err
	}

	err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(context.TODO(), pvc.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.CoreV1().PersistentVolumes().Delete(context.TODO(), pv.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	cmd = fmt.Sprintf("rbd rm %s %s", rbdImageName, rbdOptions(defaultRBDPool))
	_, _, err = execCommandInToolBoxPod(f, cmd, rookNamespace)
	return err
}

// nolint:gocyclo // reduce complexity
func validateCephFsStaticPV(f *framework.Framework, appPath, scPath string) error {
	opt := make(map[string]string)
	var (
		cephFsVolName = "testSubVol"
		groupName     = "testGroup"
		fsName        = "myfs"
		pvName        = "pv-name"
		pvcName       = "pvc-name"
		namespace     = f.UniqueName
		// minikube creates default storage class in cluster, we need to set dummy
		// storageclass on PV and PVC to avoid storageclass name mismatch
		sc         = "storage-class"
		secretName = "cephfs-static-pv-sc" // #nosec
	)

	c := f.ClientSet

	listOpt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}

	fsID, e, err := execCommandInPod(f, "ceph fsid", rookNamespace, &listOpt)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to get fsid from ceph cluster %s", e)
	}
	// remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")

	// 4GiB in bytes
	size := "4294967296"

	// create subvolumegroup, command will work even if group is already present.
	cmd := fmt.Sprintf("ceph fs subvolumegroup create %s %s", fsName, groupName)

	_, e, err = execCommandInPod(f, cmd, rookNamespace, &listOpt)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to create subvolumegroup %s", e)
	}

	// create subvolume
	cmd = fmt.Sprintf("ceph fs subvolume create %s %s %s --size %s", fsName, cephFsVolName, groupName, size)
	_, e, err = execCommandInPod(f, cmd, rookNamespace, &listOpt)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to create subvolume %s", e)
	}

	// get rootpath
	cmd = fmt.Sprintf("ceph fs subvolume getpath %s %s %s", fsName, cephFsVolName, groupName)
	rootPath, e, err := execCommandInPod(f, cmd, rookNamespace, &listOpt)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to get rootpath %s", e)
	}
	// remove new line present in rootPath
	rootPath = strings.Trim(rootPath, "\n")

	// create secret
	secret, err := getSecret(scPath)
	if err != nil {
		return err
	}
	adminKey, e, err := execCommandInPod(f, "ceph auth get-key client.admin", rookNamespace, &listOpt)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to get adminKey %s", e)
	}
	secret.StringData["userID"] = adminUser
	secret.StringData["userKey"] = adminKey
	secret.Name = secretName
	secret.Namespace = cephCSINamespace
	_, err = c.CoreV1().Secrets(cephCSINamespace).Create(context.TODO(), &secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create secret, error %w", err)
	}

	opt["clusterID"] = fsID
	opt["fsName"] = fsName
	opt["staticVolume"] = "true"
	opt["rootPath"] = rootPath
	pv := getStaticPV(pvName, pvName, "4Gi", secretName, cephCSINamespace, sc, "cephfs.csi.ceph.com", false, opt)
	_, err = c.CoreV1().PersistentVolumes().Create(context.TODO(), pv, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PV, error %w", err)
	}

	pvc := getStaticPVC(pvcName, pvName, size, namespace, sc, false)
	_, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PVC, error %w", err)
	}
	// bind pvc to app
	app, err := loadApp(appPath)
	if err != nil {
		return fmt.Errorf("failed to load app, error %w", err)
	}

	app.Namespace = namespace
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcName
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create pod, error %w", err)
	}

	err = deletePod(app.Name, namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete pod, error %w", err)
	}

	err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(context.TODO(), pvc.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.CoreV1().PersistentVolumes().Delete(context.TODO(), pv.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.CoreV1().Secrets(cephCSINamespace).Delete(context.TODO(), secret.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	// delete subvolume
	cmd = fmt.Sprintf("ceph fs subvolume rm %s %s %s", fsName, cephFsVolName, groupName)
	_, e, err = execCommandInPod(f, cmd, rookNamespace, &listOpt)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to remove sub-volume %s", e)
	}

	// delete subvolume group
	cmd = fmt.Sprintf("ceph fs subvolumegroup rm %s %s", fsName, groupName)
	_, e, err = execCommandInPod(f, cmd, rookNamespace, &listOpt)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to remove subvolume group %s", e)
	}

	return nil
}
