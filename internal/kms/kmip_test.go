/*
Copyright 2022 The Ceph-CSI Authors.

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

package kms

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	kmip "github.com/gemalto/kmip-go"
	"github.com/gemalto/kmip-go/kmip14"
	"github.com/stretchr/testify/require"
)

func TestKMIPKMSRegistered(t *testing.T) {
	t.Parallel()
	_, ok := kmsManager.providers[kmsTypeKMIP]
	require.True(t, ok)
}

func TestParseTLSMinVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version string
		want    uint16
		wantErr bool
	}{
		{kmipTLSVersion12, tls.VersionTLS12, false},
		{kmipTLSVersion13, tls.VersionTLS13, false},
		{"", 0, true},
		{"1.1", 0, true},
		{"1.4", 0, true},
		{"TLSv1.3", 0, true},
		{"1.30", 0, true},
	}

	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			t.Parallel()

			version, err := parseTLSMinVersion(test.version)
			if test.wantErr {
				require.ErrorIs(t, err, errConfigOptionInvalid)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.want, version)
		})
	}
}

// TestKMIPConnectTLSMinVersion verifies that requiring TLS 1.3 refuses a KMIP
// server that stops at TLS 1.2, and that the default keeps accepting it.
func TestKMIPConnectTLSMinVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		clientMin   uint16 // the TLS minimum the client requires
		serverMax   uint16 // the highest version the server offers
		wantVersion uint16 // the version to settle on, 0 to be refused
	}{
		{"default against a TLS 1.2 server", tls.VersionTLS12, tls.VersionTLS12, tls.VersionTLS12},
		{"default against a TLS 1.3 server", tls.VersionTLS12, tls.VersionTLS13, tls.VersionTLS13},
		{"TLS 1.3 required, server offers it", tls.VersionTLS13, tls.VersionTLS13, tls.VersionTLS13},
		{"TLS 1.3 required, server stops at 1.2", tls.VersionTLS13, tls.VersionTLS12, 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			certificate, caCertPool := kmipTestCertificate(t)

			kms := &kmipKMS{
				endpoint: kmipTestServer(t, &certificate, test.serverMax),
				tlsConfig: &tls.Config{
					MinVersion:   test.clientMin,
					RootCAs:      caCertPool,
					Certificates: []tls.Certificate{certificate},
				},
			}

			conn, err := kms.connect()
			if test.wantVersion == 0 {
				require.ErrorContains(t, err, "failed to dial kmip connection endpoint")
				require.ErrorContains(t, err, "protocol version not supported")

				return
			}

			require.NoError(t, err)

			defer conn.Close() //nolint:errcheck // test cleanup

			require.Equal(t, test.wantVersion, conn.ConnectionState().Version)
		})
	}
}

// kmipTestServer runs an in-process KMIP server behind TLS, offering no more
// than the given TLS version, and returns its endpoint. It answers the
// DiscoverVersions exchange that connect() performs, and nothing else.
func kmipTestServer(t *testing.T, certificate *tls.Certificate, maxVersion uint16) string {
	t.Helper()

	version := kmip.ProtocolVersion{
		ProtocolVersionMajor: protocolMajor,
		ProtocolVersionMinor: protocolMinor,
	}

	mux := &kmip.OperationMux{}
	mux.Handle(kmip14.OperationDiscoverVersions, &kmip.DiscoverVersionsHandler{
		SupportedVersions: []kmip.ProtocolVersion{version},
	})

	server := &kmip.Server{
		Handler: &kmip.StandardProtocolHandler{
			ProtocolVersion: version,
			MessageHandler:  mux,
		},
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   maxVersion,
		Certificates: []tls.Certificate{*certificate},
	})
	require.NoError(t, err)

	go server.Serve(listener) //nolint:errcheck // the error on shutdown is irrelevant

	t.Cleanup(func() {
		server.Close() //nolint:errcheck // test cleanup
	})

	return listener.Addr().String()
}

// kmipTestCertificate returns a self-signed certificate for 127.0.0.1 and a CA
// pool that trusts it.
func kmipTestCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "kmip-tls-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(parsed)

	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        parsed,
	}, caCertPool
}
