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
