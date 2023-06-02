/*
Copyright 2020 The Kubernetes Authors.

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
	"fmt"
	"testing"
)

// output from kubectl.
//
//nolint:lll // error string cannot be split into multiple lines as is a
func TestGetStdErr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		errString           string
		expected            string
		isAlreadyExistError bool
	}{
		{
			name: "stdErr output when error is found",
			errString: `
error running /usr/local/bin/kubectl --server=https://192.168.39.67:8443 --kubeconfig=***** --namespace=default create -f -:
Command stdout:

stderr:
Error from server (AlreadyExists): error when creating "STDIN": services "csi-rbdplugin-provisioner" already exists
Error from server (AlreadyExists): error when creating "STDIN": deployments.apps "csi-rbdplugin-provisioner" already exists

error:
exit status 1`,
			expected: `Error from server (AlreadyExists): error when creating "STDIN": services "csi-rbdplugin-provisioner" already exists
Error from server (AlreadyExists): error when creating "STDIN": deployments.apps "csi-rbdplugin-provisioner" already exists

`,
			isAlreadyExistError: true,
		},
		{
			name: "stdErr output when stderr: string is not found",
			errString: `
error running /usr/local/bin/kubectl --server=https://192.168.39.67:8443 --kubeconfig=***** --namespace=default create -f -:
Command stdout:

error:
exit status 1`,
			expected:            "",
			isAlreadyExistError: false,
		},
		{
			name: "stdErr output when error: string is not found",
			errString: `
error running /usr/local/bin/kubectl --server=https://192.168.39.67:8443 --kubeconfig=***** --namespace=default create -f -:
Command stdout:

stderr:
Error from server (AlreadyExists): error when creating "STDIN": services "csi-rbdplugin-provisioner" already exists
Error from server (AlreadyExists): error when creating "STDIN": deployments.apps "csi-rbdplugin-provisioner" already exists`,
			expected:            "",
			isAlreadyExistError: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := getStdErr(tt.errString); got != tt.expected {
				t.Errorf("getStdErr() output = %v, expected %v", got, tt.expected)
			}

			if got := isAlreadyExistsCLIError(fmt.Errorf("%s", tt.errString)); got != tt.isAlreadyExistError {
				t.Errorf("isAlreadyExistsCLIError() output = %v, expected %v", got, tt.isAlreadyExistError)
			}
		})
	}
}
