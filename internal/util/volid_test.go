/*
Copyright 2019 Ceph-CSI authors.

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
	"testing"
)

type testTuple struct {
	vID           CSIIdentifier
	composedVolID string
	wantEnc       bool
	wantEncError  bool
	wantDec       bool
	wantDecError  bool
}

// TODO: Add more test tuples to test out other edge conditions.
var testData = []testTuple{
	{
		vID: CSIIdentifier{
			LocationID:      0xffff,
			EncodingVersion: 0xffff,
			ClusterID:       "01616094-9d93-4178-bf45-c7eac19e8b15",
			ObjectUUID:      "00000000-1111-2222-bbbb-cacacacacaca",
		},
		composedVolID: "ffff-0024-01616094-9d93-4178-bf45-c7eac19e8b15-000000000000ffff-00000000-1111-2222-bbbb-cacacacacaca",
		wantEnc:       true,
		wantEncError:  false,
		wantDec:       true,
		wantDecError:  false,
	},
}

func TestComposeDecomposeID(t *testing.T) {
	t.Parallel()
	var (
		err           error
		viDecompose   CSIIdentifier
		composedVolID string
	)

	for _, test := range testData {
		if test.wantEnc {
			composedVolID, err = test.vID.ComposeCSIID()

			if err != nil && !test.wantEncError {
				t.Errorf("Composing failed: want (%#v), got (%#v %#v)",
					test, composedVolID, err)
			}

			if err == nil && test.wantEncError {
				t.Errorf("Composing failed: want (%#v), got (%#v %#v)",
					test, composedVolID, err)
			}

			if !test.wantEncError && err == nil && composedVolID != test.composedVolID {
				t.Errorf("Composing failed: want (%#v), got (%#v %#v)",
					test, composedVolID, err)
			}
		}

		if test.wantDec {
			err = viDecompose.DecomposeCSIID(test.composedVolID)

			if err != nil && !test.wantDecError {
				t.Errorf("Decomposing failed: want (%#v), got (%#v %#v)",
					test, viDecompose, err)
			}

			if err == nil && test.wantDecError {
				t.Errorf("Decomposing failed: want (%#v), got (%#v %#v)",
					test, viDecompose, err)
			}

			if !test.wantDecError && err == nil && viDecompose != test.vID {
				t.Errorf("Decomposing failed: want (%#v), got (%#v %#v)",
					test, viDecompose, err)
			}
		}
	}
}
