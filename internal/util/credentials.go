/*
Copyright 2018 The Ceph-CSI Authors.

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

package util

import (
	"errors"
	"fmt"
	"os"
)

const (
	credUserID           = "userID"
	credUserKey          = "userKey"
	credAdminID          = "adminID"
	credAdminKey         = "adminKey"
	credMonitors         = "monitors"
	tmpKeyFileLocation   = "/tmp/csi/keys"
	tmpKeyFileNamePrefix = "keyfile-"
	migUserName          = "admin"
	migUserID            = "adminId"
	migUserKey           = "key"
)

// Credentials struct represents credentials to access the ceph cluster.
type Credentials struct {
	ID      string
	KeyFile string
}

func storeKey(key string) (string, error) {
	tmpfile, err := os.CreateTemp(tmpKeyFileLocation, tmpKeyFileNamePrefix)
	if err != nil {
		return "", fmt.Errorf("error creating a temporary keyfile: %w", err)
	}
	defer func() {
		if err != nil {
			// don't complain about unhandled error
			_ = os.Remove(tmpfile.Name())
		}
	}()

	if _, err = tmpfile.WriteString(key); err != nil {
		return "", fmt.Errorf("error writing key to temporary keyfile: %w", err)
	}

	keyFile := tmpfile.Name()
	if keyFile == "" {
		err = fmt.Errorf("error reading temporary filename for key: %w", err)

		return "", err
	}

	if err = tmpfile.Close(); err != nil {
		return "", fmt.Errorf("error closing temporary filename: %w", err)
	}

	return keyFile, nil
}

func newCredentialsFromSecret(idField, keyField string, secrets map[string]string) (*Credentials, error) {
	var (
		c  = &Credentials{}
		ok bool
	)

	if len(secrets) == 0 {
		return nil, errors.New("provided secret is empty")
	}
	if c.ID, ok = secrets[idField]; !ok {
		return nil, fmt.Errorf("missing ID field '%s' in secrets", idField)
	}

	key := secrets[keyField]
	if key == "" {
		return nil, fmt.Errorf("missing key field '%s' in secrets", keyField)
	}

	keyFile, err := storeKey(key)
	if err == nil {
		c.KeyFile = keyFile
	}

	return c, err
}

// DeleteCredentials removes the KeyFile.
func (cr *Credentials) DeleteCredentials() {
	// don't complain about unhandled error
	_ = os.Remove(cr.KeyFile)
}

// NewUserCredentials creates new user credentials from secret.
func NewUserCredentials(secrets map[string]string) (*Credentials, error) {
	return newCredentialsFromSecret(credUserID, credUserKey, secrets)
}

// NewAdminCredentials creates new admin credentials from secret.
func NewAdminCredentials(secrets map[string]string) (*Credentials, error) {
	return newCredentialsFromSecret(credAdminID, credAdminKey, secrets)
}

// GetMonValFromSecret returns monitors from secret.
func GetMonValFromSecret(secrets map[string]string) (string, error) {
	if mons, ok := secrets[credMonitors]; ok {
		return mons, nil
	}

	return "", fmt.Errorf("missing %q", credMonitors)
}

// ParseAndSetSecretMapFromMigSecret parse the secretmap from the migration request and return
// newsecretmap with the userID and userKey fields set.
func ParseAndSetSecretMapFromMigSecret(secretmap map[string]string) (map[string]string, error) {
	newSecretMap := make(map[string]string)
	// parse and set userKey
	if !isMigrationSecret(secretmap) {
		return nil, errors.New("passed secret map does not contain user key or it is nil")
	}
	newSecretMap[credUserKey] = secretmap[migUserKey]
	// parse and set the userID
	newSecretMap[credUserID] = migUserName
	if secretmap[migUserID] != "" {
		newSecretMap[credUserID] = secretmap[migUserID]
	}

	return newSecretMap, nil
}

// isMigrationSecret validates if the passed in secretmap is a secret
// of a migration volume request. The migration secret carry a field
// called `key` which is the equivalent of `userKey` which is what we
// check here for identifying the secret.
func isMigrationSecret(secrets map[string]string) bool {
	// the below 'nil' check is an extra measure as the request validators like
	// ValidateNodeStageVolumeRequest() already does the nil check, however considering
	// this function can be called independently with a map of secret values
	// it is good to have this check in place, also it gives clear error about this
	// was hit on migration request compared to general one.
	return len(secrets) != 0 && secrets[migUserKey] != ""
}

// NewUserCredentialsWithMigration takes secret map from the request and validate it is
// a migration secret, if yes, it continues to create CR from it after parsing the migration
// secret. If it is not a migration it will continue the attempt to create credentials from it
// without parsing the secret. This function returns credentials and error.
func NewUserCredentialsWithMigration(secrets map[string]string) (*Credentials, error) {
	if isMigrationSecret(secrets) {
		migSecret, err := ParseAndSetSecretMapFromMigSecret(secrets)
		if err != nil {
			return nil, err
		}
		secrets = migSecret
	}
	cr, cErr := NewUserCredentials(secrets)
	if cErr != nil {
		return nil, cErr
	}

	return cr, nil
}
