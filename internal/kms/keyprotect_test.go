/*
Copyright 2021 The Ceph-CSI Authors.

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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyProtectMetadataKMSRegistered(t *testing.T) {
	t.Parallel()
	_, ok := kmsManager.providers[kmsTypeKeyProtectMetadata]
	require.True(t, ok)
}

const (
	testKeyProtectCRK        = "0987654321"
	testKeyProtectInstanceID = "1234567890"
	// #nosec:G101, not a credential, only a test value.
	testKeyProtectAPIKey = "fake-api-key"
	// the volumeID is passed as additional authenticated data.
	testKeyProtectVolumeID = "volume-id"
	// base64 encoded DEK, as KeyProtect expects and returns it.
	testKeyProtectPlainDEK = "cGxhaW4tZGVr"
	// base64 encoded wrapped DEK, as KeyProtect returns it.
	testKeyProtectCipherDEK = "d3JhcHBlZC1kZWs="
	// response body keys of the wrap and unwrap key actions.
	testKeyProtectCiphertextKey = "ciphertext"
	testKeyProtectPlaintextKey  = "plaintext"
)

// keyProtectRecorder records what the KMS sent to the KeyProtect key action.
type keyProtectRecorder struct {
	method     string
	instanceID string
	request    keyActionRequest
	attempts   int
	err        error
}

// keyProtectTestOpts tunes the server that newKeyProtectTestKMS starts.
type keyProtectTestOpts struct {
	// tokenPath is appended to the server URL to form the configured
	// IBM_KP_TOKEN_URL. Defaults to keyProtectIAMTokenPath.
	tokenPath string
	// failures is the number of times the key action responds with a
	// retryable error before it serves reply.
	failures int
}

// newKeyProtectTestKMS returns a keyProtectKMS that talks to a server
// serving the IAM token endpoint and the given KeyProtect key action. The
// action records its request and responds with reply.
func newKeyProtectTestKMS(
	t *testing.T,
	action string,
	reply map[string]string,
	opts keyProtectTestOpts,
) (*keyProtectKMS, *keyProtectRecorder) {
	t.Helper()

	rec := &keyProtectRecorder{}

	mux := http.NewServeMux()
	// IamAuthenticator appends /identity/token to the IAM
	// base endpoint it was configured with.
	mux.HandleFunc("/identity/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprint(w, `{"access_token":"fake-token","token_type":"Bearer",`+
			`"expires_in":3600,"expiration":99999999999}`)
		rec.err = errors.Join(rec.err, err)
	})
	mux.HandleFunc("/api/v2/keys/"+testKeyProtectCRK+"/actions/"+action,
		func(w http.ResponseWriter, r *http.Request) {
			rec.attempts++
			rec.method = r.Method
			rec.instanceID = r.Header.Get("Bluemix-Instance")

			err := json.NewDecoder(r.Body).Decode(&rec.request)
			rec.err = errors.Join(rec.err, err)

			if rec.attempts <= opts.failures {
				// the SDK honors Retry-After, keep the test fast
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusServiceUnavailable)

				return
			}

			w.Header().Set("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(reply)
			rec.err = errors.Join(rec.err, err)
		})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	tokenPath := opts.tokenPath
	if tokenPath == "" {
		tokenPath = keyProtectIAMTokenPath
	}

	kms := &keyProtectKMS{
		serviceAPIKey:     testKeyProtectAPIKey,
		customerRootKey:   testKeyProtectCRK,
		serviceInstanceID: testKeyProtectInstanceID,
		baseURL:           server.URL,
		// the configured token URL carries the token path, which
		// getService() has to normalize before the authenticator uses it
		tokenURL: server.URL + tokenPath,
	}

	return kms, rec
}

func TestKeyProtectEncryptDEK(t *testing.T) {
	t.Parallel()

	kms, rec := newKeyProtectTestKMS(t, "wrap", map[string]string{
		testKeyProtectCiphertextKey: testKeyProtectCipherDEK,
	}, keyProtectTestOpts{})

	encryptedDEK, err := kms.EncryptDEK(t.Context(), testKeyProtectVolumeID, testKeyProtectPlainDEK)
	require.NoError(t, err)
	require.NoError(t, rec.err)

	require.Equal(t, http.MethodPost, rec.method)
	require.Equal(t, testKeyProtectInstanceID, rec.instanceID)
	require.Equal(t, testKeyProtectPlainDEK, rec.request.Plaintext)
	require.Equal(t, []string{testKeyProtectVolumeID}, rec.request.AAD)
	require.Empty(t, rec.request.Ciphertext)

	// the ciphertext gets base64 encoded once more before it is stored
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte(testKeyProtectCipherDEK)), encryptedDEK)
}

func TestKeyProtectDecryptDEK(t *testing.T) {
	t.Parallel()

	kms, rec := newKeyProtectTestKMS(t, "unwrap", map[string]string{
		testKeyProtectPlaintextKey: testKeyProtectPlainDEK,
	}, keyProtectTestOpts{})

	encryptedDEK := base64.StdEncoding.EncodeToString([]byte(testKeyProtectCipherDEK))
	plainDEK, err := kms.DecryptDEK(t.Context(), testKeyProtectVolumeID, encryptedDEK)
	require.NoError(t, err)
	require.NoError(t, rec.err)

	require.Equal(t, http.MethodPost, rec.method)
	require.Equal(t, testKeyProtectInstanceID, rec.instanceID)
	require.Equal(t, testKeyProtectCipherDEK, rec.request.Ciphertext)
	require.Equal(t, []string{testKeyProtectVolumeID}, rec.request.AAD)
	require.Empty(t, rec.request.Plaintext)

	require.Equal(t, testKeyProtectPlainDEK, plainDEK)
}

// TestKeyProtectTokenURLVariants verifies that every form IBM_KP_TOKEN_URL
// could have been configured with still authenticates.
func TestKeyProtectTokenURLVariants(t *testing.T) {
	t.Parallel()

	for _, tokenPath := range []string{"/oidc/token", "/identity/token", ""} {
		t.Run("tokenPath="+tokenPath, func(t *testing.T) {
			t.Parallel()

			kms, rec := newKeyProtectTestKMS(t, "wrap", map[string]string{
				testKeyProtectCiphertextKey: testKeyProtectCipherDEK,
			}, keyProtectTestOpts{tokenPath: tokenPath})

			_, err := kms.EncryptDEK(t.Context(), testKeyProtectVolumeID, testKeyProtectPlainDEK)
			require.NoError(t, err)
			require.NoError(t, rec.err)
			require.Equal(t, 1, rec.attempts)
		})
	}
}

// TestKeyProtectRetriesTransientFailure verifies that a retryable KeyProtect
// response is retried, like the old client.
func TestKeyProtectRetriesTransientFailure(t *testing.T) {
	t.Parallel()

	kms, rec := newKeyProtectTestKMS(t, "wrap", map[string]string{
		testKeyProtectCiphertextKey: testKeyProtectCipherDEK,
	}, keyProtectTestOpts{failures: 2})

	encryptedDEK, err := kms.EncryptDEK(t.Context(), testKeyProtectVolumeID, testKeyProtectPlainDEK)
	require.NoError(t, err)
	require.NoError(t, rec.err)

	// two 503s then success
	require.Equal(t, 3, rec.attempts)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte(testKeyProtectCipherDEK)), encryptedDEK)
}
