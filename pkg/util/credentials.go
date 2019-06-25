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
)

const (
	credUserID   = "userID"
	credUserKey  = "userKey"
	credAdminID  = "adminID"
	credAdminKey = "adminKey"
	credMonitors = "monitors"
)

type Credentials struct {
	ID  string
	Key string
}

func getCredentials(idField, keyField string, secrets map[string]string) (*Credentials, error) {
	var (
		c  = &Credentials{}
		ok bool
	)

	if c.ID, ok = secrets[idField]; !ok {
		return nil, fmt.Errorf("missing ID field '%s' in secrets", idField)
	}

	if c.Key, ok = secrets[keyField]; !ok {
		return nil, fmt.Errorf("missing key field '%s' in secrets", keyField)
	}

	return c, nil
}

func GetUserCredentials(secrets map[string]string) (*Credentials, error) {
	return getCredentials(credUserID, credUserKey, secrets)
}

func GetAdminCredentials(secrets map[string]string) (*Credentials, error) {
	return getCredentials(credAdminID, credAdminKey, secrets)
}

func GetMonValFromSecret(secrets map[string]string) (string, error) {
	if mons, ok := secrets[credMonitors]; ok {
		return mons, nil
	}
	return "", fmt.Errorf("missing %q", credMonitors)
}
