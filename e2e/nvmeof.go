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
	"time"

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

// createPVCAppAndDelete creates a PVC and starts an application Pod with it.
// Once the Pod is running, both are deleted.
func createPVCAppAndDelete(pvcPath, appPath, storageClass string, f *framework.Framework) error {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}
	pvc.Namespace = f.UniqueName
	pvc.Spec.StorageClassName = &storageClass

	app, err := loadApp(appPath)
	if err != nil {
		return err
	}
	app.Namespace = f.UniqueName

	// Create PVC and app
	err = createPVCAndApp("", f, pvc, app, deployTimeout)
	if err != nil {
		return err
	}

	// Delete PVC and app
	err = deletePVCAndApp("", f, pvc, app)
	return err
}

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
		// rawPvcPath := nvmeofExamplePath + "raw-block-pvc.yaml"
		// rawAppPath := nvmeofExamplePath + "raw-block-pod.yaml"

		ginkgo.It("Test NVMe CSI", func() {
			ginkgo.By("Test 1: create PVC+Pod, delete Pod (keep PVC), recreate Pod", func() {
				pvc, err := loadPVC(pvcPath)
				Expect(err).ShouldNot(HaveOccurred())
				pvc.Namespace = f.UniqueName
				pvc.Spec.StorageClassName = &nvmeofStorageClass

				err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())

				// Get node pod for tcpdump
				opt := metav1.ListOptions{LabelSelector: "app=" + nvmeofDaemonsetName}
				pods, _ := f.ClientSet.CoreV1().Pods(cephCSINamespace).List(context.TODO(), opt)
				var nodePodName string
				if len(pods.Items) > 0 {
					nodePodName = pods.Items[0].Name
				}

				// Start tcpdump in background
				if nodePodName != "" {
					startTcpdump := "nohup tcpdump -i any port 4420 -tttt -n > /tmp/nvme-traffic.log 2>&1 &"
					execCommandInPodWithName(f, startTcpdump, nodePodName, nvmeofContainerName, cephCSINamespace)
					framework.Logf("Started tcpdump on node pod %s", nodePodName)
				}

				// First pod
				app1, err := loadApp(appPath)
				Expect(err).ShouldNot(HaveOccurred())
				app1.Name = "test-pod-1"
				app1.Namespace = f.UniqueName

				err = createApp(f.ClientSet, app1, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())

				// CAPTURE STATE BEFORE DELETE
				cmd := `echo "=== BEFORE POD DELETE ===" && \
nvme list && \
cat /sys/class/nvme/nvme*/state && \
cat /sys/class/nvme/nvme*/address && \
cat /proc/net/tcp | awk '{print $2, $3}' | grep ':1144' || echo "No TCP connections"`

				if nodePodName != "" {
					stdout, stderr, _ := execCommandInPodWithName(f, cmd, nodePodName, nvmeofContainerName, cephCSINamespace)
					framework.Logf("State before pod delete:\n%s\nstderr: %s", stdout, stderr)
				}

				// Delete first pod
				framework.Logf("Deleting pod %s", app1.Name)
				err = deletePod(app1.Name, app1.Namespace, f.ClientSet, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())

				// WAIT A BIT AND CAPTURE STATE AFTER DELETE
				time.Sleep(60 * time.Second)

				cmd = `echo "=== AFTER POD DELETE ===" && \
nvme list && \
cat /sys/class/nvme/nvme*/state 2>/dev/null || echo "No controllers" && \
cat /sys/class/nvme/nvme*/address 2>/dev/null || echo "No addresses" && \
cat /proc/net/tcp | awk '{print $2, $3}' | grep ':1144' || echo "No TCP connections" && \
dmesg -T | tail -50`

				if nodePodName != "" {
					stdout, stderr, _ := execCommandInPodWithName(f, cmd, nodePodName, nvmeofContainerName, cephCSINamespace)
					framework.Logf("State after pod delete:\n%s\nstderr: %s", stdout, stderr)
				}

				// Stop tcpdump and capture traffic
				if nodePodName != "" {
					stopCmd := "killall tcpdump 2>/dev/null; sleep 1; echo '=== TCPDUMP CAPTURE ===' && cat /tmp/nvme-traffic.log | tail -100"
					stdout, _, _ := execCommandInPodWithName(f, stopCmd, nodePodName, nvmeofContainerName, cephCSINamespace)
					framework.Logf("Network traffic during test:\n%s", stdout)
				}

				// Check network stats
				if nodePodName != "" {
					statsCmd := "echo '=== NETWORK STATS ===' && netstat -s | grep -A 10 -i 'tcp:' || cat /proc/net/netstat | grep -i tcp"
					stdout, _, _ := execCommandInPodWithName(f, statsCmd, nodePodName, nvmeofContainerName, cephCSINamespace)
					framework.Logf("Network statistics:\n%s", stdout)
				}

				// Try to create second pod
				framework.Logf("Creating second pod")
				app2, err := loadApp(appPath)
				Expect(err).ShouldNot(HaveOccurred())
				app2.Name = "test-pod-2"
				app2.Namespace = f.UniqueName

				err = createApp(f.ClientSet, app2, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
			})
		})

	})
})
