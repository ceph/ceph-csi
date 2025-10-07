/*
Copyright 2025 The Ceph-CSI Authors.

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

package nvmeof

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatUUID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in  string
		out string
	}{
		{"438cb4a8ae90477485677ea1414cd3ac", "438cb4a8-ae90-4774-8567-7ea1414cd3ac"},
		{"4-3-8-c--b4a8ae90477485677ea1414cd3ac", "438cb4a8-ae90-4774-8567-7ea1414cd3ac"},
		{"---438cb4a8ae90477485677ea1414cd3ac---", "438cb4a8-ae90-4774-8567-7ea1414cd3ac"},
		{"invalid", "invalid"},
		{"", ""},
	}

	for _, test := range tests {
		out := formatUUID(test.in)
		require.Equal(t, test.out, out)
	}
}
