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
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ceph/ceph-csi/internal/util/k8s"

	kmip "github.com/gemalto/kmip-go"
	"github.com/gemalto/kmip-go/kmip14"
	"github.com/gemalto/kmip-go/ttlv"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kmsTypeKMIP = "kmip"

	// kmipDefaulfReadTimeout is the default read network timeout.
	kmipDefaulfReadTimeout = 10

	// kmipDefaultWriteTimeout is the default write network timeout.
	kmipDefaultWriteTimeout = 10

	// KMIP version.
	protocolMajor = 1
	protocolMinor = 4

	// nonceSize is required to generate nonce for encrypting DEK.
	nonceSize = 16

	// kmipDefaultSecretsName is the default name of the Kubernetes Secret
	// that contains the credentials to access the KMIP server. The name of
	// the Secret can be configured by setting the `KMIP_SECRET_NAME`
	// option.
	//
	// #nosec:G101, value not credential, just references token.
	kmipDefaultSecretsName = "ceph-csi-kmip-credentials"

	kmipEndpoint      = "KMIP_ENDPOINT"
	kmipTLSServerName = "TLS_SERVER_NAME"
	kmipReadTimeOut   = "READ_TIMEOUT"
	kmipWriteTimeOut  = "WRITE_TIMEOUT"

	// The following options are part of the Kubernetes Secrets.
	//
	// #nosec:G101, value not credential, just configuration keys.
	kmipSecretNameKey    = "KMIP_SECRET_NAME"
	kmipCACert           = "CA_CERT"
	kmipCLientCert       = "CLIENT_CERT"
	kmipClientKey        = "CLIENT_KEY"
	kmipUniqueIdentifier = "UNIQUE_IDENTIFIER"
)

var _ = RegisterProvider(Provider{
	UniqueID:    kmsTypeKMIP,
	Initializer: initKMIPKMS,
})

type kmipKMS struct {
	// basic options to get the secret
	secretName string
	namespace  string

	// standard KMIP configuration options
	endpoint         string
	tlsConfig        *tls.Config
	uniqueIdentifier string
	readTimeout      uint8
	writeTimeout     uint8
}

func initKMIPKMS(args ProviderInitArgs) (EncryptionKMS, error) {
	kms := &kmipKMS{
		namespace: args.Namespace,
	}

	// get secret name if set, else use default.
	err := setConfigString(&kms.secretName, args.Config, kmipSecretNameKey)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	} else if errors.Is(err, errConfigOptionMissing) {
		kms.secretName = kmipDefaultSecretsName
	}

	err = setConfigString(&kms.endpoint, args.Config, kmipEndpoint)
	if err != nil {
		return nil, err
	}

	// optional
	serverName := ""
	err = setConfigString(&serverName, args.Config, kmipTLSServerName)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	}

	// optional
	timeout := kmipDefaulfReadTimeout
	err = setConfigInt(&timeout, args.Config, kmipReadTimeOut)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	}
	kms.readTimeout = uint8(timeout)

	// optional
	timeout = kmipDefaultWriteTimeout
	err = setConfigInt(&timeout, args.Config, kmipWriteTimeOut)
	if errors.Is(err, errConfigOptionInvalid) {
		return nil, err
	}
	kms.writeTimeout = uint8(timeout)

	// read the Kubernetes Secret with CA cert, client cert, client key
	// & key unique identifier.
	secrets, err := kms.getSecrets()
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets: %w", err)
	}

	caCert, found := secrets[kmipCACert]
	if !found {
		return nil, fmt.Errorf("%w: %s", errConfigOptionMissing, kmipCACert)
	}

	clientCert, found := secrets[kmipCLientCert]
	if !found {
		return nil, fmt.Errorf("%w: %s", errConfigOptionMissing, kmipCLientCert)
	}

	clientKey, found := secrets[kmipClientKey]
	if !found {
		return nil, fmt.Errorf("%w: %s", errConfigOptionMissing, kmipCLientCert)
	}

	kms.uniqueIdentifier, found = secrets[kmipUniqueIdentifier]
	if !found {
		return nil, fmt.Errorf("%w: %s", errConfigOptionMissing, kmipUniqueIdentifier)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(caCert))
	cert, err := tls.X509KeyPair([]byte(clientCert), []byte(clientKey))
	if err != nil {
		return nil, fmt.Errorf("invalid X509 key pair: %w", err)
	}

	kms.tlsConfig = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		ServerName:   serverName,
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{cert},
	}

	return kms, nil
}

