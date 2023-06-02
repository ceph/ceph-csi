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
	"errors"
	"fmt"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

func createNodeLabel(f *framework.Framework, labelKey, labelValue string) error {
	// NOTE: This makes all nodes (in a multi-node setup) in the test take
	//       the same label values, which is fine for the test
	nodes, err := f.ClientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list node: %w", err)
	}
	for i := range nodes.Items {
		e2enode.AddOrUpdateLabelOnNode(f.ClientSet, nodes.Items[i].Name, labelKey, labelValue)
	}

	return nil
}

func deleteNodeLabel(c kubernetes.Interface, labelKey string) error {
	nodes, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list node: %w", err)
	}
	for i := range nodes.Items {
		e2enode.RemoveLabelOffNode(c, nodes.Items[i].Name, labelKey)
	}

	return nil
}

func checkNodeHasLabel(c kubernetes.Interface, labelKey, labelValue string) error {
	nodes, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list node: %w", err)
	}
	for i := range nodes.Items {
		e2enode.ExpectNodeHasLabel(context.TODO(), c, nodes.Items[i].Name, labelKey, labelValue)
	}

	return nil
}

// List all nodes in the cluster (we have one), and return the IP-address.
// Possibly need to add a selector, pick the node where a Pod is running?
func getKubeletIP(c kubernetes.Interface) (string, error) {
	nodes, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list node: %w", err)
	}

	for _, address := range nodes.Items[0].Status.Addresses {
		if address.Type == core.NodeInternalIP {
			return address.Address, nil
		}
	}

	return "", errors.New("could not find internal IP for node")
}
