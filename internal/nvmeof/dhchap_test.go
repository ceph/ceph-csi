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

package nvmeof

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckDHCHAPSupport(t *testing.T) {
	t.Parallel()

	const supported = "instance=-1,cntlid=-1,transport=%s,traddr=%s,trsvcid=%s,nqn=%s,hostnqn=%s," +
		"duplicate_connect,discovery,dhchap_secret=%s,dhchap_ctrl_secret=%s,tls,concat\n"

	tests := []struct {
		name    string
		content *string
		wantErr error
		// wantReadErr is set when the support can not be determined (neither nil nor ErrDHCHAPUnsupported).
		wantReadErr bool
	}{
		{
			name:    "both dhchap options offered",
			content: ptr(supported),
		},
		{
			name:    "no dhchap options offered",
			content: ptr("instance=-1,cntlid=-1,transport=%s,traddr=%s,nqn=%s,hostnqn=%s,tls\n"),
			wantErr: ErrDHCHAPUnsupported,
		},
		{
			name:    "only dhchap_secret offered",
			content: ptr("instance=-1,nqn=%s,dhchap_secret=%s,tls\n"),
			wantErr: ErrDHCHAPUnsupported,
		},
		{
			name:    "only dhchap_ctrl_secret offered",
			content: ptr("instance=-1,nqn=%s,dhchap_ctrl_secret=%s,tls\n"),
			wantErr: ErrDHCHAPUnsupported,
		},
		{
			name:        "empty options list",
			content:     ptr("\n"),
			wantReadErr: true,
		},
		{
			name:        "device can not be read",
			wantReadErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			device := filepath.Join(t.TempDir(), "nvme-fabrics")
			if tt.content != nil {
				require.NoError(t, os.WriteFile(device, []byte(*tt.content), 0o600))
			}

			err := checkDHCHAPSupport(device)

			switch {
			case tt.wantReadErr:
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrDHCHAPUnsupported)
			case tt.wantErr != nil:
				require.ErrorIs(t, err, tt.wantErr)
			default:
				require.NoError(t, err)
			}
		})
	}
}

func ptr(s string) *string {
	return &s
}
