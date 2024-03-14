/*
Copyright 2024 The Ceph-CSI Authors.

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

package rbd_types

import (
	"context"

	"github.com/ceph/go-ceph/rados"
)

type Volume interface {
	// Destroy frees the resources used by the Volume.
	Destroy(/* TODO pass context.Context */)

	GetID(ctx context.Context) (string, error)

	AddToGroup(ctx context.Context, ioctx *rados.IOContext, group string) error
	RemoveFromGroup(ctx context.Context, ioctx *rados.IOContext, group string) error
}
