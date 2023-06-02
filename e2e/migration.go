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
	"encoding/hex"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

// composeIntreeMigVolID create a volID similar to intree migration volID
// the migration volID format looks like below
// mig-mons-<hash>-image-<UUID_<poolhash>
//
//nolint:lll    // ex: "mig_mons-b7f67366bb43f32e07d8a261a7840da9_image-e0b45b52-7e09-47d3-8f1b-806995fa4412_706f6f6c5f7265706c6963615f706f6f6c
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
	err = recreateCSIPods(f, rbdPodLabels, rbdDaemonsetName, rbdDeploymentName)
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

// createMigrationSC create a SC with migration specific secrets and clusterid.
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
	mons, err := getMons(rookNamespace, f.ClientSet)
	if err != nil {
		return fmt.Errorf("failed to get mons: %w", err)
	}
	mon := strings.Join(mons, ",")
	param["migration"] = "true"
	param["clusterID"] = getMonsHash(mon)
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

// setupMigrationCMSecretAndSC create custom configmap, secret and SC for migration tests.
func setupMigrationCMSecretAndSC(f *framework.Framework, scName string) error {
	c := f.ClientSet
	if scName == "" {
		scName = defaultSCName
	}
	err := generateClusterIDConfigMapForMigration(f, c)
	if err != nil {
		return fmt.Errorf("failed to generate clusterID configmap: %w", err)
	}

	err = createMigrationUserSecretAndSC(f, scName)
	if err != nil {
		return fmt.Errorf("failed to create storageclass: %w", err)
	}

	return nil
}

// tearDownMigrationSetup deletes custom configmap and secret.
func tearDownMigrationSetup(f *framework.Framework) error {
	err := deleteConfigMap(rbdDirPath)
	if err != nil {
		return fmt.Errorf("failed to delete configmap: %w", err)
	}
	err = createConfigMap(rbdDirPath, f.ClientSet, f)
	if err != nil {
		return fmt.Errorf("failed to create configmap: %w", err)
	}
	err = deleteProvNodeMigrationSecret(f, true, true)
	if err != nil {
		return fmt.Errorf("failed to delete migration users and Secrets associated: %w", err)
	}

	return nil
}