// EncryptDEK uses the KMIP encrypt operation to encrypt the DEK.
func (kms *kmipKMS) EncryptDEK(_, plainDEK string) (string, error) {
	conn, err := kms.connect()
	if err != nil {
		return "", err
	}
	defer conn.Close()

	emd := encryptedMetedataDEK{}
	emd.Nonce, err = generateNonce(nonceSize)
	if err != nil {
		return "", fmt.Errorf("failed to generated nonce: %w", err)
	}

	respMsg, decoder, uniqueBatchItemID, err := kms.send(conn,
		kmip14.OperationEncrypt,
		EncryptRequestPayload{
			UniqueIdentifier: kms.uniqueIdentifier,
			Data:             []byte(plainDEK),
			CryptographicParameters: kmip.CryptographicParameters{
				PaddingMethod:          kmip14.PaddingMethodPKCS5,
				CryptographicAlgorithm: kmip14.CryptographicAlgorithmAES,
				BlockCipherMode:        kmip14.BlockCipherModeCBC,
			},
			IVCounterNonce: emd.Nonce,
		})
	if err != nil {
		return "", err
	}

	batchItem, err := kms.verifyResponse(respMsg, kmip14.OperationEncrypt, uniqueBatchItemID)
	if err != nil {
		return "", err
	}

	ttlvPayload, ok := batchItem.ResponsePayload.(ttlv.TTLV)
	if !ok {
		return "", errors.New("failed to parse responsePayload")
	}

	var encryptRespPayload EncryptResponsePayload
	err = decoder.DecodeValue(&encryptRespPayload, ttlvPayload)
	if err != nil {
		return "", err
	}

	emd.DEK = encryptRespPayload.Data
	emdData, err := json.Marshal(&emd)
	if err != nil {
		return "", fmt.Errorf("failed to convert "+
			"encryptedMetedataDEK to JSON: %w", err)
	}

	return string(emdData), nil
}

// DecryptDEK uses the KMIP decrypt operation  to decrypt the DEK.
func (kms *kmipKMS) DecryptDEK(_, encryptedDEK string) (string, error) {
	conn, err := kms.connect()
	if err != nil {
		return "", err
	}
	defer conn.Close()

	emd := encryptedMetedataDEK{}
	err = json.Unmarshal([]byte(encryptedDEK), &emd)
	if err != nil {
		return "", fmt.Errorf("failed to convert data to "+
			"encryptedMetedataDEK: %w", err)
	}

	respMsg, decoder, uniqueBatchItemID, err := kms.send(conn,
		kmip14.OperationDecrypt,
		DecryptRequestPayload{
			UniqueIdentifier: kms.uniqueIdentifier,
			Data:             emd.DEK,
			IVCounterNonce:   emd.Nonce,
			CryptographicParameters: kmip.CryptographicParameters{
				PaddingMethod:          kmip14.PaddingMethodPKCS5,
				CryptographicAlgorithm: kmip14.CryptographicAlgorithmAES,
				BlockCipherMode:        kmip14.BlockCipherModeCBC,
			},
		})
	if err != nil {
		return "", err
	}

	batchItem, err := kms.verifyResponse(respMsg, kmip14.OperationDecrypt, uniqueBatchItemID)
	if err != nil {
		return "", err
	}

	ttlvPayload, ok := batchItem.ResponsePayload.(ttlv.TTLV)
	if !ok {
		return "", errors.New("failed to parse responsePayload")
	}

	var decryptRespPayload DecryptRequestPayload
	err = decoder.DecodeValue(&decryptRespPayload, ttlvPayload)
	if err != nil {
		return "", err
	}

	return string(decryptRespPayload.Data), nil
}

