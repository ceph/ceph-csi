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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	pykmipManifest     = "pykmip.yaml"
	kmipDeploymentName = "kmip"

	// kmipCertsSecretName is mounted by the PyKMIP Deployment and holds
	// the TLS material for the server and the client.
	//
	// #nosec:G101, value not credential, name of a Kubernetes Secret.
	kmipCertsSecretName = "ceph-csi-kmip-certs"

	// kmipCredentialsName must match the default KMIP_SECRET_NAME of the
	// kmip KMS provider.
	//
	// #nosec:G101, value not credential, name of a Kubernetes Secret.
	kmipCredentialsName = "ceph-csi-kmip-credentials"
)

// kmipCertificates holds the PEM encoded TLS material for the PyKMIP server
// and its clients. PyKMIP identifies a client by the CN of its certificate
// and only lets the owner fetch a key, so the key-creating script and
// ceph-csi use the same client certificate.
type kmipCertificates struct {
	caCert     string
	serverCert string
	serverKey  string
	clientCert string
	clientKey  string
}

// deployKMIP deploys a PyKMIP server and provisions everything the kmip KMS
// provider needs: the TLS certificates, an AES key on the server and the
// credentials Secret naming that key.
func deployKMIP(f *framework.Framework, deployTimeout int) {
	certs, err := generateKMIPCertificates(cephCSINamespace)
	Expect(err).ShouldNot(HaveOccurred())

	err = createKMIPSecret(f, kmipCertsSecretName, map[string]string{
		"ca.crt":     certs.caCert,
		"server.crt": certs.serverCert,
		"server.key": certs.serverKey,
		"client.crt": certs.clientCert,
		"client.key": certs.clientKey,
	})
	Expect(err).ShouldNot(HaveOccurred())

	data, err := replaceNamespaceInTemplate(vaultExamplePath + pykmipManifest)
	if err != nil {
		logAndFail("failed to read content from %s: %v", vaultExamplePath+pykmipManifest, err)
	}
	err = retryKubectlInput(cephCSINamespace, kubectlCreate, data, deployTimeout)
	if err != nil {
		logAndFail("failed to create PyKMIP deployment: %v", err)
	}

	err = waitForDeploymentComplete(f.ClientSet, kmipDeploymentName, cephCSINamespace, deployTimeout)
	Expect(err).ShouldNot(HaveOccurred())

	uid, err := createKMIPKey(f, deployTimeout)
	Expect(err).ShouldNot(HaveOccurred())

	// the kmip KMS provider rejects a credentials Secret with missing or
	// unknown keys, these four are exactly what it expects
	err = createKMIPSecret(f, kmipCredentialsName, map[string]string{
		"CA_CERT":           certs.caCert,
		"CLIENT_CERT":       certs.clientCert,
		"CLIENT_KEY":        certs.clientKey,
		"UNIQUE_IDENTIFIER": uid,
	})
	Expect(err).ShouldNot(HaveOccurred())
}

// deleteKMIP removes the PyKMIP server and the Secrets that deployKMIP
// created, also when a failed setup only created some of them.
func deleteKMIP() {
	data, err := replaceNamespaceInTemplate(vaultExamplePath + pykmipManifest)
	if err != nil {
		logAndFail("failed to read content from %s: %v", vaultExamplePath+pykmipManifest, err)
	}
	err = retryKubectlInput(cephCSINamespace, kubectlDelete, data, deployTimeout)
	if err != nil {
		logAndFail("failed to delete PyKMIP deployment: %v", err)
	}

	err = retryKubectlArgs(
		cephCSINamespace,
		kubectlDelete,
		deployTimeout,
		"secret",
		kmipCertsSecretName,
		kmipCredentialsName,
		"--ignore-not-found=true")
	Expect(err).ShouldNot(HaveOccurred())
}

// createKMIPSecret creates a Secret with the given content, replacing a
// leftover Secret with the same name from an earlier run.
func createKMIPSecret(f *framework.Framework, name string, data map[string]string) error {
	err := retryKubectlArgs(
		cephCSINamespace,
		kubectlDelete,
		deployTimeout,
		"secret",
		name,
		"--ignore-not-found=true")
	if err != nil {
		return fmt.Errorf("failed to delete Secret %q: %w", name, err)
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		StringData: data,
	}
	_, err = f.ClientSet.CoreV1().Secrets(cephCSINamespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Secret %q: %w", name, err)
	}

	return nil
}

// createKMIPKey runs the key creating script inside the PyKMIP pod until it
// succeeds and returns the unique identifier of the created key. Retries
// leave extra keys on the server, which is harmless because the key is
// addressed by the returned unique identifier.
func createKMIPKey(f *framework.Framework, deployTimeout int) (string, error) {
	opt := metav1.ListOptions{
		LabelSelector: "app=kmip",
	}
	uid := ""
	timeout := time.Duration(deployTimeout) * time.Minute
	err := wait.PollUntilContextTimeout(context.TODO(), poll, timeout, true, func(_ context.Context) (bool, error) {
		stdOut, stdErr := execCommandInPodAndAllowFail(f, "python3 /etc/pykmip/create_key.py", cephCSINamespace, &opt)
		out := strings.TrimSpace(stdOut)
		if out == "" {
			framework.Logf("creating the KMIP key has not succeeded yet: %v", stdErr)

			return false, nil
		}

		// the unique identifier is on the last line of the output
		lines := strings.Split(out, "\n")
		uid = strings.TrimSpace(lines[len(lines)-1])

		return true, nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to create a key on the KMIP server: %w", err)
	}

	return uid, nil
}

// generateKMIPCertificates returns a fresh CA, a server certificate for the
// DNS names of the kmip Service in the given namespace, and one client
// certificate with a single CN and the clientAuth extended key usage, which
// PyKMIP requires of its clients.
func generateKMIPCertificates(namespace string) (*kmipCertificates, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate the CA key: %w", err)
	}

	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ceph-csi-kmip-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create the CA certificate: %w", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the CA certificate: %w", err)
	}

	serverCert, serverKey, err := signKMIPLeaf(caCert, caKey, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: kmipDeploymentName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: []string{
			kmipDeploymentName,
			kmipDeploymentName + "." + namespace,
			kmipDeploymentName + "." + namespace + ".svc.cluster.local",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create the server certificate: %w", err)
	}

	clientCert, clientKey, err := signKMIPLeaf(caCert, caKey, &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "ceph-csi-kmip-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create the client certificate: %w", err)
	}

	return &kmipCertificates{
		caCert:     pemEncodeCertificate(caDER),
		serverCert: serverCert,
		serverKey:  serverKey,
		clientCert: clientCert,
		clientKey:  clientKey,
	}, nil
}

// signKMIPLeaf creates a key pair for the given template, signs the
// certificate with the CA and returns both PEM encoded.
func signKMIPLeaf(
	caCert *x509.Certificate,
	caKey *ecdsa.PrivateKey,
	template *x509.Certificate,
) (string, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate a key: %w", err)
	}

	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to create a certificate: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal a key: %w", err)
	}

	keyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyDER,
	}))

	return pemEncodeCertificate(der), keyPEM, nil
}

func pemEncodeCertificate(der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	}))
}
