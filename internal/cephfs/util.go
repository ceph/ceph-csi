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

package cephfs

import (
	"context"
	"time"

	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
)

func parseTime(ctx context.Context, createTime time.Time) (*timestamp.Timestamp, error) {
	tm, err := ptypes.TimestampProto(createTime)
	if err != nil {
		log.ErrorLog(ctx, "failed to convert time %s %v", createTime, err)

		return tm, err
	}

	return tm, nil
}
