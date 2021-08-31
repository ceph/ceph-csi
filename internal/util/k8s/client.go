/*
Copyright 2020 The CephCSI Authors.

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
	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewK8sClient create kubernetes client.
func NewK8sClient() (*kubernetes.Clientset, error) {
	var cfg *rest.Config
	var err error
	cPath := os.Getenv("KUBERNETES_CONFIG_PATH")
	if cPath != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", cPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster config from %q: %w", cPath, err)
		}
	} else {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster config: %w", err)
		}
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return client, nil
}
