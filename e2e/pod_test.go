/*
Copyright 2026 The Ceph-CSI Authors.

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

package e2e

import "testing"

const (
	// sha512sum of "hello\n".
	sha512Hello = "e7c22b994c59d9cf2b48e549b1e24666636045930d3da7c1acb299d1c3b7f931" +
		"f94aae41edda2c2b207a36e10f8bcb8d45223e54878f5b316e7ce3b6bc019629"
	// sha512sum of "hello\nworld\n".
	sha512HelloWorld = "595beeb13e543b7107c182bb23ac7766bd62ab3122e250e90573eb540f895201" +
		"22150e5f69bb99140bc47ab526c9b7b2a3b2402b09b44b7391501a149876765c"
)

func TestParseSHA512sum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		sha512sumOut string
		want         string
		wantErr      bool
	}{
		{
			name:         "checksum and file",
			sha512sumOut: sha512Hello + "  /mnt/test\n",
			want:         sha512Hello,
		},
		{
			name:         "leading and trailing whitespace",
			sha512sumOut: "\n  " + sha512Hello + "  /mnt/test  \n",
			want:         sha512Hello,
		},
		{
			name:         "empty output",
			sha512sumOut: "",
			wantErr:      true,
		},
		{
			name:         "whitespace only",
			sha512sumOut: " \n\t\n",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseSHA512sum(tt.sha512sumOut)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseSHA512sum() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseSHA512sum() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseSHA512sumDifferentContents ensures different file contents result in
// different checksums instead of a constant value (see ceph-csi#1848).
func TestParseSHA512sumDifferentContents(t *testing.T) {
	t.Parallel()

	sumHello, err := parseSHA512sum(sha512Hello + "  /mnt/test\n")
	if err != nil {
		t.Fatalf("parseSHA512sum() failed for first content: %v", err)
	}
	sumHelloWorld, err := parseSHA512sum(sha512HelloWorld + "  /mnt/test\n")
	if err != nil {
		t.Fatalf("parseSHA512sum() failed for second content: %v", err)
	}

	if sumHello == sumHelloWorld {
		t.Errorf("checksums for different contents are equal: %q", sumHello)
	}
}
