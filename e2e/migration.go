package e2e

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

func validateRBDStaticMigrationPVDeletion(f *framework.Framework, appPath, scName string, isBlock bool) error {
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
		return fmt.Errorf("failed to delete PVC and application with error %w", err)
	}

	return err
}

// composeIntreeMigVolID create a volID similar to intree migration volID
// the migration volID format looks like below
// mig-mons-<hash>-image-<UUID_<poolhash>
// nolint:lll    // ex: "mig_mons-b7f67366bb43f32e07d8a261a7840da9_image-e0b45b52-7e09-47d3-8f1b-806995fa4412_706f6f6c5f7265706c6963615f706f6f6c
func composeIntreeMigVolID(mons, rbdImageName string) string {
	poolField := hex.EncodeToString([]byte(defaultRBDPool))
	monsField := monsPrefix + getMonsHash(mons)
	imageUID := strings.Split(rbdImageName, intreeVolPrefix)[1:]
	imageField := imagePrefix + imageUID[0]
	vhSlice := []string{migIdentifier, monsField, imageField, poolField}

	return strings.Join(vhSlice, "_")
}

// generateClusterIDConfigMapForMigration retrieve monitors and generate a hash value which
// is used as a clusterID in the custom configmap, this function also recreate RBD CSI pods
// once the custom config map has been recreated.
func generateClusterIDConfigMapForMigration(f *framework.Framework, c kubernetes.Interface) error {
	// create monitors hash by fetching monitors from the cluster.
	mons, err := getMons(rookNamespace, c)
	if err != nil {
		return fmt.Errorf("failed to get monitors %w", err)
	}
	mon := strings.Join(mons, ",")
	inClusterID := getMonsHash(mon)

	clusterInfo := map[string]map[string]string{}
	clusterInfo[inClusterID] = map[string]string{}

	// create custom configmap
	err = createCustomConfigMap(f.ClientSet, rbdDirPath, clusterInfo)
	if err != nil {
		return fmt.Errorf("failed to create configmap with error %w", err)
	}
	// restart csi pods for the configmap to take effect.
	err = recreateCSIRBDPods(f)
	if err != nil {
		return fmt.Errorf("failed to recreate rbd csi pods with error %w", err)
	}

	return nil
}
