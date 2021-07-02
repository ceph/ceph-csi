package e2e

import (
	"fmt"
	"io/ioutil"
	"strings"

	"k8s.io/kubernetes/test/e2e/framework"
)

// deployer interface modify the templates on demand and also help
// to create or delete templates.
type deployer interface {
	replaceNamespaceInTemplate(string) (string, error)
	runAction(string, string, string) error
}

type deploy struct{}

func (d deploy) replaceNamespaceInTemplate(filePath string) (string, error) {
	read, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return strings.ReplaceAll(string(read), "namespace: default", fmt.Sprintf("namespace: %s", cephCSINamespace)), nil
}

func (d deploy) runAction(template, action, ns string) error {
	data, err := d.replaceNamespaceInTemplate(template)
	if err != nil {
		return fmt.Errorf("failed to read content from %s with error %w", template, err)
	}
	if action == "create" {
		_, err = framework.RunKubectlInput(cephCSINamespace, data, ns, action, "-f", "-")
	}
	if action == "delete" {
		_, err = framework.RunKubectlInput(cephCSINamespace, data, "--ignore-not-found=true", ns, action, "-f", "-")
	}
	if err != nil {
		return fmt.Errorf("failed to %s %q with error %w", action, template, err)
	}
	return err
}
