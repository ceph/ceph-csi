package e2e

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

func createNamespace(c kubernetes.Interface, name string) error {
	timeout := time.Duration(deployTimeout) * time.Minute
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_, err := c.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	if err != nil && !apierrs.IsAlreadyExists(err) {
		return err
	}

	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		_, err := c.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			e2elog.Logf("Error getting namespace: '%s': %v", name, err)
			if apierrs.IsNotFound(err) {
				return false, nil
			}
			if isRetryableAPIError(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func deleteNamespace(c kubernetes.Interface, name string) error {
	timeout := time.Duration(deployTimeout) * time.Minute
	err := c.CoreV1().Namespaces().Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil && !apierrs.IsNotFound(err) {
		return err
	}
	return wait.PollImmediate(poll, timeout, func() (bool, error) {
		_, err = c.CoreV1().Namespaces().Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			e2elog.Logf("Error getting namespace: '%s': %v", name, err)
			if isRetryableAPIError(err) {
				return false, nil
			}
			return false, err
		}
		return false, nil
	})
}

func replaceNamespaceInTemplate(filePath string) (string, error) {
	read, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(read), "namespace: default", fmt.Sprintf("namespace: %s", cephCSINamespace)), nil
}
