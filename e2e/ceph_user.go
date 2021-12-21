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
	"fmt"
	"strings"

	"k8s.io/kubernetes/test/e2e/framework"
)

// #nosec because of the word `Secret`
const (
	// ceph user names.
	keyringRBDProvisionerUsername          = "cephcsi-rbd-provisioner"
	keyringRBDNodePluginUsername           = "cephcsi-rbd-node"
	keyringRBDNamespaceProvisionerUsername = "cephcsi-rbd-ns-provisioner"
	keyringRBDNamespaceNodePluginUsername  = "cephcsi-rbd-ns-node"
	keyringCephFSProvisionerUsername       = "cephcsi-cephfs-provisioner"
	keyringCephFSNodePluginUsername        = "cephcsi-cephfs-node"
	// secret names.
	rbdNodePluginSecretName           = "cephcsi-rbd-node"
	rbdProvisionerSecretName          = "cephcsi-rbd-provisioner"
	rbdNamespaceNodePluginSecretName  = "cephcsi-rbd-ns-node"
	rbdNamespaceProvisionerSecretName = "cephcsi-rbd-ns-provisioner"
	rbdMigrationNodePluginSecretName  = "cephcsi-rbd-mig-node"
	rbdMigrationProvisionerSecretName = "cephcsi-rbd-mig-provisioner"
	cephFSNodePluginSecretName        = "cephcsi-cephfs-node"
	cephFSProvisionerSecretName       = "cephcsi-cephfs-provisioner"
)

// refer https://github.com/ceph/ceph-csi/blob/devel/docs/capabilities.md#rbd
// for RBD caps.
func rbdNodePluginCaps(pool, rbdNamespace string) []string {
	caps := []string{
		"mon", "'profile rbd'",
		"mgr", "'allow rw'",
	}
	if rbdNamespace == "" {
		caps = append(caps, "osd", "'profile rbd'")
	} else {
		caps = append(caps, fmt.Sprintf("osd 'profile rbd pool=%s namespace=%s'", pool, rbdNamespace))
	}

	return caps
}

func rbdProvisionerCaps(pool, rbdNamespace string) []string {
	caps := []string{
		"mon", "'profile rbd'",
		"mgr", "'allow rw'",
	}
	if rbdNamespace == "" {
		caps = append(caps, "osd", "'profile rbd'")
	} else {
		caps = append(caps, fmt.Sprintf("osd 'profile rbd pool=%s namespace=%s'", pool, rbdNamespace))
	}

	return caps
}

// refer https://github.com/ceph/ceph-csi/blob/devel/docs/capabilities.md#rbd
// for cephFS caps.
func cephFSNodePluginCaps() []string {
	caps := []string{
		"mon", "'allow r'",
		"mgr", "'allow rw'",
		"osd", "'allow rw tag cephfs *=*'",
		"mds", "'allow rw'",
	}

	return caps
}

func cephFSProvisionerCaps() []string {
	caps := []string{
		"mon", "'allow r'",
		"mgr", "'allow rw'",
		"osd", "'allow rw tag cephfs metadata=*'",
	}

	return caps
}

func createCephUser(f *framework.Framework, user string, caps []string) (string, error) {
	cmd := fmt.Sprintf("ceph auth get-or-create-key client.%s %s", user, strings.Join(caps, " "))
	stdOut, stdErr, err := execCommandInToolBoxPod(f, cmd, rookNamespace)
	if err != nil {
		return "", err
	}
	if stdErr != "" {
		return "", fmt.Errorf("failed to create user %s: %v", cmd, stdErr)
	}

	return strings.TrimSpace(stdOut), nil
}

func deleteCephUser(f *framework.Framework, user string) error {
	cmd := fmt.Sprintf("ceph auth del client.%s", user)
	_, _, err := execCommandInToolBoxPod(f, cmd, rookNamespace)

	return err
}
