package util

import (
	"fmt"
	"os"
)

const (
	// podNamespaceEnv ENV should be set in the cephcsi container.
	podNamespaceEnv = "POD_NAMESPACE"
)

// GetPodNamespace reads the POD_NAMESPACE environment variable to discover the
// namespace the driver pod is running in.
func GetPodNamespace() (string, error) {
	ns := os.Getenv(podNamespaceEnv)
	if ns == "" {
		return "", fmt.Errorf("%q is not set in the environment", podNamespaceEnv)
	}

	return ns, nil
}
