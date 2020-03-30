package e2e

import (
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
		ns           = f.UniqueName
		// minikube creates default class in cluster, we need to set dummy
		// storageclass on PV and PVC to avoid storageclass name missmatch
		sc = "storage-class"
	)

	c := f.ClientSet

	listOpt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-tools",
	}

	fsID, e := execCommandInPod(f, "ceph fsid", rookNamespace, &listOpt)
	if e != "" {
		return fmt.Errorf("failed to get fsid from ceph cluster %s", e)
	}
	// remove new line present in fsID
	fsID = strings.Trim(fsID, "\n")
	size := "4Gi"
	// create rbd image
	cmd := fmt.Sprintf("rbd create %s --size=%d --pool=replicapool --image-feature=layering", rbdImageName, 4096)

	_, e = execCommandInPod(f, cmd, rookNamespace, &listOpt)
	if e != "" {
		return fmt.Errorf("failed to create rbd image %s", e)
	}
	opt["clusterID"] = fsID
	opt["imageFeatures"] = "layering"
	opt["pool"] = "replicapool"
	opt["staticVolume"] = "true"

	pv := getStaticPV(pvName, rbdImageName, size, "csi-rbd-secret", cephCSINamespace, sc, "rbd.csi.ceph.com", isBlock, opt)

	_, err := c.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("PV Create API error: %v", err)
	}

	pvc := getStaticPVC(pvcName, pvName, size, ns, sc, isBlock)

	_, err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("PVC Create API error: %v", err)
	}
	// bind pvc to app
	app, err := loadApp(appPath)
	if err != nil {
		return err
	}

	app.Namespace = ns
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcName
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}

	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return err
	}

	err = c.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(ctx, pvc.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	err = c.CoreV1().PersistentVolumes().Delete(ctx, pv.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	cmd = fmt.Sprintf("rbd rm %s --pool=replicapool", rbdImageName)
	execCommandInPod(f, cmd, rookNamespace, &listOpt)
	return nil
}
