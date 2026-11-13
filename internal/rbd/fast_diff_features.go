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
	"context"
	"errors"
	"fmt"
	"syscall"

	librbd "github.com/ceph/go-ceph/rbd"

	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	// fastDiffFeatures enables efficient DiffIterate operations.
	fastDiffFeatures = librbd.FeatureExclusiveLock | librbd.FeatureObjectMap | librbd.FeatureFastDiff

	fastDiffInvalidFlags = librbd.FlagObjectMapInvalid | librbd.FlagFastDiffInvalid
)

// EnsureFastDiffFeatures repairs the volume and its complete ancestor chain in
// clone-first order. The caller must gate this operation on StorageClass features.
func (rv *rbdVolume) EnsureFastDiffFeatures(ctx context.Context) error {
	var parent *librbd.ParentInfo
	for {
		var err error
		parent, err = rv.reconcileImage(ctx, parent)
		if err != nil {
			return err
		}
		if parent == nil {
			return nil
		}
	}
}

// reconcileImage repairs one image and returns the next ancestor. A nil parent
// selects the volume itself; ancestors are opened by ID, including trashed images.
func (rv *rbdVolume) reconcileImage(ctx context.Context, parent *librbd.ParentInfo) (
	_ *librbd.ParentInfo, err error,
) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	ri := &rv.rbdImage
	var image *librbd.Image
	if parent == nil {
		image, err = ri.open()
	} else {
		ri = &rbdImage{
			Pool:           parent.Image.PoolName,
			RadosNamespace: parent.Image.PoolNamespace,
			ImageID:        parent.Image.ImageID,
			RbdImageName:   parent.Image.ImageName,
			conn:           rv.conn.Copy(),
		}
		defer ri.Destroy(ctx)
		err = ri.openIoctx()
		if err == nil {
			image, err = librbd.OpenImageById(ri.ioctx, ri.ImageID, librbd.NoSnapshot)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open image %s: %w", ri, err)
	}
	defer func() { err = errors.Join(err, image.Close()) }()

	if parent != nil {
		var needsRepair bool
		needsRepair, err = parentNeedsRepair(image, parent.Snap.ID)
		if err != nil {
			return nil, err
		}
		var trashInfo *librbd.TrashInfo
		trashInfo, err = ri.prepareParentForFastDiff(image, parent.Image.Trash, needsRepair)
		if err != nil {
			return nil, err
		}
		defer func() {
			// Reset before HEAD-only metadata operations. Attempt cleanup even
			// if resetting fails, and keep both errors for the caller to retry.
			err = errors.Join(err, image.SetSnapshot(librbd.NoSnapshot), ri.cleanupParent(image, trashInfo))
		}()
	}

	changed, err := enableFeatures(ctx, image)
	if err != nil {
		return nil, err
	}

	if err = rebuildObjectMapIfNeeded(ctx, image, changed); err != nil {
		return nil, fmt.Errorf("failed to repair image HEAD: %w", err)
	}
	if parent != nil {
		if err = reconcileSnapshot(ctx, image, parent.Snap.ID, changed); err != nil {
			return nil, err
		}
		log.DebugLog(ctx, "reconcile snapshot on pv %s", rv.RequestName)
	}
	// An ancestor's referenced snapshot remains selected here: its parent may
	// differ from HEAD's parent, for example after HEAD has been flattened.
	next, err := image.GetParent()
	if errors.Is(err, librbd.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get parent: %w", err)
	}

	return next, nil
}

// reconcileSnapshot leaves the snapshot selected so traversal follows its parent
// relationship. The caller must reset to HEAD before HEAD-only operations.
func reconcileSnapshot(ctx context.Context, image *librbd.Image, snapID uint64, changed bool) error {
	// Names may no longer exist after VolumeSnapshot deletion, but the clone
	// relationship retains the numeric ID, including for trash snapshots.
	err := image.SetSnapByID(snapID)
	if err != nil {
		return fmt.Errorf("failed to select parent snapshot %d: %w", snapID, err)
	}
	err = rebuildObjectMapIfNeeded(ctx, image, changed)
	if err != nil {
		return fmt.Errorf("failed to repair parent snapshot %d: %w", snapID, err)
	}

	return nil
}

// enableFeatures adds exclusive-lock first, then missing object-map and fast-diff
// features. It preserves existing features and reports whether any were added.
func enableFeatures(ctx context.Context, image *librbd.Image) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	features, err := image.GetFeatures()
	if err != nil {
		return false, fmt.Errorf("failed to get features: %w", err)
	}
	missing := fastDiffFeatures &^ features
	// Enable the lock prerequisite before requesting object-map and fast-diff.
	if missing&librbd.FeatureExclusiveLock != 0 {
		err = image.UpdateFeatures(librbd.FeatureExclusiveLock, true)
		if err != nil {
			return false, fmt.Errorf("failed to enable exclusive-lock: %w", err)
		}
	}
	mapFeatures := missing & (librbd.FeatureObjectMap | librbd.FeatureFastDiff)
	if mapFeatures != 0 {
		err = image.UpdateFeatures(mapFeatures, true)
		if err != nil {
			return false, fmt.Errorf("failed to enable object-map and fast-diff: %w", err)
		}
	}
	if missing != 0 {
		log.DebugLog(ctx, "updated features on image %s", image.GetName())
	}

	return missing != 0, nil
}

// rebuildObjectMapIfNeeded checks validity even with complete features so retries
// finish a rebuild interrupted after feature enablement.
func rebuildObjectMapIfNeeded(ctx context.Context, image *librbd.Image, force bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	flags, err := image.GetFlags()
	if err != nil {
		return fmt.Errorf("failed to get image flags: %w", err)
	}
	if !force && flags&fastDiffInvalidFlags == 0 {
		return nil
	}
	err = image.RebuildObjectMapWithProgress(func(_, _ uint64, _ any) int {
		if ctx.Err() != nil {
			return -int(syscall.ECANCELED)
		}

		return 0
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to rebuild object map: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return err
	}

	log.DebugLog(ctx, "rebuild object map for image %s", image.GetName())

	flags, err = image.GetFlags()
	if err != nil {
		return fmt.Errorf("failed to verify rebuilt object map: %w", err)
	}
	if flags&fastDiffInvalidFlags != 0 {
		return errors.New("object map or fast-diff remains invalid after rebuild")
	}

	return nil
}

// parentNeedsRepair only reads image state, so a healthy parent can remain
// in trash. Restore the handle to HEAD before feature changes or rebuilding.
func parentNeedsRepair(image *librbd.Image, snapID uint64) (bool, error) {
	features, err := image.GetFeatures()
	if err != nil {
		return false, err
	}
	if features&fastDiffFeatures != fastDiffFeatures {
		return true, nil
	}
	flags, err := image.GetFlags()
	if err != nil {
		return false, err
	}
	if flags&fastDiffInvalidFlags != 0 {
		return true, nil
	}
	err = image.SetSnapByID(snapID)
	if err != nil {
		return false, err
	}
	flags, err = image.GetFlags()
	err = errors.Join(err, image.SetSnapshot(librbd.NoSnapshot))

	return flags&fastDiffInvalidFlags != 0, err
}
