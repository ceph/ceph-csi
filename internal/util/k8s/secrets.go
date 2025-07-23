/*
Copyright 2025 The CephCSI Authors.

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

// GetSecret retrieves a Kubernetes Secret by its name and namespace.
// It returns a map of key-value pairs contained in the Secret's data.
// If the Secret does not exist or cannot be accessed, it returns an error.
func GetSecret(secretName, secretNamespace string) (map[string]string, error) {
	client, err := NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Kubernetes: %w", err)
	}

	secret, err := client.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %q in namespace %q information: %w", secretName, secretNamespace, err)
	}

	secretData := map[string]string{}
	for key, value := range secret.Data {
		secretData[key] = string(value)
	}

	return secretData, nil
}
