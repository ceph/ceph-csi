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
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	staticPVSize         = "4Gi"
	staticPVNewSize      = "8Gi"
	staticPVImageFeature = "layering"
	monsPrefix           = "mons-"
	imagePrefix          = "image-"
	migIdentifier        = "mig"
	intreeVolPrefix      = "kubernetes-dynamic-pvc-"
)

func getStaticPV(
	name, volName, size, secretName, secretNS, sc, driverName string,
	blockPV bool,
	options, annotations map[string]string, policy v1.PersistentVolumeReclaimPolicy,
) *v1.PersistentVolume {
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: policy,
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
	if len(annotations) > 0 {
		pv.Annotations = make(map[string]string)
		for k, v := range annotations {
			pv.Annotations[k] = v
		}
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

func validateRBDStaticPV(f *framework.Framework, appPath string, isBlock, checkImgFeat bool) error {
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

	fsID, err := getClusterID(f)
	if err != nil {
		return fmt.Errorf("failed to get clusterID: %w", err)
	}

	size := staticPVSize
	// create rbd image
	cmd := fmt.Sprintf(
		"rbd create %s --size=%s --image-feature=layering %s",
		rbdImageName,
		staticPVSize,
		rbdOptions(defaultRBDPool))

	_, e, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to create rbd image %s", e)
	}
	opt["clusterID"] = fsID
	if !checkImgFeat {
		opt["imageFeatures"] = staticPVImageFeature
	}
	opt["pool"] = defaultRBDPool
	opt["staticVolume"] = strconv.FormatBool(true)
	if radosNamespace != "" {
		opt["radosNamespace"] = radosNamespace
	}

	pv := getStaticPV(
		pvName,
		rbdImageName,
		size,
		rbdNodePluginSecretName,
		cephCSINamespace,
		sc,
		"rbd.csi.ceph.com",
		isBlock,
		opt,
		nil, retainPolicy)

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

	app.Labels = make(map[string]string)
	app.Labels[appKey] = appLabel
	appOpt := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", appKey, appLabel),
	}
	app.Namespace = namespace
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcName
	if checkImgFeat {
		err = createAppErr(f.ClientSet, app, deployTimeout, "missing required parameter imageFeatures")
	} else {
		err = createApp(f.ClientSet, app, deployTimeout)
	}
	if err != nil {
		return err
	}

	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return err
	}

	// resize image only if the image is already mounted and formatted
	if !checkImgFeat {
		err = validateRBDStaticResize(f, app, &appOpt, pvc, rbdImageName)
		if err != nil {
			return err
		}
	}

	err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(context.TODO(), pvc.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pvc: %w", err)
	}

	err = c.CoreV1().PersistentVolumes().Delete(context.TODO(), pv.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pv: %w", err)
	}

	cmd = fmt.Sprintf("rbd rm %s %s", rbdImageName, rbdOptions(defaultRBDPool))
	_, _, err = execCommandInToolBoxPod(f, cmd, rookNamespace)

	return err
}

func validateRBDStaticMigrationPVC(f *framework.Framework, appPath, scName string, isBlock bool) error {
	opt := make(map[string]string)
	var (
		rbdImageName        = "kubernetes-dynamic-pvc-e0b45b52-7e09-47d3-8f1b-806995fa4412"
		pvName              = "pv-name"
		pvcName             = "pvc-name"
		namespace           = f.UniqueName
		sc                  = scName
		provisionerAnnKey   = "pv.kubernetes.io/provisioned-by"
		provisionerAnnValue = "rbd.csi.ceph.com"
	)

	c := f.ClientSet
	PVAnnMap := make(map[string]string)
	PVAnnMap[provisionerAnnKey] = provisionerAnnValue
	mons, err := getMons(rookNamespace, c)
	if err != nil {
		return fmt.Errorf("failed to get mons: %w", err)
	}
	mon := strings.Join(mons, ",")
	size := staticPVSize
	// create rbd image
	cmd := fmt.Sprintf(
		"rbd create %s --size=%s --image-feature=layering %s",
		rbdImageName,
		staticPVSize,
		rbdOptions(defaultRBDPool))

	_, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to create rbd image %s", stdErr)
	}

	opt["migration"] = "true"
	opt["clusterID"] = getMonsHash(mon)
	opt["imageFeatures"] = staticPVImageFeature
	opt["pool"] = defaultRBDPool
	opt["staticVolume"] = strconv.FormatBool(true)
	opt["imageName"] = rbdImageName

	// Make volumeID similar to the migration volumeID
	volID := composeIntreeMigVolID(mon, rbdImageName)
	pv := getStaticPV(
		pvName,
		volID,
		size,
		rbdNodePluginSecretName,
		cephCSINamespace,
		sc,
		provisionerAnnValue,
		isBlock,
		opt,
		PVAnnMap,
		deletePolicy)

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

	err = deletePVCAndApp("", f, pvc, app)
	if err != nil {
		return fmt.Errorf("failed to delete PVC and application: %w", err)
	}

	return err
}