func (kms *kmipKMS) Destroy() {
	// Nothing to do.
}

func (kms *kmipKMS) RequiresDEKStore() DEKStoreType {
	return DEKStoreMetadata
}

// getSecrets returns required options from the Kubernetes Secret.
func (kms *kmipKMS) getSecrets() (map[string]string, error) {
	c, err := k8s.NewK8sClient()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Kubernetes to "+
			"get Secret %s/%s: %w", kms.namespace, kms.secretName, err)
	}

	secret, err := c.CoreV1().Secrets(kms.namespace).Get(context.TODO(),
		kms.secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get Secret %s/%s: %w",
			kms.namespace, kms.secretName, err)
	}

	config := make(map[string]string)
	for k, v := range secret.Data {
		switch k {
		case kmipClientKey, kmipCLientCert, kmipCACert, kmipUniqueIdentifier:
			config[k] = string(v)
		default:
			return nil, fmt.Errorf("unsupported option for KMS "+
				"provider %q: %s", kmsTypeKMIP, k)
		}
	}

	return config, nil
}

// connect to the kmip endpoint, perform TLS and KMIP handshakes.
func (kms *kmipKMS) connect() (*tls.Conn, error) {
	conn, err := tls.Dial("tcp", kms.endpoint, kms.tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial kmip connection endpoint: %w", err)
	}
	if kms.readTimeout != 0 {
		err = conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(kms.readTimeout)))
		if err != nil {
			return nil, fmt.Errorf("failed to set read deadline: %w", err)
		}
	}
	if kms.writeTimeout != 0 {
		err = conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(kms.writeTimeout)))
		if err != nil {
			return nil, fmt.Errorf("failed to set write deadline: %w", err)
		}
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()

	err = conn.Handshake()
	if err != nil {
		return nil, fmt.Errorf("failed to perform connection handshake: %w", err)
	}

	err = kms.discover(conn)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// discover performs KMIP discover operation.
// https://docs.oasis-open.org/kmip/spec/v1.4/kmip-spec-v1.4.html
// chapter 4.26.
func (kms *kmipKMS) discover(conn io.ReadWriter) error {
	respMsg, decoder, uniqueBatchItemID, err := kms.send(conn,
		kmip14.OperationDiscoverVersions,
		kmip.DiscoverVersionsRequestPayload{
			ProtocolVersion: []kmip.ProtocolVersion{
				{
					ProtocolVersionMajor: protocolMajor,
					ProtocolVersionMinor: protocolMinor,
				},
			},
		})
	if err != nil {
		return err
	}

	batchItem, err := kms.verifyResponse(
		respMsg,
		kmip14.OperationDiscoverVersions,
		uniqueBatchItemID)
	if err != nil {
		return err
	}

	ttlvPayload, ok := batchItem.ResponsePayload.(ttlv.TTLV)
	if !ok {
		return errors.New("failed to parse responsePayload")
	}

	var respDiscoverVersionsPayload kmip.DiscoverVersionsResponsePayload
	err = decoder.DecodeValue(&respDiscoverVersionsPayload, ttlvPayload)
	if err != nil {
		return err
	}

	if len(respDiscoverVersionsPayload.ProtocolVersion) != 1 {
		return fmt.Errorf("invalid len of discovered protocol versions %v expected 1",
			len(respDiscoverVersionsPayload.ProtocolVersion))
	}
	pv := respDiscoverVersionsPayload.ProtocolVersion[0]
	if pv.ProtocolVersionMajor != protocolMajor || pv.ProtocolVersionMinor != protocolMinor {
		return fmt.Errorf("invalid discovered protocol version %v.%v expected %v.%v",
			pv.ProtocolVersionMajor, pv.ProtocolVersionMinor, protocolMajor, protocolMinor)
	}

	return nil
}

