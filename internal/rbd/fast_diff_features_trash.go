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
package rbd

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"syscall"
	"time"

	"github.com/ceph/go-ceph/rados"
	librbd "github.com/ceph/go-ceph/rbd"

	rbderrors "github.com/ceph/ceph-csi/internal/rbd/errors"
)

const diffIterateTrashKey = ".rbd.csi.ceph.com/diff-iterate-trash"

// prepareParentForFastDiff persists the original trash entry before restoring it.
// Returning the entry lets cleanup finish after repair, or on a later retry.
func (ri *rbdImage) prepareParentForFastDiff(
	image *librbd.Image, inTrash, needsRepair bool,
) (*librbd.TrashInfo, error) {
	var info *librbd.TrashInfo
	value, err := image.GetMetadata(diffIterateTrashKey)
	if err != nil && !errors.Is(err, librbd.ErrNotFound) {
		return nil, fmt.Errorf("failed to read parent trash intent: %w", err)
	}
	if err == nil {
		err = json.Unmarshal([]byte(value), &info)
		if err != nil {
			return nil, fmt.Errorf("failed to decode parent trash intent: %w", err)
		}
		if info == nil || info.Name == "" || info.Id != ri.ImageID {
			return nil, errors.New("parent trash intent does not match the image")
		}
	}
	if inTrash && !needsRepair && info == nil {
		return nil, nil
	}
	recovering := info != nil
	if inTrash && needsRepair {
		info, err = ri.trashInfo()
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(info)
		if err != nil {
			return nil, fmt.Errorf("failed to encode parent trash intent: %w", err)
		}
		err = image.SetMetadata(diffIterateTrashKey, string(value))
		if err != nil {
			return nil, fmt.Errorf("failed to persist parent trash intent: %w", err)
		}
		err = ri.restoreParent(info, recovering)
		if err != nil {
			return nil, fmt.Errorf("failed to restore parent from trash: %w", err)
		}
	}
	if info == nil {
		return nil, nil
	}

	return info, nil
}

func (ri *rbdImage) cleanupParent(image *librbd.Image, info *librbd.TrashInfo) error {
	if info == nil {
		return nil
	}
	err := trashParent(ri.ioctx, ri.ImageID, info.Name, diffTrashDelay(info.DefermentEndTime))
	if err != nil {
		return fmt.Errorf("failed to return parent to trash: %w", err)
	}
	err = image.RemoveMetadata(diffIterateTrashKey)
	if err != nil {
		return fmt.Errorf("failed to clear parent trash intent: %w", err)
	}

	return nil
}

func (ri *rbdImage) trashInfo() (*librbd.TrashInfo, error) {
	entries, err := librbd.GetTrashList(ri.ioctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list parent trash: %w", err)
	}
	for _, entry := range entries {
		if entry.Id == ri.ImageID {
			return &entry, nil
		}
	}

	return nil, fmt.Errorf("parent %s not found in trash: %w", ri.ImageID, librbd.ErrNotFound)
}

// restoreParent first lets librbd resume an interrupted restore. A marked
// MOVING entry instead needs its pending trash move completed before restore.
func (ri *rbdImage) restoreParent(info *librbd.TrashInfo, recovering bool) error {
	err := librbd.TrashRestore(ri.ioctx, ri.ImageID, info.Name)
	var errCode rbderrors.ErrorCode
	if !recovering || !errors.As(err, &errCode) || errCode.ErrorCode() != -int(syscall.EBUSY) {
		return err
	}
	if err := trashParent(ri.ioctx, ri.ImageID, info.Name, diffTrashDelay(info.DefermentEndTime)); err != nil {
		return fmt.Errorf("restore is busy and completing the pending trash move failed: %w", err)
	}

	return librbd.TrashRestore(ri.ioctx, ri.ImageID, info.Name)
}

func diffTrashDelay(expiry time.Time) time.Duration {
	delay := max(time.Until(expiry), 0)
	// librbd accepts whole seconds. Round up to preserve the deferment.
	rounded := delay.Truncate(time.Second)
	if rounded < delay {
		rounded += time.Second
	}

	return rounded
}

// trashParent retries interrupted moves, without moving a replacement image
// that happens to reuse the original name. Trash-entry presence alone is not
// sufficient: librbd creates that entry before removing the active directory.
func trashParent(ioctx *rados.IOContext, imageID, name string, delay time.Duration) error {
	names, err := librbd.GetImageNames(ioctx)
	if err != nil {
		return err
	}
	active := slices.Contains(names, name)
	if active {
		image, err := librbd.OpenImageReadOnly(ioctx, name, librbd.NoSnapshot)
		if err != nil {
			return fmt.Errorf("cannot verify active parent %s before completing trash move: %w", name, err)
		}
		id, err := image.GetId()
		err = errors.Join(err, image.Close())
		if err != nil {
			return err
		}
		if id != imageID {
			return fmt.Errorf("parent name %s now identifies a different image", name)
		}
	} else {
		err = verifyTrashEntry(ioctx, imageID, name)
		if err != nil {
			return err
		}
	}
	// OpenImageById handles have no name. A named handle lets librbd finish a
	// pending move even after the directory entry has been removed.
	err = librbd.GetImage(ioctx, name).Trash(delay)
	if !active && errors.Is(err, librbd.ErrNotFound) {
		// A completed move has no directory entry and a normal trash entry, which
		// we verified above. Only this case makes ENOENT an idempotent success.
		return nil
	}

	return err
}

func verifyTrashEntry(ioctx *rados.IOContext, imageID, name string) error {
	entries, err := librbd.GetTrashList(ioctx)
	if err != nil {
		return err
	}
	found := false
	for _, entry := range entries {
		if entry.Name != name {
			continue
		}
		if entry.Id != imageID {
			return fmt.Errorf("ambiguous trash entries for parent name %s", name)
		}
		found = true
	}
	if !found {
		return fmt.Errorf("parent %s is absent from both directory and trash", imageID)
	}

	return nil
}
