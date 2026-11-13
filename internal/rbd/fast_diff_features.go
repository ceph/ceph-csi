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
	// fastDiffFeatures is the feature set required for efficient DiffIterate.
	fastDiffFeatures = librbd.FeatureExclusiveLock | librbd.FeatureObjectMap | librbd.FeatureFastDiff

	fastDiffInvalidFlags = librbd.FlagObjectMapInvalid | librbd.FlagFastDiffInvalid
)

// EnsureFastDiffFeatures repairs the immediate parent of the volume and the
// snapshot it was cloned from. The parent is the intermediate image ceph-csi
// creates for a clone or restore: the temporary clone image for a PVC-PVC
// clone, or the snapshot backing image for a VolumeSnapshot.
//
// Only the immediate parent is repaired; the chain is not walked further up.
// The parent is repaired in place, including while it is in the trash:
// features live in the image header and object maps in objects keyed by the
// image ID, neither of which a trash move changes, so restoring the parent is
// not required.
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
		ClusterID:      rv.ClusterID,
		conn:           rv.conn.Copy(),
	}
	defer ri.Destroy(ctx)
	if err = ri.openIoctx(); err != nil {
		return fmt.Errorf("failed to open pool of image %s: %w", ri, err)
	}
	// The parent may be in the trash, where it cannot be opened by name; the
	// image ID stays valid whether or not the parent is currently in the trash.
	parentImage, err := librbd.OpenImageById(ri.ioctx, ri.ImageID, librbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("failed to open image %s: %w", ri, err)
	}
	defer func() { err = errors.Join(err, parentImage.Close()) }()

	// Images opened by ID carry no name, so pass one for log and error messages.
	name := ri.String()
	changed, err := reconcileImage(ctx, parentImage, name)
	if err != nil {
		return err
	}
	if err = reconcileSnapshot(ctx, parentImage, name, parent.Snap.ID, changed); err != nil {
		return err
	}
	log.DebugLog(ctx, "reconciled fast-diff features of parent %s for pv %s", ri, rv.RequestName)

	return nil
}

// reconcileImage enables missing features and rebuilds the object map if needed.
func reconcileImage(ctx context.Context, image *librbd.Image, name string) (bool, error) {
	changed, err := enableFeatures(ctx, image, name)
	if err != nil {
		return false, err
	}
	if err = rebuildObjectMapIfNeeded(ctx, image, name, changed); err != nil {
		return changed, fmt.Errorf("failed to repair HEAD of image %s: %w", name, err)
	}

	return changed, nil
}

// reconcileSnapshot repairs the snapshot the volume was cloned from. The name
// may be gone after a VolumeSnapshot deletion, so the snapshot is selected by
// the numeric ID kept in the clone relationship. Flags and object maps are
// per-snapshot, which is why the handle must select it before checking or
// rebuilding.
func reconcileSnapshot(ctx context.Context, image *librbd.Image, name string, snapID uint64, changed bool) error {
	snapName := fmt.Sprintf("%s@%d", name, snapID)
	err := image.SetSnapByID(snapID)
	if err != nil {
		return fmt.Errorf("failed to select snapshot %s: %w", snapName, err)
	}
	err = rebuildObjectMapIfNeeded(ctx, image, snapName, changed)
	if err != nil {
		return fmt.Errorf("failed to repair snapshot %s: %w", snapName, err)
	}

	return nil
}

// enableFeatures adds exclusive-lock first, then missing object-map and fast-diff
// features. It preserves existing features and reports whether any were added.
func enableFeatures(ctx context.Context, image *librbd.Image, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	features, err := image.GetFeatures()
	if err != nil {
		return false, fmt.Errorf("failed to get features of image %s: %w", name, err)
	}
	missing := fastDiffFeatures &^ features
	// Enable the lock prerequisite before requesting object-map and fast-diff.
	if missing&librbd.FeatureExclusiveLock != 0 {
		err = image.UpdateFeatures(librbd.FeatureExclusiveLock, true)
		if err != nil {
			return false, fmt.Errorf("failed to enable exclusive-lock on image %s: %w", name, err)
		}
	}
	mapFeatures := missing & (librbd.FeatureObjectMap | librbd.FeatureFastDiff)
	if mapFeatures != 0 {
		err = image.UpdateFeatures(mapFeatures, true)
		if err != nil {
			return false, fmt.Errorf("failed to enable object-map and fast-diff on image %s: %w", name, err)
		}
	}
	if missing != 0 {
		log.DebugLog(ctx, "enabled missing fast-diff features on image %s", name)
	}

	return missing != 0, nil
}

// rebuildObjectMapIfNeeded checks validity even with complete features so retries
// finish a rebuild interrupted after feature enablement.
func rebuildObjectMapIfNeeded(ctx context.Context, image *librbd.Image, name string, force bool) error {
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

	log.DebugLog(ctx, "rebuilt object map of %s", name)

	flags, err = image.GetFlags()
	if err != nil {
		return fmt.Errorf("failed to verify rebuilt object map: %w", err)
	}
	if flags&fastDiffInvalidFlags != 0 {
		return fmt.Errorf("object map or fast-diff of %s remains invalid after rebuild", name)
	}

	return nil
}
