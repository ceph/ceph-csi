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
	"fmt"
	"io/ioutil"
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
)

type Credentials struct {
	ID      string
	KeyFile string
}

func storeKey(key string) (string, error) {
	tmpfile, err := ioutil.TempFile(tmpKeyFileLocation, tmpKeyFileNamePrefix)
	if err != nil {
		return "", fmt.Errorf("error creating a temporary keyfile (%s)", err)
	}
	defer func() {
		if err != nil {
			// don't complain about unhandled error
			_ = os.Remove(tmpfile.Name())
		}
	}()

	if _, err = tmpfile.Write([]byte(key)); err != nil {
		return "", fmt.Errorf("error writing key to temporary keyfile (%s)", err)
	}

	keyFile := tmpfile.Name()
	if keyFile == "" {
		err = fmt.Errorf("error reading temporary filename for key (%s)", err)
		return "", err
	}

	if err = tmpfile.Close(); err != nil {
		return "", fmt.Errorf("error closing temporary filename (%s)", err)
	}

	return keyFile, nil
}

func newCredentialsFromSecret(idField, keyField string, secrets map[string]string) (*Credentials, error) {
	var (
		c  = &Credentials{}
		ok bool
	)

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

func (cr *Credentials) DeleteCredentials() {
	// don't complain about unhandled error
	_ = os.Remove(cr.KeyFile)
}

func NewUserCredentials(secrets map[string]string) (*Credentials, error) {
	return newCredentialsFromSecret(credUserID, credUserKey, secrets)
}

func NewAdminCredentials(secrets map[string]string) (*Credentials, error) {
	return newCredentialsFromSecret(credAdminID, credAdminKey, secrets)
}

func NewCredentials(id, key string) (*Credentials, error) {
	var c = &Credentials{}

	c.ID = id
	keyFile, err := storeKey(key)
	if err == nil {
		c.KeyFile = keyFile
	}

	return c, err
}

func GetMonValFromSecret(secrets map[string]string) (string, error) {
	if mons, ok := secrets[credMonitors]; ok {
		return mons, nil
	}
	return "", fmt.Errorf("missing %q", credMonitors)
}
