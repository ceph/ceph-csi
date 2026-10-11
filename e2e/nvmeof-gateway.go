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
	"encoding/json"
	"fmt"
	"net"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	frameworkPod "k8s.io/kubernetes/test/e2e/framework/pod"
)

const (
	e2eGatewayPath = "nvmeof/"

	e2eGatewaySecurityContext = e2eGatewayPath + "scc.yaml"
	e2eGatewayServiceAccount  = e2eGatewayPath + "serviceaccount.yaml"
	e2eGatewayConfig          = e2eGatewayPath + "config.yaml"
	e2eGatewayDeployment      = e2eGatewayPath + "deployment.yaml"
)

func createORDeleteGateway(action kubectlAction) {
	var resources []ResourceDeployer

	if isOpenShift {
		resources = append(resources, &yamlResource{
			filename: e2eGatewaySecurityContext,
			// The SCC contains the namespace:serviceaccount string. Everything should
			// be deployed in the "rook-ceph" namespace.
			replace: map[string]string{
				// replace the namespace for the service account
				":rook-ceph:": ":" + rookNamespace + ":",
			},
		})
	}

	resources = append(resources,
		&yamlResourceNamespaced{
			filename:  e2eGatewayServiceAccount,
			namespace: rookNamespace,
		},
		&yamlResourceNamespaced{
			filename:  e2eGatewayConfig,
			namespace: rookNamespace,
		},
		&yamlResourceNamespaced{
			filename:   e2eGatewayDeployment,
			namespace:  rookNamespace,
			oneReplica: true,
		},
	)

	for _, r := range resources {
		err := r.Do(action)
		Expect(err).ShouldNot(HaveOccurred())
	}
}

func deployGateway(f *framework.Framework, deployTimeout int) {
	// a gateway gets deployed with a pool, the pool needs to exist
	framework.Logf("going to create pool %q with help from the Rook Toolbox", nvmeofPool)
	err := createPool(f, nvmeofPool)
	Expect(err).ShouldNot(HaveOccurred())

	createORDeleteGateway(kubectlCreate)

	err = waitForDeploymentInAvailableState(f.ClientSet, "ceph-nvmeof-gateway", rookNamespace, deployTimeout)
	Expect(err).ShouldNot(HaveOccurred())

	opt := &metav1.ListOptions{
		LabelSelector: "app=ceph-nvmeof-gateway",
	}

	pod, _, err := findPodAndContainerName(f, rookNamespace, "", opt)
	Expect(err).ShouldNot(HaveOccurred())

	err = waitForPodInRunningState(pod, rookNamespace, f.ClientSet, deployTimeout, noError)
	if err != nil {
		// After a failure, the deployment is automatically removed. The gateway Pod
		// is not available anymore when the e2e job gathers all the logs. Record
		// the logs in the job for now (the way of deploying will move to Rook in
		// the future anyway).
		for _, container := range []string{"generate-minimal-ceph-conf", "nvmeof-gateway"} {
			logs, _ := frameworkPod.GetPodLogs(context.TODO(), f.ClientSet, rookNamespace, pod, container)
			framework.Logf("Logs from the %q container of the NVMe-oF gateway:\n%s", container, logs)
		}
	}
	Expect(err).ShouldNot(HaveOccurred())
}

func deleteGateway(f *framework.Framework) {
	createORDeleteGateway(kubectlDelete)

	err := deletePool(nvmeofPool, false, f)
	Expect(err).ShouldNot(HaveOccurred())
}

// getNVMeofGateway returns the name and IP-address of the gateway Pod.
func getNVMeofGateway(c kubernetes.Interface) (string, string) {
	opt := metav1.ListOptions{
		LabelSelector: "app=ceph-nvmeof-gateway",
	}

	pods, err := c.CoreV1().Pods(rookNamespace).List(context.TODO(), opt)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(pods.Items).Should(HaveLen(1))

	return pods.Items[0].Name, pods.Items[0].Status.PodIP
}

// getNVMeofGatewayNetworkMask returns the subnet (CIDR) of the gateway Pod's own interface,
// for use as the networkMask StorageClass parameter. The gateway only auto-creates listeners
// on addresses inside that mask, and node.Spec.PodCIDR does not always match the Pod's real subnet.
func getNVMeofGatewayNetworkMask(f *framework.Framework) string {
	opt := metav1.ListOptions{
		LabelSelector: "app=ceph-nvmeof-gateway",
	}

	pods, err := f.ClientSet.CoreV1().Pods(rookNamespace).List(context.TODO(), opt)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(pods.Items).Should(HaveLen(1))

	gatewayPod := &pods.Items[0]

	// Ask the kernel (inside the gateway container) for the interface that holds the Pod IP,
	// as JSON: [{"addr_info":[{"local":"10.244.1.6","prefixlen":24,...}],...}]
	output, stderr, err := execCommandInPodWithName(
		f, "ip -j -4 addr show to "+gatewayPod.Status.PodIP, gatewayPod.Name, "nvmeof-gateway", rookNamespace)
	Expect(err).ShouldNot(HaveOccurred(), "failed to read gateway interface: %s", stderr)

	// Only the fields needed to build the CIDR are decoded.
	var ifaces []struct {
		AddrInfo []struct {
			Local     string `json:"local"`
			PrefixLen int    `json:"prefixlen"`
		} `json:"addr_info"`
	}
	err = json.Unmarshal([]byte(output), &ifaces)
	Expect(err).ShouldNot(HaveOccurred(), "failed to parse ip output %q", output)
	// The "to <ip>" filter limits the result to the single interface with the Pod IP.
	Expect(ifaces).Should(HaveLen(1), "unexpected ip output %q", output)
	Expect(ifaces[0].AddrInfo).ShouldNot(BeEmpty(), "no address in ip output %q", output)

	// ParseCIDR masks the host bits: 10.244.1.6/24 -> 10.244.1.0/24.
	addr := ifaces[0].AddrInfo[0]
	_, subnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", addr.Local, addr.PrefixLen))
	Expect(err).ShouldNot(HaveOccurred())

	networkMask := subnet.String()
	framework.Logf("gateway Pod %q IP %q uses networkMask %q",
		gatewayPod.Name, gatewayPod.Status.PodIP, networkMask)

	return networkMask
}
