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

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateServiceAccountToken creates a token for the given ServiceAccount.
func CreateServiceAccountToken(namespace, name string) (*authenticationv1.TokenRequest, error) {
	client, err := NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Kubernetes: %w", err)
	}

	tokenRequest := &authenticationv1.TokenRequest{}
	tokenRequest, err = client.CoreV1().ServiceAccounts(namespace).CreateToken(
		context.TODO(),
		name,
		tokenRequest,
		metav1.CreateOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create token for ServiceAccount %s/%s: %w", namespace, name, err)
	}

	return tokenRequest, nil
}

// GetServiceAccount retrieves the ServiceAccount object for the given name and namespace.
func GetServiceAccount(namespace, name string) (*corev1.ServiceAccount, error) {
	client, err := NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Kubernetes: %w", err)
	}

	sa, err := client.CoreV1().ServiceAccounts(namespace).Get(context.TODO(),
		name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ServiceAccount %s/%s: %w", namespace, name, err)
	}

	return sa, nil
}
