package e2e

import (
	"context"
	"errors"

	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

func createNodeLabel(f *framework.Framework, labelKey, labelValue string) error {
	// NOTE: This makes all nodes (in a multi-node setup) in the test take
	//       the same label values, which is fine for the test
	nodes, err := f.ClientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range nodes.Items {
		framework.AddOrUpdateLabelOnNode(f.ClientSet, nodes.Items[i].Name, labelKey, labelValue)
	}
	return nil
}

func deleteNodeLabel(c kubernetes.Interface, labelKey string) error {
	nodes, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range nodes.Items {
		framework.RemoveLabelOffNode(c, nodes.Items[i].Name, labelKey)
	}
	return nil
}

func checkNodeHasLabel(c kubernetes.Interface, labelKey, labelValue string) error {
	nodes, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for i := range nodes.Items {
		framework.ExpectNodeHasLabel(c, nodes.Items[i].Name, labelKey, labelValue)
	}
	return nil
}

// List all nodes in the cluster (we have one), and return the IP-address.
// Possibly need to add a selector, pick the node where a Pod is running?
func getKubeletIP(c kubernetes.Interface) (string, error) {
	nodes, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	for _, address := range nodes.Items[0].Status.Addresses {
		if address.Type == core.NodeInternalIP {
			return address.Address, nil
		}
	}

	return "", errors.New("could not find internal IP for node")
}
