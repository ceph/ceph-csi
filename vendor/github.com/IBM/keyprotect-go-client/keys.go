// Copyright 2019 IBM Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kp

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"time"
)

const (
	ReturnMinimal        PreferReturn = 0
	ReturnRepresentation PreferReturn = 1

	keyType = "application/vnd.ibm.kms.key+json"
)

var (
	preferHeaders = []string{"return=minimal", "return=representation"}
)

// PreferReturn designates the value for the "Prefer" header.
type PreferReturn int

// Key represents a key as returned by the KP API.
type Key struct {
	ID                  string      `json:"id,omitempty"`
	Name                string      `json:"name,omitempty"`
	Description         string      `json:"description,omitempty"`
	Type                string      `json:"type,omitempty"`
	Tags                []string    `json:"Tags,omitempty"`
	Aliases             []string    `json:"aliases,omitempty"`
	AlgorithmType       string      `json:"algorithmType,omitempty"`
	CreatedBy           string      `json:"createdBy,omitempty"`
	CreationDate        *time.Time  `json:"creationDate,omitempty"`
	LastUpdateDate      *time.Time  `json:"lastUpdateDate,omitempty"`
	LastRotateDate      *time.Time  `json:"lastRotateDate,omitempty"`
	KeyVersion          *KeyVersion `json:"keyVersion,omitempty" mapstructure:keyVersion`
	KeyRingID           string      `json:"keyRingID,omitempty"`
	Extractable         bool        `json:"extractable"`
	Expiration          *time.Time  `json:"expirationDate,omitempty"`
	Imported            bool        `json:"imported,omitempty"`
	Payload             string      `json:"payload,omitempty"`
	State               int         `json:"state,omitempty"`
	EncryptionAlgorithm string      `json:"encryptionAlgorithm,omitempty"`
	CRN                 string      `json:"crn,omitempty"`
	EncryptedNonce      string      `json:"encryptedNonce,omitempty"`
	IV                  string      `json:"iv,omitempty"`
	Deleted             *bool       `json:"deleted,omitempty"`
	DeletedBy           *string     `json:"deletedBy,omitempty"`
	DeletionDate        *time.Time  `json:"deletionDate,omitempty"`
	DualAuthDelete      *DualAuth   `json:"dualAuthDelete,omitempty"`
}

// KeysMetadata represents the metadata of a collection of keys.
type KeysMetadata struct {
	CollectionType string `json:"collectionType"`
	NumberOfKeys   int    `json:"collectionTotal"`
}

// Keys represents a collection of Keys.
type Keys struct {
	Metadata KeysMetadata `json:"metadata"`
	Keys     []Key        `json:"resources"`
}

// KeysActionRequest represents request parameters for a key action
// API call.
type KeysActionRequest struct {
	PlainText  string   `json:"plaintext,omitempty"`
	AAD        []string `json:"aad,omitempty"`
	CipherText string   `json:"ciphertext,omitempty"`
	Payload    string   `json:"payload,omitempty"`
}

type KeyVersion struct {
	ID           string     `json:"id,omitempty"`
	CreationDate *time.Time `json:"creationDate,omitempty"`
}

// CreateKey creates a new KP key.
func (c *Client) CreateKey(ctx context.Context, name string, expiration *time.Time, extractable bool) (*Key, error) {
	return c.CreateImportedKey(ctx, name, expiration, "", "", "", extractable)
}

// CreateImportedKey creates a new KP key from the given key material.
func (c *Client) CreateImportedKey(ctx context.Context, name string, expiration *time.Time, payload, encryptedNonce, iv string, extractable bool) (*Key, error) {
	key := Key{
		Name:        name,
		Type:        keyType,
		Extractable: extractable,
		Payload:     payload,
	}

	if payload != "" && encryptedNonce != "" && iv != "" {
		key.EncryptedNonce = encryptedNonce
		key.IV = iv
		key.EncryptionAlgorithm = importTokenEncAlgo
	}

	if expiration != nil {
		key.Expiration = expiration
	}

	return c.createKey(ctx, key)
}

