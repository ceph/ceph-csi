package e2e

import (
	"context"

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
