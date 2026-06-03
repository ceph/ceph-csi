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

package rbd

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
)

type mockError struct {
	code int
	msg  string
}

func (e *mockError) Error() string {
	return fmt.Sprintf("%s: code %d", e.msg, e.code)
}

func (e *mockError) ErrorCode() int {
	return e.code
}

func newMockError(code int, msg string) *mockError {
	return &mockError{
		code: code,
		msg:  msg,
	}
}

func Test_createDiffIterateByIDCB(t *testing.T) {
	t.Parallel()

	t.Run("collects single block without LUKS padding", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		changedBlocks := make([]*csi.BlockMetadata, 0)
		var sentBatches [][]*csi.BlockMetadata
		sendResponse := func(blocks []*csi.BlockMetadata) error {
			copied := make([]*csi.BlockMetadata, len(blocks))
			copy(copied, blocks)
			sentBatches = append(sentBatches, copied)

			return nil
		}

		cb := createDiffIterateByIDCB(ctx, &changedBlocks, 10, 0, sendResponse)

		// Simulate a single block callback from librbd
		ret := cb(4096, 8192, 1, nil)
		if ret != int(codes.OK) {
			t.Errorf("expected OK return code, got %d", ret)
		}
		if len(changedBlocks) != 1 {
			t.Fatalf("expected 1 changed block, got %d", len(changedBlocks))
		}
		if changedBlocks[0].GetByteOffset() != 4096 {
			t.Errorf("expected offset 4096, got %d", changedBlocks[0].GetByteOffset())
		}
		if changedBlocks[0].GetSizeBytes() != 8192 {
			t.Errorf("expected size 8192, got %d", changedBlocks[0].GetSizeBytes())
		}
		// Should not have sent any batches yet (below maxResults)
		if len(sentBatches) != 0 {
			t.Errorf("expected 0 sent batches, got %d", len(sentBatches))
		}
	})

	t.Run("adjusts offset for LUKS header padding", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		changedBlocks := make([]*csi.BlockMetadata, 0)
		sendResponse := func(_ []*csi.BlockMetadata) error { return nil }

		luksHeaderPadding := uint64(16777216) // 16MB LUKS header
		cb := createDiffIterateByIDCB(ctx, &changedBlocks, 10, luksHeaderPadding, sendResponse)

		// librbd reports offset including LUKS header, callback should subtract it
		ret := cb(16777216+4096, 8192, 1, nil)
		if ret != int(codes.OK) {
			t.Errorf("expected OK return code, got %d", ret)
		}
		if len(changedBlocks) != 1 {
			t.Fatalf("expected 1 changed block, got %d", len(changedBlocks))
		}
		// Offset should be adjusted: 16777216+4096 - 16777216 = 4096
		if changedBlocks[0].GetByteOffset() != 4096 {
			t.Errorf("expected adjusted offset 4096, got %d", changedBlocks[0].GetByteOffset())
		}
	})

	t.Run("batches at maxResults and resets slice", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		changedBlocks := make([]*csi.BlockMetadata, 0)
		var sentBatches [][]*csi.BlockMetadata
		sendResponse := func(blocks []*csi.BlockMetadata) error {
			copied := make([]*csi.BlockMetadata, len(blocks))
			copy(copied, blocks)
			sentBatches = append(sentBatches, copied)

			return nil
		}

		maxResults := int32(3)
		cb := createDiffIterateByIDCB(ctx, &changedBlocks, maxResults, 0, sendResponse)

		// Send exactly maxResults blocks
		for i := range 3 {
			ret := cb(uint64(i*4096), 4096, 1, nil)
			if ret != int(codes.OK) {
				t.Errorf("block %d: expected OK return code, got %d", i, ret)
			}
		}

		// Should have sent one batch
		if len(sentBatches) != 1 {
			t.Fatalf("expected 1 sent batch, got %d", len(sentBatches))
		}
		if len(sentBatches[0]) != 3 {
			t.Errorf("expected batch of 3 blocks, got %d", len(sentBatches[0]))
		}
		// changedBlocks should be reset (length 0, reusing underlying array)
		if len(changedBlocks) != 0 {
			t.Errorf("expected changedBlocks reset to 0, got %d", len(changedBlocks))
		}
	})
}

