/*
Copyright 2018 The Kubernetes Authors.

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

package cephfs

import "fmt"

const (
	credUserId   = "userID"
	credUserKey  = "userKey"
	credAdminId  = "adminID"
	credAdminKey = "adminKey"
	credMonitors = "monitors"
)

type credentials struct {
	id  string
	key string
}

func getCredentials(idField, keyField string, secrets map[string]string) (*credentials, error) {
	var (
		c  = &credentials{}
		ok bool
	)

	if c.id, ok = secrets[idField]; !ok {
		return nil, fmt.Errorf("missing ID field '%s' in secrets", idField)
	}

	if c.key, ok = secrets[keyField]; !ok {
		return nil, fmt.Errorf("missing key field '%s' in secrets", keyField)
	}

	return c, nil
}

func getUserCredentials(secrets map[string]string) (*credentials, error) {
	return getCredentials(credUserId, credUserKey, secrets)
}

func getAdminCredentials(secrets map[string]string) (*credentials, error) {
	return getCredentials(credAdminId, credAdminKey, secrets)
}

func getMonValFromSecret(secrets map[string]string) (string, error) {
	if mons, ok := secrets[credMonitors]; ok {
		return mons, nil
	}
	return "", fmt.Errorf("missing %q", credMonitors)
}
