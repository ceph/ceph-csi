/*
Copyright 2023 The CephCSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetNodeLabels(nodeName string) (map[string]string, error) {
	client, err := NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("can not get node %q information, failed "+
			"to connect to Kubernetes: %w", nodeName, err)
	}

	node, err := client.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node %q information: %w", nodeName, err)
	}

	return node.GetLabels(), nil
}
