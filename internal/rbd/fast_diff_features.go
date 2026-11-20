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

// EnsureFastDiffFeatures repairs the immediate parent of the volume and the
// snapshot it was cloned from. The parent is the intermediate image ceph-csi
// creates for a clone or restore: the temporary clone image for a PVC-PVC
// clone, or the snapshot backing image for a VolumeSnapshot. Such chains
// created before imageFeatures were inherited by those images can lack the
// features for efficient DiffIterate even when the volume itself has them.
//
// The volume is only opened read-only to find its parent; its own features and
// object map are left untouched, as the volume may be in use by a client.
func (rv *rbdVolume) EnsureFastDiffFeatures(ctx context.Context) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	image, err := rv.openReadOnly()
	if err != nil {
		return fmt.Errorf("failed to open image %s: %w", &rv.rbdImage, err)
	}
	parent, err := image.GetParent()
	if errors.Is(err, librbd.ErrNotFound) {
		parent, err = nil, nil
	} else if err != nil {
		err = fmt.Errorf("failed to get parent: %w", err)
	}
	err = errors.Join(err, image.Close())
	if err != nil || parent == nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}

	ri := &rbdImage{
		Pool:           parent.Image.PoolName,
		RadosNamespace: parent.Image.PoolNamespace,
		ImageID:        parent.Image.ImageID,
		RbdImageName:   parent.Image.ImageName,
		conn:           rv.conn.Copy(),
	}
	defer ri.Destroy(ctx)
	if err = ri.openIoctx(); err != nil {
		return fmt.Errorf("failed to open image %s: %w", ri, err)
	}
	parentImage, err := librbd.OpenImageById(ri.ioctx, ri.ImageID, librbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("failed to open image %s: %w", ri, err)
	}
	defer func() { err = errors.Join(err, parentImage.Close()) }()

	needsRepair, err := parentNeedsRepair(parentImage, parent.Snap.ID)
	if err != nil {
		return err
	}
	trashInfo, err := ri.prepareParentForFastDiff(parentImage, parent.Image.Trash, needsRepair)
	if err != nil {
		return err
	}
	defer func() {
		// Attempt cleanup even if resetting fails, and preserve both errors for the caller to retry.
		err = errors.Join(err, parentImage.SetSnapshot(librbd.NoSnapshot), ri.cleanupParent(parentImage, trashInfo))
	}()

	changed, err := reconcileImage(ctx, parentImage)
	if err != nil {
		return err
	}
	if err = reconcileSnapshot(ctx, parentImage, parent.Snap.ID, changed); err != nil {
		return err
	}
	log.DebugLog(ctx, "reconcile snapshot on pv %s", rv.RequestName)

	return nil
}

// reconcileImage enables missing features and rebuild object-map if needed.
func reconcileImage(ctx context.Context, image *librbd.Image) (bool, error) {
	changed, err := enableFeatures(ctx, image)
	if err != nil {
		return false, err
	}
	if err = rebuildObjectMapIfNeeded(ctx, image, changed); err != nil {
		return changed, fmt.Errorf("failed to repair image HEAD: %w", err)
	}

	return changed, nil
}

// reconcileSnapshot repairs the snapshot referenced by the volume.
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