func Test_createDiffIterateByIDCB_errorAndEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("returns Unknown code on sendResponse error", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		changedBlocks := make([]*csi.BlockMetadata, 0)
		sendResponse := func(_ []*csi.BlockMetadata) error {
			return errors.New("stream broken")
		}

		maxResults := int32(1)
		cb := createDiffIterateByIDCB(ctx, &changedBlocks, maxResults, 0, sendResponse)

		// First block should trigger sendResponse (maxResults=1)
		ret := cb(0, 4096, 1, nil)
		if ret != int(codes.Unknown) {
			t.Errorf("expected Unknown return code on send failure, got %d", ret)
		}
	})

	t.Run("returns Canceled on context cancellation", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel() // Cancel immediately

		changedBlocks := make([]*csi.BlockMetadata, 0)
		sendResponse := func(_ []*csi.BlockMetadata) error { return nil }

		cb := createDiffIterateByIDCB(ctx, &changedBlocks, 10, 0, sendResponse)

		ret := cb(0, 4096, 1, nil)
		if ret != int(codes.Canceled) {
			t.Errorf("expected Canceled return code, got %d", ret)
		}
		// Should not have collected any blocks
		if len(changedBlocks) != 0 {
			t.Errorf("expected 0 changed blocks on cancellation, got %d", len(changedBlocks))
		}
	})

	t.Run("multiple batches with remainder", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		changedBlocks := make([]*csi.BlockMetadata, 0)
		var sentBatches [][]*csi.BlockMetadata
		sendResponse := func(blocks []*csi.BlockMetadata) error {
			copied := make([]*csi.BlockMetadata, len(blocks))
			copy(copied, blocks)
			sentBatches = append(sentBatches, copied)

			return nil
		}

		maxResults := int32(2)
		cb := createDiffIterateByIDCB(ctx, &changedBlocks, maxResults, 0, sendResponse)

		// Send 5 blocks: should produce 2 full batches + 1 remainder
		for i := range 5 {
			ret := cb(uint64(i*4096), 4096, 1, nil)
			if ret != int(codes.OK) {
				t.Errorf("block %d: expected OK return code, got %d", i, ret)
			}
		}

		// 2 full batches sent (blocks 0-1, blocks 2-3)
		if len(sentBatches) != 2 {
			t.Fatalf("expected 2 sent batches, got %d", len(sentBatches))
		}
		// 1 block remaining (block 4)
		if len(changedBlocks) != 1 {
			t.Errorf("expected 1 remaining block, got %d", len(changedBlocks))
		}
		if changedBlocks[0].GetByteOffset() != 4*4096 {
			t.Errorf("expected remaining block offset %d, got %d", 4*4096, changedBlocks[0].GetByteOffset())
		}
	})

	t.Run("zero blocks produces no batches", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		changedBlocks := make([]*csi.BlockMetadata, 0)
		sendCalled := false
		sendResponse := func(_ []*csi.BlockMetadata) error {
			sendCalled = true

			return nil
		}

		_ = createDiffIterateByIDCB(ctx, &changedBlocks, 10, 0, sendResponse)

		// Don't call the callback at all (no changed blocks from librbd)
		if sendCalled {
			t.Error("sendResponse should not be called when no blocks are produced")
		}
		if len(changedBlocks) != 0 {
			t.Errorf("expected 0 changed blocks, got %d", len(changedBlocks))
		}
	})
}

func Test_handleDiffIterateError(t *testing.T) {
	t.Parallel()
	type args struct {
		ctx context.Context
		err error
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "nil error",
			args: args{
				ctx: t.Context(),
				err: nil,
			},
			wantErr: false,
		},
		{
			name: "stream closed",
			args: args{
				ctx: t.Context(),
				err: newMockError(int(codes.Canceled), "stream closed"),
			},
			wantErr: false,
		},
		{
			name: "response send failure",
			args: args{
				ctx: t.Context(),
				err: newMockError(int(codes.Unknown), "failed to send response"),
			},
			wantErr: true,
		},
		{
			name: "unrecognized error code",
			args: args{
				ctx: t.Context(),
				err: newMockError(999, "unrecognized error code"),
			},
			wantErr: true,
		},
		{
			name: "non-ErrorCode error",
			args: args{
				ctx: t.Context(),
				err: errors.New("generic error"),
			},
			wantErr: true,
		},
		{
			name: "ok error code",
			args: args{
				ctx: t.Context(),
				err: newMockError(int(codes.OK), "ok error code"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := handleDiffIterateError(tt.args.err); (err != nil) != tt.wantErr {
				t.Errorf("handleDiffIterateError() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