// CreateRootKey creates a new, non-extractable key resource without
// key material.
func (c *Client) CreateRootKey(ctx context.Context, name string, expiration *time.Time) (*Key, error) {
	return c.CreateKey(ctx, name, expiration, false)
}

// CreateStandardKey creates a new, extractable key resource without
// key material.
func (c *Client) CreateStandardKey(ctx context.Context, name string, expiration *time.Time) (*Key, error) {
	return c.CreateKey(ctx, name, expiration, true)
}

// CreateImportedRootKey creates a new, non-extractable key resource
// with the given key material.
func (c *Client) CreateImportedRootKey(ctx context.Context, name string, expiration *time.Time, payload, encryptedNonce, iv string) (*Key, error) {
	return c.CreateImportedKey(ctx, name, expiration, payload, encryptedNonce, iv, false)
}

// CreateStandardKey creates a new, extractable key resource with the
// given key material.
func (c *Client) CreateImportedStandardKey(ctx context.Context, name string, expiration *time.Time, payload string) (*Key, error) {
	return c.CreateImportedKey(ctx, name, expiration, payload, "", "", true)
}

// CreateKeyWithAliaes creats a new key with alias names. A key can have a maximum of 5 alias names.
// For more information please refer to the links below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-create-root-keys#create-root-key-api
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-create-standard-keys#create-standard-key-api
func (c *Client) CreateKeyWithAliases(ctx context.Context, name string, expiration *time.Time, extractable bool, aliases []string) (*Key, error) {
	return c.CreateImportedKeyWithAliases(ctx, name, expiration, "", "", "", extractable, aliases)
}

// CreateImportedKeyWithAliases creates a new key with alias name and provided key material. A key can have a maximum of 5 alias names
// When importing root keys with import-token encryptedNonce and iv need to passed along with payload.
// Standard Keys cannot be imported with an import token hence only payload is required.
// For more information please refer to the links below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-import-root-keys#import-root-key-api
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-import-standard-keys#import-standard-key-gui
func (c *Client) CreateImportedKeyWithAliases(ctx context.Context, name string, expiration *time.Time, payload, encryptedNonce, iv string, extractable bool, aliases []string) (*Key, error) {
	key := Key{
		Name:        name,
		Type:        keyType,
		Extractable: extractable,
		Payload:     payload,
		Aliases:     aliases,
	}

	if !extractable && payload != "" && encryptedNonce != "" && iv != "" {
		key.EncryptedNonce = encryptedNonce
		key.IV = iv
		key.EncryptionAlgorithm = importTokenEncAlgo
	}

	if expiration != nil {
		key.Expiration = expiration
	}

	return c.createKey(ctx, key)
}

func (c *Client) createKey(ctx context.Context, key Key) (*Key, error) {
	keysRequest := Keys{
		Metadata: KeysMetadata{
			CollectionType: keyType,
			NumberOfKeys:   1,
		},
		Keys: []Key{key},
	}

	req, err := c.newRequest("POST", "keys", &keysRequest)
	if err != nil {
		return nil, err
	}

	keysResponse := Keys{}
	if _, err := c.do(ctx, req, &keysResponse); err != nil {
		return nil, err
	}

	return &keysResponse.Keys[0], nil
}

// GetKeys retrieves a collection of keys that can be paged through.
func (c *Client) GetKeys(ctx context.Context, limit int, offset int) (*Keys, error) {
	if limit == 0 {
		limit = 2000
	}

	req, err := c.newRequest("GET", "keys", nil)
	if err != nil {
		return nil, err
	}

	v := url.Values{}
	v.Set("limit", strconv.Itoa(limit))
	v.Set("offset", strconv.Itoa(offset))
	req.URL.RawQuery = v.Encode()

	keys := Keys{}
	_, err = c.do(ctx, req, &keys)
	if err != nil {
		return nil, err
	}

	return &keys, nil
}

