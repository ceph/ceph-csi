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
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilnet "k8s.io/apimachinery/pkg/util/net"
)

func isRetryableAPIError(err error) bool {
	// These errors may indicate a transient error that we can retry in tests.
	if apierrors.IsInternalError(err) || apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) ||
		apierrors.IsTooManyRequests(err) || utilnet.IsProbableEOF(err) || utilnet.IsConnectionReset(err) ||
		utilnet.IsConnectionRefused(err) {
		return true
	}

	// If the error sends the Retry-After header, we respect it as an explicit confirmation we should retry.
	if _, shouldRetry := apierrors.SuggestsClientDelay(err); shouldRetry {
		return true
	}

	// "etcdserver: request timed out" does not seem to match the timeout errors above
	if strings.Contains(err.Error(), "etcdserver: request timed out") {
		return true
	}

	// "unable to upgrade connection" happens occasionally when executing commands in Pods
	if strings.Contains(err.Error(), "unable to upgrade connection") {
		return true
	}

	// "transport is closing" is an internal gRPC err, we can not use ErrConnClosing
	if strings.Contains(err.Error(), "transport is closing") {
		return true
	}

	// "transport: missing content-type field" is an error that sometimes
	// is returned while talking to the kubernetes-api-server. There does
	// not seem to be a public error constant for this.
	if strings.Contains(err.Error(), "transport: missing content-type field") {
		return true
	}

	return false
}

//nolint:lll // sample output cannot be split into multiple lines.
/*
getStdErr will extract the stderror and returns the actual error message

Sample kubectl output:

error running /usr/local/bin/kubectl --server=https://192.168.39.67:8443 --kubeconfig=***** --namespace=default create -f -:
Command stdout:

stderr:
Error from server (AlreadyExists): error when creating "STDIN": services "csi-rbdplugin-provisioner" already exists
Error from server (AlreadyExists): error when creating "STDIN": deployments.apps "csi-rbdplugin-provisioner" already exists

error:
exit status 1

Sample message returned from this function:

Error from server (AlreadyExists): error when creating "STDIN": services "csi-rbdplugin-provisioner" already exists
Error from server (AlreadyExists): error when creating "STDIN": deployments.apps "csi-rbdplugin-provisioner" already exists.
*/
func getStdErr(errString string) string {
	stdErrStr := "stderr:\n"
	errStr := "error:\n"
	stdErrPosition := strings.Index(errString, stdErrStr)
	if stdErrPosition == -1 {
		return ""
	}

	errPosition := strings.Index(errString, errStr)
	if errPosition == -1 {
		return ""
	}

	stdErrPositionLength := stdErrPosition + len(stdErrStr)
	if stdErrPositionLength >= errPosition {
		return ""
	}

	return errString[stdErrPosition+len(stdErrStr) : errPosition]
}

// isAlreadyExistsCLIError checks for already exists error from kubectl CLI.
func isAlreadyExistsCLIError(err error) bool {
	if err == nil {
		return false
	}
	// if multiple resources already exists. each error is separated by newline
	stdErr := getStdErr(err.Error())
	if stdErr == "" {
		return false
	}

	stdErrs := strings.Split(stdErr, "\n")
	for _, s := range stdErrs {
		// If the string is just a new line continue
		if strings.TrimSuffix(s, "\n") == "" {
			continue
		}
		// Ignore warnings
		if strings.Contains(s, "Warning") {
			continue
		}
		// Resource already exists error message
		if !strings.Contains(s, "Error from server (AlreadyExists)") {
			return false
		}
	}

	return true
}

// isNotFoundCLIError checks for "is not found" error from kubectl CLI.
func isNotFoundCLIError(err error) bool {
	if err == nil {
		return false
	}
	// if multiple resources already exists. each error is separated by newline
	stdErr := getStdErr(err.Error())
	if stdErr == "" {
		return false
	}

	stdErrs := strings.Split(stdErr, "\n")
	for _, s := range stdErrs {
		// If the string is just a new line continue
		if strings.TrimSuffix(s, "\n") == "" {
			continue
		}
		// Ignore warnings
		if strings.Contains(s, "Warning") {
			continue
		}
		// Resource not found error message
		if !strings.Contains(s, "Error from server (NotFound)") {
			return false
		}
	}

	return true
}
