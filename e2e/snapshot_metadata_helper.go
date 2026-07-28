/*
Copyright 2026 The Ceph-CSI Authors.

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
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"os/exec"
	"time"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	. "github.com/onsi/gomega"
	authv1 "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	storageutils "k8s.io/kubernetes/test/e2e/storage/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	grpcMetadata "google.golang.org/grpc/metadata"
)

const (
	smsDriverName    = "rbd.csi.ceph.com"
	smsAudience      = smsDriverName
	smsTLSSecretName = "csi-snapshot-metadata-tls"
	smsServiceName   = "csi-snapshot-metadata"
	smsTestSAName    = "sms-e2e-tester"
	smsClusterRole   = "sms-e2e-tester"
	smsLocalPort     = 9443
	smsSidecarPort   = 50051
	smsContainerName = "csi-snapshot-metadata"
	smsCRDManifest   = "vendor/k8s.io/kubernetes/test/e2e/testing-manifests/" +
		"storage-csi/external-snapshot-metadata/cbt.storage.k8s.io_snapshotmetadataservices.yaml"
)

// smsInfra holds references needed during teardown.
type smsInfra struct {
	caCertPEM []byte
}

// generateSMSTLS generates a self-signed CA and server certificate for the SMS sidecar.
func generateSMSTLS(driverNamespace string) (
	caCert *x509.Certificate, caKey *rsa.PrivateKey,
	serverCertPEM, serverKeyPEM, caCertPEM []byte,
	err error,
) {
	const keySize = 4096

	caKey, err = rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to generate CA key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: smsServiceName},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to create CA cert: %w", err)
	}

	caCert, err = x509.ParseCertificate(caCertBytes)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to parse CA cert: %w", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to generate server key: %w", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: smsServiceName},
		DNSNames: []string{
			smsServiceName,
			fmt.Sprintf("%s.%s", smsServiceName, driverNamespace),
			fmt.Sprintf("%s.%s.svc", smsServiceName, driverNamespace),
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(60 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	srvCertBytes, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to create server cert: %w", err)
	}

	serverCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srvCertBytes})
	serverKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})
	caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})

	return caCert, caKey, serverCertPEM, serverKeyPEM, caCertPEM, nil
}

// setupSnapshotMetadataInfra performs all BeforeAll steps for SMS e2e tests.
// Returns the CA cert PEM for gRPC client usage.
func setupSnapshotMetadataInfra(
	clientSet kubernetes.Interface,
	dynClient dynamic.Interface,
	operatorNamespace, testNamespace string,
) (*smsInfra, error) {
	ctx := context.TODO()

	// 1. Generate TLS certificates
	_, _, certPEM, keyPEM, caCertPEM, err := generateSMSTLS(operatorNamespace)
	if err != nil {
		return nil, err
	}

	// 2. Create TLS secret
	tlsSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      smsTLSSecretName,
			Namespace: operatorNamespace,
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": certPEM,
			"tls.key": keyPEM,
		},
	}
	if _, err := clientSet.CoreV1().Secrets(operatorNamespace).Create(ctx, tlsSecret, metav1.CreateOptions{}); err != nil {
		return nil, fmt.Errorf("failed to create TLS secret: %w", err)
	}
	framework.Logf("Created TLS secret %s", smsTLSSecretName)

	// 3. Patch OperatorConfig CR - triggers sidecar injection
	if err := patchOperatorConfigWithTLS(operatorNamespace, smsTLSSecretName); err != nil {
		return nil, fmt.Errorf("failed to patch OperatorConfig: %w", err)
	}
	framework.Logf("Patched OperatorConfig with tls-key volume")

	// 4. Wait for ctrlplugin rollout
	if err := waitForDeploymentComplete(clientSet, operatorRBDDeploymentName, operatorNamespace, deployTimeout); err != nil {
		return nil, fmt.Errorf("ctrlplugin rollout failed: %w", err)
	}

	// 5. Verify sidecar running
	if err := verifySidecarRunning(clientSet, operatorRBDDeploymentName, operatorNamespace); err != nil {
		return nil, fmt.Errorf("sidecar verification failed: %w", err)
	}
	framework.Logf("Sidecar container verified running")

	// 6. Create K8s Service
	if err := createSnapshotMetadataService(ctx, clientSet, operatorNamespace, operatorRBDDeploymentName); err != nil {
		return nil, fmt.Errorf("failed to create SMS service: %w", err)
	}

	// 7. Install SMS CRD and wait for it to be established
	if err := installSMSCRD(); err != nil {
		return nil, fmt.Errorf("failed to install SMS CRD: %w", err)
	}
	if err := waitForSMSCRDReady(dynClient); err != nil {
		return nil, fmt.Errorf("SMS CRD not ready: %w", err)
	}

	// 8. Create SMS CR
	if err := createSnapshotMetadataServiceCR(dynClient, operatorNamespace, caCertPEM); err != nil {
		return nil, fmt.Errorf("failed to create SMS CR: %w", err)
	}

	// 9. Create test SA + RBAC
	if err := createSnapshotMetadataRBAC(clientSet, smsTestSAName, testNamespace); err != nil {
		return nil, fmt.Errorf("failed to create RBAC: %w", err)
	}

	return &smsInfra{
		caCertPEM: caCertPEM,
	}, nil
}

// cleanupSnapshotMetadataInfra tears down all resources in reverse order.
func cleanupSnapshotMetadataInfra(
	clientSet kubernetes.Interface,
	dynClient dynamic.Interface,
	operatorNamespace, testNamespace string,
) {
	ctx := context.TODO()

	deleteSnapshotMetadataRBAC(clientSet, smsTestSAName, testNamespace)
	deleteSnapshotMetadataServiceCR(dynClient)
	if err := deleteSMSCRD(); err != nil {
		framework.Logf("Warning: failed to delete SMS CRD: %v", err)
	}

	err := clientSet.CoreV1().Services(operatorNamespace).Delete(ctx, smsServiceName, metav1.DeleteOptions{})
	if err != nil && !apierrs.IsNotFound(err) {
		framework.Logf("Warning: failed to delete service %s: %v", smsServiceName, err)
	}

	if err := unpatchOperatorConfigTLS(operatorNamespace); err != nil {
		framework.Logf("Warning: failed to unpatch OperatorConfig: %v", err)
	}

	if err := waitForDeploymentComplete(clientSet, operatorRBDDeploymentName, operatorNamespace, deployTimeout); err != nil {
		framework.Logf("Warning: ctrlplugin rollout after unpatch: %v", err)
	}

	err = clientSet.CoreV1().Secrets(operatorNamespace).Delete(ctx, smsTLSSecretName, metav1.DeleteOptions{})
	if err != nil && !apierrs.IsNotFound(err) {
		framework.Logf("Warning: failed to delete TLS secret: %v", err)
	}
}

// patchOperatorConfigWithTLS patches the OperatorConfig CR to add a
// tls-key volume, triggering operator sidecar injection.
func patchOperatorConfigWithTLS(namespace, tlsSecretName string) error {
	patch := fmt.Sprintf(`{"spec":{"driverSpecDefaults":{"controllerPlugin":{"volumes":[{"volume":{"name":"tls-key","secret":{"secretName":%q}},"mount":{"name":"tls-key","mountPath":"/tmp/certificates","readOnly":true}}]}}}}`, tlsSecretName)

	args := []string{
		"operatorconfigs.csi.ceph.io",
		OperatorConfigName,
		"--type=merge",
		"-p",
		patch,
	}

	return retryKubectlArgs(namespace, kubectlPatch, deployTimeout, args...)
}

// unpatchOperatorConfigTLS removes the controllerPlugin volumes.
func unpatchOperatorConfigTLS(namespace string) error {
	args := []string{
		"operatorconfigs.csi.ceph.io",
		OperatorConfigName,
		"--type=json",
		"-p",
		`[{"op":"remove","path":"/spec/driverSpecDefaults/controllerPlugin/volumes"}]`,
	}

	return retryKubectlArgs(namespace, kubectlPatch, deployTimeout, args...)
}

// verifySidecarRunning checks ctrlplugin pods have the sidecar container running.
func verifySidecarRunning(clientSet kubernetes.Interface, deploymentName, namespace string) error {
	timeout := time.Duration(deployTimeout) * time.Minute

	return wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(ctx context.Context) (bool, error) {
		deploy, err := clientSet.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			return false, err
		}

		pods, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(deploy.Spec.Selector),
		})
		if err != nil {
			if isRetryableAPIError(err) {
				return false, nil
			}
			return false, err
		}

		if len(pods.Items) == 0 {
			return false, nil
		}

		for i := range pods.Items {
			found := false
			for j := range pods.Items[i].Status.ContainerStatuses {
				cs := &pods.Items[i].Status.ContainerStatuses[j]
				if cs.Name == smsContainerName {
					found = true
					if !cs.Ready || cs.State.Running == nil {
						return false, nil
					}
				}
			}
			if !found {
				return false, nil
			}
		}

		return true, nil
	})
}

// createSnapshotMetadataService creates the K8s Service for the sidecar.
func createSnapshotMetadataService(ctx context.Context, clientSet kubernetes.Interface, namespace, deploymentName string) error {
	deploy, err := clientSet.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment %s: %w", deploymentName, err)
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      smsServiceName,
			Namespace: namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "snapshot-metadata",
					Port:       smsLocalPort,
					Protocol:   v1.ProtocolTCP,
					TargetPort: intstr.FromInt32(smsSidecarPort),
				},
			},
			Selector: deploy.Spec.Selector.MatchLabels,
		},
	}

	if _, err := clientSet.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create service %s: %w", smsServiceName, err)
	}

	framework.Logf("Created SMS service %s in %s", smsServiceName, namespace)
	return nil
}

// installSMSCRD applies the vendored SnapshotMetadataService CRD manifest.
func installSMSCRD() error {
	return retryKubectlArgs("", kubectlCreate, deployTimeout, "-f", smsCRDManifest)
}

// deleteSMSCRD removes the SnapshotMetadataService CRD.
func deleteSMSCRD() error {
	return retryKubectlArgs("", kubectlDelete, deployTimeout, "-f", smsCRDManifest)
}

// waitForSMSCRDReady polls until the SnapshotMetadataService GVR is served by the API server.
func waitForSMSCRDReady(dynClient dynamic.Interface) error {
	timeout := time.Duration(deployTimeout) * time.Minute
	return wait.PollUntilContextTimeout(context.TODO(), 2*time.Second, timeout, true, func(_ context.Context) (bool, error) {
		_, err := dynClient.Resource(smsGVR).List(context.TODO(), metav1.ListOptions{Limit: 1})
		if err != nil {
			framework.Logf("SMS CRD not ready yet: %v", err)
			return false, nil
		}
		return true, nil
	})
}

var smsGVR = schema.GroupVersionResource{
	Group:    storageutils.SnapshotMetadataServiceGroup,
	Version:  storageutils.SnapshotMetadataServiceVersion,
	Resource: storageutils.SnapshotMetadataServiceResource,
}

// createSnapshotMetadataServiceCR creates the SnapshotMetadataService CR pointing to the sidecar service.
func createSnapshotMetadataServiceCR(dynClient dynamic.Interface, operatorNamespace string, caCertPEM []byte) error {
	sms := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       storageutils.SnapshotMetadataServiceKind,
			"apiVersion": storageutils.SnapshotMetadataServiceAPIVersion,
			"metadata": map[string]interface{}{
				"name": smsDriverName,
			},
			"spec": map[string]interface{}{
				"caCert":   caCertPEM,
				"audience": smsAudience,
				"address":  fmt.Sprintf("%s.%s:%d", smsServiceName, operatorNamespace, smsLocalPort),
			},
		},
	}

	ctx := context.TODO()
	if _, err := dynClient.Resource(smsGVR).Create(ctx, sms, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create SnapshotMetadataService CR: %w", err)
	}

	framework.Logf("Created SnapshotMetadataService CR %s", smsDriverName)
	return nil
}

// deleteSnapshotMetadataServiceCR deletes the SMS CR.
func deleteSnapshotMetadataServiceCR(dynClient dynamic.Interface) {
	ctx := context.TODO()
	err := dynClient.Resource(smsGVR).Delete(ctx, smsDriverName, metav1.DeleteOptions{})
	if err != nil && !apierrs.IsNotFound(err) {
		framework.Logf("Warning: failed to delete SMS CR %s: %v", smsDriverName, err)
	}
}

// createSnapshotMetadataRBAC creates SA, ClusterRole, ClusterRoleBinding.
func createSnapshotMetadataRBAC(clientSet kubernetes.Interface, saName, testNamespace string) error {
	ctx := context.TODO()

	sa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: testNamespace,
		},
	}
	if _, err := clientSet.CoreV1().ServiceAccounts(testNamespace).Create(ctx, sa, metav1.CreateOptions{}); err != nil && !apierrs.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create SA %s: %w", saName, err)
	}

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: smsClusterRole},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"snapshot.storage.k8s.io"},
				Resources: []string{"volumesnapshots", "volumesnapshotcontents"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"cbt.storage.k8s.io"},
				Resources: []string{"snapshotmetadataservices"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"serviceaccounts/token"},
				Verbs:     []string{"create", "get"},
			},
		},
	}
	if _, err := clientSet.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{}); err != nil && !apierrs.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create ClusterRole %s: %w", smsClusterRole, err)
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: smsClusterRole},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     smsClusterRole,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: testNamespace,
			},
		},
	}
	if _, err := clientSet.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{}); err != nil && !apierrs.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create ClusterRoleBinding %s: %w", smsClusterRole, err)
	}

	return nil
}

// deleteSnapshotMetadataRBAC removes RBAC resources.
func deleteSnapshotMetadataRBAC(clientSet kubernetes.Interface, saName, testNamespace string) {
	ctx := context.TODO()

	err := clientSet.RbacV1().ClusterRoleBindings().Delete(ctx, smsClusterRole, metav1.DeleteOptions{})
	if err != nil && !apierrs.IsNotFound(err) {
		framework.Logf("Warning: failed to delete CRB %s: %v", smsClusterRole, err)
	}

	err = clientSet.RbacV1().ClusterRoles().Delete(ctx, smsClusterRole, metav1.DeleteOptions{})
	if err != nil && !apierrs.IsNotFound(err) {
		framework.Logf("Warning: failed to delete CR %s: %v", smsClusterRole, err)
	}

	err = clientSet.CoreV1().ServiceAccounts(testNamespace).Delete(ctx, saName, metav1.DeleteOptions{})
	if err != nil && !apierrs.IsNotFound(err) {
		framework.Logf("Warning: failed to delete SA %s: %v", saName, err)
	}
}

// requestSAToken creates a TokenRequest for the SA with the specified audience.
func requestSAToken(clientSet kubernetes.Interface, saName, namespace, audience string) (string, error) {
	ctx := context.TODO()
	expirationSeconds := int64(3600)

	tr, err := clientSet.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, saName,
		&authv1.TokenRequest{
			Spec: authv1.TokenRequestSpec{
				Audiences:         []string{audience},
				ExpirationSeconds: &expirationSeconds,
			},
		}, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create token for SA %s: %w", saName, err)
	}

	return tr.Status.Token, nil
}

// portForwardHandle manages a kubectl port-forward process.
type portForwardHandle struct {
	cmd  *exec.Cmd
	done chan error
}

// startPortForward starts kubectl port-forward to the sidecar Service.
func startPortForward(namespace, serviceName string, localPort, remotePort int) (string, func(), error) {
	portMapping := fmt.Sprintf("%d:%d", localPort, remotePort)
	svcRef := fmt.Sprintf("svc/%s", serviceName)

	cmd := exec.Command("kubectl", "port-forward", "-n", namespace, svcRef, portMapping) //nolint:gosec // e2e test helper
	done := make(chan error, 1)

	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("failed to start port-forward: %w", err)
	}

	go func() {
		done <- cmd.Wait()
	}()

	time.Sleep(3 * time.Second)

	stop := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
	}

	localAddr := fmt.Sprintf("localhost:%d", localPort)
	framework.Logf("Port-forward started: %s -> %s/%s:%d", localAddr, namespace, serviceName, remotePort)

	return localAddr, stop, nil
}

// newSidecarGRPCClient creates a TLS gRPC connection to the sidecar.
func newSidecarGRPCClient(address string, caCertPEM []byte, operatorNamespace string) (*grpc.ClientConn, api.SnapshotMetadataClient, error) {
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCertPEM) {
		return nil, nil, fmt.Errorf("failed to parse CA certificate")
	}

	serverName := fmt.Sprintf("%s.%s", smsServiceName, operatorNamespace)

	tlsConfig := &tls.Config{
		RootCAs:    certPool,
		ServerName: serverName,
		MinVersion: tls.VersionTLS12,
	}

	conn, err := grpc.NewClient(address,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create gRPC client: %w", err)
	}

	return conn, api.NewSnapshotMetadataClient(conn), nil
}

// withBearerToken adds the bearer token to gRPC metadata.
func withBearerToken(ctx context.Context, token string) context.Context {
	return grpcMetadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

// collectAllocatedBlocks calls GetMetadataAllocated and collects all blocks.
func collectAllocatedBlocks(
	ctx context.Context,
	client api.SnapshotMetadataClient,
	token, namespace, snapshotName string,
	startingOffset int64,
	maxResults int32,
) ([]*api.BlockMetadata, int64, error) {
	authCtx := withBearerToken(ctx, token)

	stream, err := client.GetMetadataAllocated(authCtx, &api.GetMetadataAllocatedRequest{
		SecurityToken:  token,
		Namespace:      namespace,
		SnapshotName:   snapshotName,
		StartingOffset: startingOffset,
		MaxResults:     maxResults,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("GetMetadataAllocated failed: %w", err)
	}

	var (
		blocks      []*api.BlockMetadata
		volCapacity int64
	)

	for {
		resp, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			return nil, 0, fmt.Errorf("GetMetadataAllocated stream recv failed: %w", recvErr)
		}
		blocks = append(blocks, resp.GetBlockMetadata()...)
		if resp.GetVolumeCapacityBytes() > 0 {
			volCapacity = resp.GetVolumeCapacityBytes()
		}
	}

	return blocks, volCapacity, nil
}

// collectDeltaBlocks calls GetMetadataDelta and collects all blocks.
func collectDeltaBlocks(
	ctx context.Context,
	client api.SnapshotMetadataClient,
	token, namespace string,
	baseSnapshotHandle, targetSnapshotName string,
	startingOffset int64,
	maxResults int32,
) ([]*api.BlockMetadata, error) {
	authCtx := withBearerToken(ctx, token)

	stream, err := client.GetMetadataDelta(authCtx, &api.GetMetadataDeltaRequest{
		SecurityToken:      token,
		Namespace:          namespace,
		BaseSnapshotId:     baseSnapshotHandle,
		TargetSnapshotName: targetSnapshotName,
		StartingOffset:     startingOffset,
		MaxResults:         maxResults,
	})
	if err != nil {
		return nil, fmt.Errorf("GetMetadataDelta failed: %w", err)
	}

	var blocks []*api.BlockMetadata

	for {
		resp, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			return nil, fmt.Errorf("GetMetadataDelta stream recv failed: %w", recvErr)
		}
		blocks = append(blocks, resp.GetBlockMetadata()...)
	}

	return blocks, nil
}

// collectAllocatedResponses returns individual response messages for batching verification.
func collectAllocatedResponses(
	ctx context.Context,
	client api.SnapshotMetadataClient,
	token, namespace, snapshotName string,
	startingOffset int64,
	maxResults int32,
) ([]*api.GetMetadataAllocatedResponse, error) {
	authCtx := withBearerToken(ctx, token)

	stream, err := client.GetMetadataAllocated(authCtx, &api.GetMetadataAllocatedRequest{
		SecurityToken:  token,
		Namespace:      namespace,
		SnapshotName:   snapshotName,
		StartingOffset: startingOffset,
		MaxResults:     maxResults,
	})
	if err != nil {
		return nil, fmt.Errorf("GetMetadataAllocated failed: %w", err)
	}

	var responses []*api.GetMetadataAllocatedResponse

	for {
		resp, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			return nil, fmt.Errorf("GetMetadataAllocated stream recv failed: %w", recvErr)
		}
		responses = append(responses, resp)
	}

	return responses, nil
}

// writeBlocksAtOffsets writes random data at 4MB-aligned offsets on a
// raw block device via kubectl exec into the pod.
func writeBlocksAtOffsets(
	f *framework.Framework,
	podName, namespace, devicePath string,
	blockSizeMB int,
	offsets []int,
) error {
	for _, offset := range offsets {
		cmd := fmt.Sprintf("dd if=/dev/urandom of=%s bs=%dM count=1 seek=%d oflag=direct",
			devicePath, blockSizeMB, offset)

		opt := metav1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", podName),
		}
		_, _, err := execCommandInPod(f, cmd, namespace, &opt)
		if err != nil {
			return fmt.Errorf("failed to write block at offset %d in pod %s: %w", offset, podName, err)
		}
	}

	return nil
}

// getSnapshotHandle extracts the CSI snapshot handle from VolumeSnapshotContent.
func getSnapshotHandle(namespace, snapName string) (string, error) {
	vsc, err := getVolumeSnapshotContent(namespace, snapName)
	if err != nil {
		return "", fmt.Errorf("failed to get VolumeSnapshotContent for %s: %w", snapName, err)
	}

	if vsc.Status == nil || vsc.Status.SnapshotHandle == nil {
		return "", fmt.Errorf("snapshot handle not available for %s", snapName)
	}

	return *vsc.Status.SnapshotHandle, nil
}

// verifyBlockMetadata asserts blocks match expected offsets and sizes.
func verifyBlockMetadata(blocks []*api.BlockMetadata, expectedOffsets []int64, blockSize int64) {
	Expect(len(blocks)).To(Equal(len(expectedOffsets)),
		"block count mismatch: got %d, want %d", len(blocks), len(expectedOffsets))

	for i, block := range blocks {
		Expect(block.GetByteOffset()).To(Equal(expectedOffsets[i]),
			"block %d: offset mismatch", i)
		Expect(block.GetSizeBytes()).To(Equal(blockSize),
			"block %d: size mismatch", i)
	}
}