// GetKey retrieves a key by ID or alias name.
// For more information on Key Alias please refer to the link below
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-retrieve-key
func (c *Client) GetKey(ctx context.Context, idOrAlias string) (*Key, error) {
	return c.getKey(ctx, idOrAlias, "keys/%s")
}

// GetKeyMetadata retrieves the metadata of a Key by ID or alias name.
// Note that the "/api/v2/keys/{id}/metadata" API does not return the payload,
// therefore the payload attribute in the Key pointer will always be empty.
// If you need the payload, you need to use the GetKey() function with the
// correct service access role.
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-manage-access#service-access-roles
func (c *Client) GetKeyMetadata(ctx context.Context, idOrAlias string) (*Key, error) {
	return c.getKey(ctx, idOrAlias, "keys/%s/metadata")
}

func (c *Client) getKey(ctx context.Context, id string, path string) (*Key, error) {
	keys := Keys{}

	req, err := c.newRequest("GET", fmt.Sprintf(path, id), nil)
	if err != nil {
		return nil, err
	}

	_, err = c.do(ctx, req, &keys)
	if err != nil {
		return nil, err
	}

	return &keys.Keys[0], nil
}

type CallOpt interface{}

type ForceOpt struct {
	Force bool
}

// DeleteKey deletes a key resource by specifying the ID of the key.
func (c *Client) DeleteKey(ctx context.Context, id string, prefer PreferReturn, callOpts ...CallOpt) (*Key, error) {

	req, err := c.newRequest("DELETE", fmt.Sprintf("keys/%s", id), nil)
	if err != nil {
		return nil, err
	}

	for _, opt := range callOpts {
		switch v := opt.(type) {
		case ForceOpt:
			params := url.Values{}
			params.Set("force", strconv.FormatBool(v.Force))
			req.URL.RawQuery = params.Encode()
		default:
			log.Printf("WARNING: Ignoring invalid CallOpt passed to DeleteKey: %v\n", v)
		}
	}

	req.Header.Set("Prefer", preferHeaders[prefer])

	keys := Keys{}
	_, err = c.do(ctx, req, &keys)
	if err != nil {
		return nil, err
	}

	if len(keys.Keys) > 0 {
		return &keys.Keys[0], nil
	}

	return nil, nil
}

// RestoreKey method reverts a delete key status to active key
// This method performs restore of any key from deleted state to active state.
// For more information please refer to the link below:
// https://cloud.ibm.com/dowcs/key-protect?topic=key-protect-restore-keys
func (c *Client) RestoreKey(ctx context.Context, id string) (*Key, error) {
	req, err := c.newRequest("POST", fmt.Sprintf("keys/%s/restore", id), nil)
	if err != nil {
		return nil, err
	}

	keysResponse := Keys{}

	_, err = c.do(ctx, req, &keysResponse)
	if err != nil {
		return nil, err
	}

	return &keysResponse.Keys[0], nil
}

// Wrap calls the wrap action with the given plain text.
func (c *Client) Wrap(ctx context.Context, id string, plainText []byte, additionalAuthData *[]string) ([]byte, error) {
	_, ct, err := c.wrap(ctx, id, plainText, additionalAuthData)
	return ct, err
}

// WrapCreateDEK calls the wrap action without plain text.
func (c *Client) WrapCreateDEK(ctx context.Context, id string, additionalAuthData *[]string) ([]byte, []byte, error) {
	return c.wrap(ctx, id, nil, additionalAuthData)
}

func (c *Client) wrap(ctx context.Context, id string, plainText []byte, additionalAuthData *[]string) ([]byte, []byte, error) {
	keysActionReq := &KeysActionRequest{}

	if plainText != nil {
		_, err := base64.StdEncoding.DecodeString(string(plainText))
		if err != nil {
			return nil, nil, err
		}
		keysActionReq.PlainText = string(plainText)
	}

	if additionalAuthData != nil {
		keysActionReq.AAD = *additionalAuthData
	}

	keysAction, err := c.doKeysAction(ctx, id, "wrap", keysActionReq)
	if err != nil {
		return nil, nil, err
	}

	pt := []byte(keysAction.PlainText)
	ct := []byte(keysAction.CipherText)

	return pt, ct, nil
}