// send sends KMIP operation over tls connection, returns
// kmip response message,
// ttlv Decoder to decode message into desired format,
// batchItem ID,
// and error.
func (kms *kmipKMS) send(
	conn io.ReadWriter,
	operation kmip14.Operation,
	payload interface{},
) (*kmip.ResponseMessage, *ttlv.Decoder, []byte, error) {
	biID := uuid.New()

	msg := kmip.RequestMessage{
		RequestHeader: kmip.RequestHeader{
			ProtocolVersion: kmip.ProtocolVersion{
				ProtocolVersionMajor: protocolMajor,
				ProtocolVersionMinor: protocolMinor,
			},
			BatchCount: 1,
		},
		BatchItem: []kmip.RequestBatchItem{
			{
				UniqueBatchItemID: biID[:],
				Operation:         operation,
				RequestPayload:    payload,
			},
		},
	}

	req, err := ttlv.Marshal(msg)
	if err != nil {
		return nil, nil, nil,
			fmt.Errorf("failed to ttlv marshal message: %w", err)
	}

	_, err = conn.Write(req)
	if err != nil {
		return nil, nil, nil,
			fmt.Errorf("failed to write request onto connection: %w", err)
	}

	decoder := ttlv.NewDecoder(bufio.NewReader(conn))
	resp, err := decoder.NextTTLV()
	if err != nil {
		return nil, nil, nil,
			fmt.Errorf("failed to read ttlv KMIP value: %w", err)
	}

	var respMsg kmip.ResponseMessage
	err = decoder.DecodeValue(&respMsg, resp)
	if err != nil {
		return nil, nil, nil,
			fmt.Errorf("failed to decode response value: %w", err)
	}

	return &respMsg, decoder, biID[:], nil
}

// verifyResponse verifies the response success and return the batch item.
func (kms *kmipKMS) verifyResponse(
	respMsg *kmip.ResponseMessage,
	operation kmip14.Operation,
	uniqueBatchItemID []byte,
) (*kmip.ResponseBatchItem, error) {
	if respMsg.ResponseHeader.BatchCount != 1 {
		return nil, fmt.Errorf("batch count %q should be \"1\"",
			respMsg.ResponseHeader.BatchCount)
	}
	if len(respMsg.BatchItem) != 1 {
		return nil, fmt.Errorf("batch Intems list len %q should be \"1\"",
			len(respMsg.BatchItem))
	}
	batchItem := respMsg.BatchItem[0]
	if operation != batchItem.Operation {
		return nil, fmt.Errorf("unexpected operation, real %q expected %q",
			batchItem.Operation, operation)
	}
	if !bytes.Equal(uniqueBatchItemID, batchItem.UniqueBatchItemID) {
		return nil, fmt.Errorf("unexpected uniqueBatchItemID, real %q expected %q",
			batchItem.UniqueBatchItemID, uniqueBatchItemID)
	}
	if kmip14.ResultStatusSuccess != batchItem.ResultStatus {
		return nil, fmt.Errorf("unexpected result status %q expected success %q,"+
			"result reason %q, result message %q",
			batchItem.ResultStatus, kmip14.ResultStatusSuccess,
			batchItem.ResultReason, batchItem.ResultMessage)
	}

	return &batchItem, nil
}

func (kms *kmipKMS) GetSecret(volumeID string) (string, error) {
	return "", ErrGetSecretUnsupported
}

// TODO: use the following structs from https://github.com/gemalto/kmip-go
// when https://github.com/ThalesGroup/kmip-go/issues/21 is resolved.
// refer: https://docs.oasis-open.org/kmip/spec/v1.4/kmip-spec-v1.4.html.
type EncryptRequestPayload struct {
	UniqueIdentifier        string
	CryptographicParameters kmip.CryptographicParameters
	Data                    []byte
	IVCounterNonce          []byte
}

type EncryptResponsePayload struct {
	UniqueIdentifier string
	Data             []byte
	IVCounterNonce   []byte
}

type DecryptRequestPayload struct {
	UniqueIdentifier        string
	CryptographicParameters kmip.CryptographicParameters
	Data                    []byte
	IVCounterNonce          []byte
}

type DecryptResponsePayload struct {
	UniqueIdentifier string
	Data             []byte
	IVCounterNonce   []byte
}
