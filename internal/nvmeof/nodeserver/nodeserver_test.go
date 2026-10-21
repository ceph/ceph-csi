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

package nodeserver

import (
	"errors"
	"slices"
	"strings"
	"testing"

	utilexec "k8s.io/utils/exec"
)

// fakeCmd returns a canned result from CombinedOutput(), the only method
// used by mount.ResizeFs. The embedded interface is intentionally nil.
type fakeCmd struct {
	utilexec.Cmd

	out []byte
	err error
}

func (c *fakeCmd) CombinedOutput() ([]byte, error) {
	return c.out, c.err
}

// fakeExec records the executed commands and answers them based on the
// command name.
type fakeExec struct {
	utilexec.Interface

	// blkid output, e.g. "TYPE=ext4\n"
	blkidOut string
	// error returned by resize2fs
	resizeErr error
	// executed command names
	calls []string
}

func (e *fakeExec) Command(cmd string, args ...string) utilexec.Cmd {
	e.calls = append(e.calls, cmd)

	switch cmd {
	case "blockdev":
		// --getro: device is writable
		return &fakeCmd{out: []byte("0\n")}
	case "blkid":
		return &fakeCmd{out: []byte(e.blkidOut)}
	case "resize2fs":
		return &fakeCmd{err: e.resizeErr}
	}

	return &fakeCmd{err: errors.New("unexpected command " + cmd)}
}

func (e *fakeExec) called(cmd string) bool {
	return slices.Contains(e.calls, cmd)
}

func TestResizeStagePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		exec       *fakeExec
		wantErr    string
		wantResize bool
	}{
		{
			name:       "existing ext4 filesystem is resized",
			exec:       &fakeExec{blkidOut: "TYPE=ext4\n"},
			wantResize: true,
		},
		{
			name: "unformatted device is not resized",
			exec: &fakeExec{blkidOut: ""},
		},
		{
			name:       "resize failure is returned",
			exec:       &fakeExec{blkidOut: "TYPE=ext4\n", resizeErr: errors.New("boom")},
			wantErr:    "resize failed on staging path",
			wantResize: true,
		},
		{
			name:    "unsupported filesystem is returned as an error",
			exec:    &fakeExec{blkidOut: "TYPE=ntfs\n"},
			wantErr: "need resize check failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := resizeStagePath(tt.exec, "/dev/nvme0n1", "/staging")

			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}

			if got := tt.exec.called("resize2fs"); got != tt.wantResize {
				t.Errorf("resize2fs called = %v, want %v", got, tt.wantResize)
			}
		})
	}
}
