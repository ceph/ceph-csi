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
package util

import (
	"reflect"
	"testing"
)

func TestIsMigrationSecret(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		vc   map[string]string
		want bool
	}{
		{
			"proper migration secret key set",
			map[string]string{"key": "QVFBOFF2SlZheUJQRVJBQWgvS2cwT1laQUhPQno3akZwekxxdGc9PQ=="},
			true,
		},
		{
			"no key set",
			map[string]string{"": ""},
			false,
		},
	}
	for _, tt := range tests {
		newtt := tt
		t.Run(newtt.name, func(t *testing.T) {
			t.Parallel()
			if got := isMigrationSecret(newtt.vc); got != newtt.want {
				t.Errorf("isMigrationSecret() = %v, want %v", got, newtt.want)
			}
		})
	}
}

func TestParseAndSetSecretMapFromMigSecret(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		secretmap map[string]string
		want      map[string]string
		wantErr   bool
	}{
		{
			"valid migration secret key set",
			map[string]string{"key": "QVFBOFF2SlZheUJQRVJBQWgvS2cwT1laQUhPQno3akZwekxxdGc9PQ=="},
			map[string]string{"userKey": "QVFBOFF2SlZheUJQRVJBQWgvS2cwT1laQUhPQno3akZwekxxdGc9PQ==", "userID": "admin"},
			false,
		},
		{
			"migration secret key value nil",
			map[string]string{"key": ""},
			nil,
			true,
		},
		{
			"migration secret key field nil",
			map[string]string{"": ""},
			nil,
			true,
		},
		{
			"valid migration secret key and userID set",
			map[string]string{"key": "QVFBOFF2SlZheUJQRVJBQWgvS2cwT1laQUhPQno3akZwekxxdGc9PQ==", "adminId": "pooladmin"},
			map[string]string{"userKey": "QVFBOFF2SlZheUJQRVJBQWgvS2cwT1laQUhPQno3akZwekxxdGc9PQ==", "userID": "pooladmin"},
			false,
		},
	}
	for _, tt := range tests {
		newtt := tt
		t.Run(newtt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseAndSetSecretMapFromMigSecret(newtt.secretmap)
			if (err != nil) != newtt.wantErr {
				t.Errorf("ParseAndSetSecretMapFromMigSecret() error = %v, wantErr %v", err, newtt.wantErr)

				return
			}
			if !reflect.DeepEqual(got, newtt.want) {
				t.Errorf("ParseAndSetSecretMapFromMigSecret() got = %v, want %v", got, newtt.want)
			}
		})
	}
}
