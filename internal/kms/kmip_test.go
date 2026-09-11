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
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
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

func TestIsKMIP(t *testing.T) {
	t.Parallel()
	require.True(t, IsKMIP(&kmipKMS{}))
	require.False(t, IsKMIP(secretsMetadataKMS{}))
}

func TestKMIPGetSecretUnsupportedWithCryptoRPC(t *testing.T) {
	t.Parallel()

	kms := &kmipKMS{
		useCryptoRPC: true,
	}

	_, err := kms.GetSecret(context.TODO(), "")
	require.ErrorIs(t, err, ErrGetSecretUnsupported)
	require.ErrorContains(t, err, kmipUseCryptoRPC)
}

func TestKMIPGetSecret(t *testing.T) {
	t.Parallel()

	keyUID := "kmip-test-key-uid"
	keyMaterial := make([]byte, 32)
	_, err := rand.Read(keyMaterial)
	require.NoError(t, err)

	endpoint, tlsConfig := startKMIPServer(t, keyUID, keyMaterial)

	kms := &kmipKMS{
		endpoint:         endpoint,
		tlsConfig:        tlsConfig,
		uniqueIdentifier: keyUID,
		readTimeout:      kmipDefaulfReadTimeout,
		writeTimeout:     kmipDefaultWriteTimeout,
		useCryptoRPC:     false,
	}

	secret, err := kms.GetSecret(context.TODO(), "")
	require.NoError(t, err)
	require.Equal(t, base64.StdEncoding.EncodeToString(keyMaterial), secret)

	// the passphrase has to be reproducible for the lifetime of the
	// volume, a second call must return the identical value
	again, err := kms.GetSecret(context.TODO(), "")
	require.NoError(t, err)
	require.Equal(t, secret, again)
}

// startKMIPServer runs an in-process KMIP server that serves the given
// symmetric key, and returns its endpoint together with a matching client
// TLS configuration.
func startKMIPServer(t *testing.T, keyUID string, keyMaterial []byte) (string, *tls.Config) {
	t.Helper()

	certificate, caCertPool := generateTestCertificate(t)

	mux := &kmip.OperationMux{}
	mux.Handle(kmip14.OperationDiscoverVersions, &kmip.DiscoverVersionsHandler{
		SupportedVersions: []kmip.ProtocolVersion{
			{
				ProtocolVersionMajor: protocolMajor,
				ProtocolVersionMinor: protocolMinor,
			},
		},
	})
	mux.Handle(kmip14.OperationGet, &kmip.GetHandler{
		Get: func(_ context.Context, payload *kmip.GetRequestPayload) (*kmip.GetResponsePayload, error) {
			if payload.UniqueIdentifier != keyUID {
				return nil, kmip.WithResultReason(
					errors.New("no such key"), kmip14.ResultReasonItemNotFound)
			}

			return &kmip.GetResponsePayload{
				ObjectType:       kmip14.ObjectTypeSymmetricKey,
				UniqueIdentifier: payload.UniqueIdentifier,
				SymmetricKey: &kmip.SymmetricKey{
					KeyBlock: kmip.KeyBlock{
						KeyFormatType: kmip14.KeyFormatTypeRaw,
						KeyValue: &kmip.KeyValue{
							KeyMaterial: keyMaterial,
						},
						CryptographicAlgorithm: kmip14.CryptographicAlgorithmAES,
						CryptographicLength:    len(keyMaterial) * 8,
					},
				},
			}, nil
		},
	})

	server := &kmip.Server{
		Handler: &kmip.StandardProtocolHandler{
			ProtocolVersion: kmip.ProtocolVersion{
				ProtocolVersionMajor: protocolMajor,
				ProtocolVersionMinor: protocolMinor,
			},
			MessageHandler: mux,
		},
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
	})
	require.NoError(t, err)

	go server.Serve(listener) //nolint:errcheck // the error on shutdown is irrelevant

	t.Cleanup(func() {
		server.Close() //nolint:errcheck // test cleanup
	})

	clientTLSConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{certificate},
	}

	return listener.Addr().String(), clientTLSConfig
}

// generateTestCertificate returns a self-signed certificate for 127.0.0.1
// and a CA pool that trusts it.
func generateTestCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "kmip-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	certificate := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  key,
	}

	parsed, err := x509.ParseCertificate(derBytes)
	require.NoError(t, err)

	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(parsed)

	return certificate, caCertPool
}
