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
		return fmt.Errorf("failed to delete PVC and application: %w", err)
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
		return fmt.Errorf("failed to create configmap: %w", err)
	}
	// restart csi pods for the configmap to take effect.
	err = recreateCSIRBDPods(f)
	if err != nil {
		return fmt.Errorf("failed to recreate rbd csi pods: %w", err)
	}

	return nil
}

// createRBDMigrationSecret creates a migration secret with the passed in user name.
// this secret differs from csi secret data on below aspects.
// equivalent to the `UserKey` field, migration secret has `key` field.
// if 'userName' has passed and if it is not admin, the passed in userName will be
// set as the `adminId` field in the secret.
func createRBDMigrationSecret(f *framework.Framework, secretName, userName, userKey string) error {
	secPath := fmt.Sprintf("%s/%s", rbdExamplePath, "secret.yaml")
	sec, err := getSecret(secPath)
	if err != nil {
		return err
	}
	if secretName != "" {
		sec.Name = secretName
	}
	// if its admin, we dont need to change anything in the migration secret, the CSI driver
	// will use the key from existing secret and continue.
	if userName != "admin" {
		sec.StringData["adminId"] = userName
	}
	sec.StringData["key"] = userKey
	sec.Namespace = cephCSINamespace
	_, err = f.ClientSet.CoreV1().Secrets(cephCSINamespace).Create(context.TODO(), &sec, metav1.CreateOptions{})

	return err
}

// createMigrationUserSecretAndSC creates migration user and a secret associated with this user first,
// then create SC based on the same.
func createMigrationUserSecretAndSC(f *framework.Framework, scName string) error {
	if scName == "" {
		scName = defaultSCName
	}
	err := createProvNodeCephUserAndSecret(f, true, true)
	if err != nil {
		return err
	}

	err = createMigrationSC(f, scName)
	if err != nil {
		return err
	}

	return nil
}

func createMigrationSC(f *framework.Framework, scName string) error {
	err := deleteResource(rbdExamplePath + "storageclass.yaml")
	if err != nil {
		return fmt.Errorf("failed to delete storageclass: %w", err)
	}
	param := make(map[string]string)
	// add new secrets to the SC parameters
	param["csi.storage.k8s.io/provisioner-secret-namespace"] = cephCSINamespace
	param["csi.storage.k8s.io/provisioner-secret-name"] = rbdMigrationProvisionerSecretName
	param["csi.storage.k8s.io/controller-expand-secret-namespace"] = cephCSINamespace
	param["csi.storage.k8s.io/controller-expand-secret-name"] = rbdMigrationProvisionerSecretName
	param["csi.storage.k8s.io/node-stage-secret-namespace"] = cephCSINamespace
	param["csi.storage.k8s.io/node-stage-secret-name"] = rbdMigrationNodePluginSecretName
	err = createRBDStorageClass(f.ClientSet, f, scName, nil, param, deletePolicy)
	if err != nil {
		return fmt.Errorf("failed to create storageclass: %w", err)
	}

	return nil
}

// createProvNodeCephUserAndSecret fetches the ceph migration user's key and create migration secret
// with it based on the arg values of 'provSecret' and 'nodeSecret'.
func createProvNodeCephUserAndSecret(f *framework.Framework, provisionerSecret, nodeSecret bool) error {
	if provisionerSecret {
		// Fetch the key.
		key, err := createCephUser(
			f,
			keyringRBDProvisionerUsername,
			rbdProvisionerCaps(defaultRBDPool, radosNamespace),
		)
		if err != nil {
			return fmt.Errorf("failed to create user %q: %w", keyringRBDProvisionerUsername, err)
		}
		err = createRBDMigrationSecret(f, rbdMigrationProvisionerSecretName, keyringRBDProvisionerUsername, key)
		if err != nil {
			return fmt.Errorf("failed to create provisioner secret: %w", err)
		}
	}

	if nodeSecret {
		// Fetch the key.
		key, err := createCephUser(
			f,
			keyringRBDNodePluginUsername,
			rbdNodePluginCaps(defaultRBDPool, radosNamespace))
		if err != nil {
			return fmt.Errorf("failed to create user %q: %w", keyringRBDNodePluginUsername, err)
		}
		err = createRBDMigrationSecret(f, rbdMigrationNodePluginSecretName, keyringRBDNodePluginUsername, key)
		if err != nil {
			return fmt.Errorf("failed to create node secret: %w", err)
		}
	}

	return nil
}

// deleteProvNodeMigrationSecret deletes ceph migration secrets based on the
// arg values of 'provisionerSecret' and 'nodeSecret'.
func deleteProvNodeMigrationSecret(f *framework.Framework, provisionerSecret, nodeSecret bool) error {
	c := f.ClientSet
	if provisionerSecret {
		// delete RBD provisioner secret.
		err := c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), rbdMigrationProvisionerSecretName, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete provisioner secret: %w", err)
		}
	}

	if nodeSecret {
		// delete RBD node secret.
		err := c.CoreV1().
			Secrets(cephCSINamespace).
			Delete(context.TODO(), rbdMigrationNodePluginSecretName, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete node secret: %w", err)
		}
	}

	return nil
}