// Unwrap is deprecated since it returns only plaintext and doesn't know how to handle rotation.
func (c *Client) Unwrap(ctx context.Context, id string, cipherText []byte, additionalAuthData *[]string) ([]byte, error) {
	plainText, _, err := c.UnwrapV2(ctx, id, cipherText, additionalAuthData)
	if err != nil {
		return nil, err
	}
	return plainText, nil
}

// Unwrap with rotation support.
func (c *Client) UnwrapV2(ctx context.Context, id string, cipherText []byte, additionalAuthData *[]string) ([]byte, []byte, error) {

	keysAction := &KeysActionRequest{
		CipherText: string(cipherText),
	}

	if additionalAuthData != nil {
		keysAction.AAD = *additionalAuthData
	}

	respAction, err := c.doKeysAction(ctx, id, "unwrap", keysAction)
	if err != nil {
		return nil, nil, err
	}

	plainText := []byte(respAction.PlainText)
	rewrapped := []byte(respAction.CipherText)

	return plainText, rewrapped, nil
}

// Rotate rotates a CRK.
func (c *Client) Rotate(ctx context.Context, id, payload string) error {

	actionReq := &KeysActionRequest{
		Payload: payload,
	}

	_, err := c.doKeysAction(ctx, id, "rotate", actionReq)
	if err != nil {
		return err
	}

	return nil
}

// Disable a key. The key will not be deleted but it will not be active
// and key operations cannot be performed on a disabled key.
// For more information can refer to the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-disable-keys
func (c *Client) DisableKey(ctx context.Context, id string) error {
	_, err := c.doKeysAction(ctx, id, "disable", nil)
	return err
}

// Enable a key. Only disabled keys can be enabled. After enable
// the key becomes active and key operations can be performed on it.
// Note: This does not recover Deleted keys.
// For more information can refer to the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-disable-keys#enable-api
func (c *Client) EnableKey(ctx context.Context, id string) error {
	_, err := c.doKeysAction(ctx, id, "enable", nil)
	return err
}

// InitiateDualAuthDelete sets a key for deletion. The key must be configured with a DualAuthDelete policy.
// After the key is set to deletion it can be deleted by another user who has Manager access.
// For more information refer to the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-delete-dual-auth-keys#set-key-deletion-api
func (c *Client) InitiateDualAuthDelete(ctx context.Context, id string) error {
	_, err := c.doKeysAction(ctx, id, "setKeyForDeletion", nil)
	return err
}

// CancelDualAuthDelete unsets the key for deletion. If a key is set for deletion, it can
// be prevented from getting deleted by unsetting the key for deletion.
// For more information refer to the Key Protect docs in the link below:
//https://cloud.ibm.com/docs/key-protect?topic=key-protect-delete-dual-auth-keys#unset-key-deletion-api
func (c *Client) CancelDualAuthDelete(ctx context.Context, id string) error {
	_, err := c.doKeysAction(ctx, id, "unsetKeyForDeletion", nil)
	return err
}

// doKeysAction calls the KP Client to perform an action on a key.
func (c *Client) doKeysAction(ctx context.Context, id string, action string, keysActionReq *KeysActionRequest) (*KeysActionRequest, error) {
	keyActionRsp := KeysActionRequest{}

	v := url.Values{}
	v.Set("action", action)

	req, err := c.newRequest("POST", fmt.Sprintf("keys/%s", id), keysActionReq)
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = v.Encode()

	_, err = c.do(ctx, req, &keyActionRsp)
	if err != nil {
		return nil, err
	}
	return &keyActionRsp, nil
}