//nolint:gocyclo,cyclop // reduce complexity
func validateCephFsStaticPV(f *framework.Framework, appPath, scPath string) error {
	opt := make(map[string]string)
	var (
		cephFsVolName = "testSubVol"
		groupName     = "testGroup"
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

	fsID, err := getClusterID(f)
	if err != nil {
		return fmt.Errorf("failed to get clusterID: %w", err)
	}

	// 4GiB in bytes
	size := "4294967296"

	// create subvolumegroup, command will work even if group is already present.
	cmd := fmt.Sprintf("ceph fs subvolumegroup create %s %s", fileSystemName, groupName)

	_, e, err := execCommandInPod(f, cmd, rookNamespace, &listOpt)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to create subvolumegroup: %s", e)
	}

	// create subvolume
	cmd = fmt.Sprintf("ceph fs subvolume create %s %s %s --size %s", fileSystemName, cephFsVolName, groupName, size)
	_, e, err = execCommandInPod(f, cmd, rookNamespace, &listOpt)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to create subvolume: %s", e)
	}

	// get rootpath
	cmd = fmt.Sprintf("ceph fs subvolume getpath %s %s %s", fileSystemName, cephFsVolName, groupName)
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
		return fmt.Errorf("failed to create secret: %w", err)
	}

	opt["clusterID"] = fsID
	opt["fsName"] = fileSystemName
	opt["staticVolume"] = strconv.FormatBool(true)
	opt["rootPath"] = rootPath
	pv := getStaticPV(
		pvName,
		pvName,
		staticPVSize,
		secretName,
		cephCSINamespace,
		sc,
		"cephfs.csi.ceph.com",
		false,
		opt,
		nil,
		retainPolicy)
	_, err = c.CoreV1().PersistentVolumes().Create(context.TODO(), pv, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PV: %w", err)
	}

	pvc := getStaticPVC(pvcName, pvName, size, namespace, sc, false)
	_, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PVC: %w", err)
	}
	// bind pvc to app
	app, err := loadApp(appPath)
	if err != nil {
		return fmt.Errorf("failed to load app: %w", err)
	}

	app.Namespace = namespace
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcName
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create pod: %w", err)
	}

	err = deletePod(app.Name, namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(context.TODO(), pvc.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pvc: %w", err)
	}

	err = c.CoreV1().PersistentVolumes().Delete(context.TODO(), pv.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pv: %w", err)
	}

	err = c.CoreV1().Secrets(cephCSINamespace).Delete(context.TODO(), secret.Name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	// delete subvolume
	cmd = fmt.Sprintf("ceph fs subvolume rm %s %s %s", fileSystemName, cephFsVolName, groupName)
	_, e, err = execCommandInPod(f, cmd, rookNamespace, &listOpt)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to remove sub-volume %s", e)
	}

	// delete subvolume group
	cmd = fmt.Sprintf("ceph fs subvolumegroup rm %s %s", fileSystemName, groupName)
	_, e, err = execCommandInPod(f, cmd, rookNamespace, &listOpt)
	if err != nil {
		return err
	}
	if e != "" {
		return fmt.Errorf("failed to remove subvolume group %s", e)
	}

	return nil
}

func validateRBDStaticResize(
	f *framework.Framework,
	app *v1.Pod,
	appOpt *metav1.ListOptions,
	pvc *v1.PersistentVolumeClaim,
	rbdImageName string,
) error {
	// resize rbd image
	size := staticPVNewSize
	cmd := fmt.Sprintf(
		"rbd resize %s --size=%s %s",
		rbdImageName,
		size,
		rbdOptions(defaultRBDPool))

	_, _, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return err
	}
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}
	// check size for the filesystem type PVC
	if *pvc.Spec.VolumeMode == v1.PersistentVolumeFilesystem {
		err = checkDirSize(app, f, appOpt, size)
		if err != nil {
			return err
		}
	}

	return deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
}
