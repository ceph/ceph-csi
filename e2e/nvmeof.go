/*
Copyright 2025 The Ceph-CSI Authors.

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
	"strings"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	e2edebug "k8s.io/kubernetes/test/e2e/framework/debug"
	"k8s.io/pod-security-admission/api"
)

const (
	nvmeofPool = "nvmeofpool"
)

var _ = ginkgo.Describe("nvmeof", func() {
	if !testNVMeoF {
		framework.Logf("Skipping NVMe-oF E2E")

		return
	}

	// only support deployment through YAML files (for now)
	if helmTest || operatorDeployment {
		framework.Logf("Skipping NVMe-oF E2E (simple deployment only)")

		return
	}

	f := framework.NewDefaultFramework("nvmeof")
	f.NamespacePodSecurityEnforceLevel = api.LevelPrivileged

	// is set during BeforeEach(), f.UniqueName is empty here
	var nvmeofStorageClass string

	ginkgo.BeforeEach(func() {
		if !deployNVMeoF {
			return
		}

		version, err := getCephVersion(f)
		if err != nil {
			logAndFail("failed to get Ceph cluster version: %v", err)
		}
		if version.GetMajor() < CephMajorTentacle {
			deployNVMeoF = false
			ginkgo.Skip("Skipping NVMe-oF E2E, requires Ceph 20 (Tentacle):" + version.String())
		}

		framework.Logf("NVMe-oF testing supported, Ceph version: %s", version)

		// FIXME: gateway should get deployed by Rook
		deployGateway(f, deployTimeout)

		// No need to create the namespace if ceph-csi is deployed via helm or operator.
		if cephCSINamespace != defaultNs && !(helmTest || operatorDeployment) {
			err := createNamespace(f.ClientSet, cephCSINamespace)
			if err != nil {
				logAndFail("failed to create namespace: %v", err)
			}
		}

		// Ceph credentials referenced in the StorageClass
		createNVMeoFCredentials(f)

		// FIXME: use ceph-csi-operator
		deployNVMeoFPlugin(f, deployTimeout)

		// create the StorageClass
		options := map[string]string{}
		params := map[string]string{
			"pool": nvmeofPool,
		}
		policy := v1.PersistentVolumeReclaimDelete

		nvmeofStorageClass = "e2e-" + f.UniqueName + "-sc"
		createNVMeoFStorageClass(f, nvmeofStorageClass, options, params, policy)
	})

	ginkgo.AfterEach(func() {
		if !deployNVMeoF {
			return
		}

		if ginkgo.CurrentSpecReport().Failed() {
			// log pods created by helm chart
			//logsCSIPods("app="+helmNFSPodsLabel, c)
			// log provisioner
			logsCSIPods("app="+nvmeofDeploymentName, f.ClientSet)
			// log node plugin
			logsCSIPods("app="+nvmeofDaemonsetName, f.ClientSet)

			// Gateway logs - need to search in rook-ceph namespace
			opt := metav1.ListOptions{LabelSelector: "app=ceph-nvmeof-gateway"}
			podList, _ := f.ClientSet.CoreV1().Pods(rookNamespace).List(context.TODO(), opt)
			for i := range podList.Items {
				kubectlLogPod(f.ClientSet, &podList.Items[i])
			}

			// log all details from the namespace where Ceph-CSI is deployed
			e2edebug.DumpAllNamespaceInfo(context.TODO(), f.ClientSet, cephCSINamespace)
		}

		deleteNVMeoFPlugin()
		deleteGateway(f)
		deleteNVMeofStorageClass(f, nvmeofStorageClass)
	})

	ginkgo.Context("Test NVMe CSI", func() {

		pvcPath := nvmeofExamplePath + "pvc.yaml"
		appPath := nvmeofExamplePath + "pod.yaml"
		rawPvcPath := nvmeofExamplePath + "raw-block-pvc.yaml"
		rawAppPath := nvmeofExamplePath + "raw-block-pod.yaml"

		ginkgo.It("Test NVMe CSI", func() {
			ginkgo.By("Check Kubernetes setup details", func() {
				// Check nodes
				nodes, err := f.ClientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
				if err == nil && len(nodes.Items) > 0 {
					node := nodes.Items[0]
					framework.Logf("Node name: %s", node.Name)
					framework.Logf("Node labels: %v", node.Labels)
					framework.Logf("Node provider ID: %s", node.Spec.ProviderID)
				}

				// Check for network mode
				cmd := "ip route show && ip addr show"
				opt := metav1.ListOptions{
					LabelSelector: "app=" + nvmeofDaemonsetName,
				}
				pods, _ := f.ClientSet.CoreV1().Pods(cephCSINamespace).List(context.TODO(), opt)
				if len(pods.Items) > 0 {
					stdout, _, _ := execCommandInPodWithName(f, cmd, pods.Items[0].Name, nvmeofContainerName, cephCSINamespace)
					framework.Logf("Network routing info:\n%s", stdout)
				}
			})

			ginkgo.By("Check CNI configuration", func() {
				// Method 1: Check for CNI pods
				cniPods, err := f.ClientSet.CoreV1().Pods("kube-system").List(context.TODO(), metav1.ListOptions{})
				if err == nil {
					framework.Logf("kube-system pods:")
					for _, pod := range cniPods.Items {
						if strings.Contains(pod.Name, "calico") ||
							strings.Contains(pod.Name, "flannel") ||
							strings.Contains(pod.Name, "weave") ||
							strings.Contains(pod.Name, "cilium") ||
							strings.Contains(pod.Name, "kindnet") {
							framework.Logf("  CNI pod found: %s", pod.Name)
						}
					}
				}

				// Method 2: Check node for CNI config
				cmd := "ls -la /etc/cni/net.d/ && cat /etc/cni/net.d/* 2>/dev/null || echo 'No CNI config found'"

				opt := metav1.ListOptions{
					LabelSelector: "app=" + nvmeofDaemonsetName,
				}
				pods, err := f.ClientSet.CoreV1().Pods(cephCSINamespace).List(context.TODO(), opt)
				if err == nil && len(pods.Items) > 0 {
					stdout, stderr, _ := execCommandInPodWithName(f, cmd, pods.Items[0].Name, nvmeofContainerName, cephCSINamespace)
					framework.Logf("CNI config on node: stdout=%s, stderr=%s", stdout, stderr)
				}
			})

			ginkgo.By("create a PVC and delete it", func() {
				pvc, err := loadPVC(pvcPath)
				Expect(err).ShouldNot(HaveOccurred())

				pvc.Namespace = f.UniqueName
				pvc.Spec.StorageClassName = &nvmeofStorageClass

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())

				validateRBDImageCount(f, 1, nvmeofPool)
				validateOmapCount(f, 1, rbdType, nvmeofPool, volumesType)

				err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())

				// validate created backend rbd images
				validateRBDImageCount(f, 0, nvmeofPool)
				validateOmapCount(f, 0, rbdType, nvmeofPool, volumesType)
			})

			ginkgo.By("Resize Filesystem PVC and check application directory size", func() {

				// GADI
				// Get gateway IP before test
				gwName, gwIP := getNVMeofGateway(f.ClientSet)
				framework.Logf("Gateway before filesystem test: %s at IP %s", gwName, gwIP)
				// end GADI

				pvc, err := loadPVC(pvcPath)
				Expect(err).ShouldNot(HaveOccurred())

				pvc.Namespace = f.UniqueName
				pvc.Spec.StorageClassName = &nvmeofStorageClass

				err = resizePVCAndValidateSize(pvc, appPath, f)
				Expect(err).ShouldNot(HaveOccurred())

				// GADI
				// Get gateway IP after test
				gwName, gwIP = getNVMeofGateway(f.ClientSet)
				framework.Logf("Gateway after filesystem test: %s at IP %s", gwName, gwIP)
				// end GADI

				// validate created backend rbd images
				validateRBDImageCount(f, 0, nvmeofPool)
				validateOmapCount(f, 0, rbdType, nvmeofPool, volumesType)
			})

			ginkgo.By("Check NVMe controller state before block test", func() {
				cmd := `
echo "=== Controller States ===" && \
cat /sys/class/nvme/nvme*/state && \
echo "=== Controller Addresses ===" && \
cat /sys/class/nvme/nvme*/address && \
echo "=== Namespace Paths ===" && \
ls -d /sys/block/nvme*n* 2>/dev/null || echo "No namespaces found" && \
echo "=== ANA States ===" && \
cat /sys/block/nvme*n*/ana_state 2>/dev/null || echo "No ANA states found"
`

				opt := metav1.ListOptions{
					LabelSelector: "app=" + nvmeofDaemonsetName,
				}
				pods, err := f.ClientSet.CoreV1().Pods(cephCSINamespace).List(context.TODO(), opt)
				if err == nil && len(pods.Items) > 0 {
					stdout, stderr, execErr := execCommandInPodWithName(
						f, cmd,
						pods.Items[0].Name,
						nvmeofContainerName,
						cephCSINamespace,
					)
					framework.Logf("NVMe controller state:\nstdout:\n%s\nstderr:\n%s\nerr: %v",
						stdout, stderr, execErr)
				}
			})

			ginkgo.By("Resize Block PVC and check Device size", func() {

				// // Run nvme disconnect-all on the node
				// cmd := "nvme disconnect-all"
				// // Get a pod running on the node (use your test pod or a daemonset pod)
				// opt := metav1.ListOptions{
				// 	LabelSelector: "app=" + nvmeofDaemonsetName, // "csi-nvmeofplugin"
				// }
				// pods, err := f.ClientSet.CoreV1().Pods(cephCSINamespace).List(context.TODO(), opt)
				// Expect(err).ShouldNot(HaveOccurred())
				// Expect(len(pods.Items)).Should(BeNumerically(">", 0))

				// podName := pods.Items[0].Name
				// containerName := pods.Items[0].Spec.Containers[0].Name

				// _, _, err = execCommandInPodWithName(f, cmd, podName, containerName, cephCSINamespace)
				// if err != nil {
				// 	framework.Logf("Warning: failed to disconnect NVMe devices: %v", err)
				// }

				// GADI
				// Get gateway IP before test
				gwName, gwIP := getNVMeofGateway(f.ClientSet)
				framework.Logf("Gateway before block test: %s at IP %s", gwName, gwIP)
				// end GADI

				pvc, err := loadPVC(rawPvcPath)
				Expect(err).ShouldNot(HaveOccurred())

				pvc.Namespace = f.UniqueName
				pvc.Spec.StorageClassName = &nvmeofStorageClass

				err = resizePVCAndValidateSize(pvc, rawAppPath, f)
				Expect(err).ShouldNot(HaveOccurred())

				// GADI
				// Get gateway IP after test
				gwName, gwIP = getNVMeofGateway(f.ClientSet)
				framework.Logf("Gateway after block test: %s at IP %s", gwName, gwIP)
				// end GADI

				// validate created backend rbd images
				validateRBDImageCount(f, 0, nvmeofPool)
				validateOmapCount(f, 0, rbdType, nvmeofPool, volumesType)
			})
		})
	})
})
