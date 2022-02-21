/*
Copyright 2022 The Ceph-CSI Authors.

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

package reftracker

import (
	goerrors "errors"
	"fmt"

	"github.com/ceph/ceph-csi/internal/util/reftracker/errors"
	"github.com/ceph/ceph-csi/internal/util/reftracker/radoswrapper"
	"github.com/ceph/ceph-csi/internal/util/reftracker/reftype"
	v1 "github.com/ceph/ceph-csi/internal/util/reftracker/v1"
	"github.com/ceph/ceph-csi/internal/util/reftracker/version"

	"github.com/ceph/go-ceph/rados"
)

// reftracker is key-based implementation of a reference counter.
//
// Unlike integer-based counter, reftracker counts references by tracking
// unique keys. This allows accounting in situations where idempotency must be
// preserved. It guarantees there will be no duplicit increments or decrements
// of the counter.
//
// It is stored persistently as a RADOS object, and is safe to use with
// multiple concurrent writers, and across different nodes of a cluster.
//
// Example:
//
//      created, err := Add(
//      	ioctx,
//      	"my-reftracker",
//      	map[string]struct{}{
//      		"ref-key-1": {},
//      		"ref-key-2": {},
//      	},
//      )
//
//  Since this is a new reftracker object, `created` is `true`.
//
//      "my-reftracker" now holds:
//          ["ref-key-1":reftype.Normal, "ref-key-2":reftype.Normal]
//      The reference count is 2.
//
//      created, err := Add(
//      	ioctx,
//      	"my-reftracker",
//      	map[string]struct{}{
//      		"ref-key-1": {},
//      		"ref-key-2": {},
//      		"ref-key-3": {},
//      	},
//      )
//
//  Reftracker named "my-reftracker" already exists, so `created` is now
//  `false`. Since "ref-key-1" and "ref-key-2" keys are already tracked,
//  only "ref-key-3" is added.
//
//      "my-reftracker" now holds:
//          ["ref-key-1":reftype.Normal, "ref-key-2":reftype.Normal,
//           "ref-key-3":reftype.Normal]
//      The reference count is 3.
//
//      deleted, err := Remove(
//      	ioctx,
//      	"my-reftracker",
//      	map[string]reftype.RefType{
//      		"ref-key-1": reftype.Normal,
//      		"ref-key-2": reftype.Mask,
//      	},
//      )
//
//      "my-reftracker" now holds:
//          ["ref-key-2":reftype.Mask, "ref-key-3":reftype.Normal]
//      The reference count is 1.
//
//  Since the reference count is greater than zero, `deleted` is `false`.
//  "ref-key-1" was removed, and so is not listed among tracked references.
//  "ref-key-2" was only masked, so it's been kept. However, masked references
//  don't contribute to overall reference count, so the resulting refcount
//  after this Remove() call is 1.
//
//      created, err := Add(
//      	ioctx,
//      	"my-reftracker",
//      	map[string]struct{}{
//      		"ref-key-2": {},
//      	},
//      )
//
//      "my-reftracker" now holds:
//          ["ref-key-2":reftype.Mask, "ref-key-3":reftype.Normal]
//      The reference count is 1.
//
//  "ref-key-2" is already tracked, so it will not be added again. Since it
//  remains masked, it won't contribute to the reference count.
//
//      deleted, err := Remove(
//      	ioctx,
//      	"my-reftracker",
//      	map[string]reftype.RefType{
//      		"ref-key-3": reftype.Normal,
//      	},
//      )
//
//  "ref-key-3" was the only tracked key that contributed to reference count.
//  After this Remove() call it's now removed. As a result, the reference count
//  dropped down to zero, and the whole object has been deleted too.
//  `deleted` is `true`.

// Add atomically adds references to `rtName` reference tracker.
// If the reftracker object doesn't exist yet, it is created and `true` is
// returned. If some keys in `refs` map are already tracked by this reftracker
// object, they will not be added again.
func Add(
	ioctx radoswrapper.IOContextW,
	rtName string,
	refs map[string]struct{},
) (bool, error) {
	if err := validateAddInput(rtName, refs); err != nil {
		return false, err
	}

	// Read reftracker version.

	rtVer, err := version.Read(ioctx, rtName)
	if err != nil {
		if goerrors.Is(err, rados.ErrNotFound) {
			// This is a new reftracker. Initialize it with `refs`.
			if err = v1.Init(ioctx, rtName, refs); err != nil {
				return false, fmt.Errorf("failed to initialize reftracker: %w", err)
			}

			return true, nil
		}

		return false, fmt.Errorf("failed to read reftracker version: %w", err)
	}

	// Add references to reftracker object.

	gen, err := ioctx.GetLastVersion()
	if err != nil {
		return false, fmt.Errorf("failed to get RADOS object version: %w", err)
	}

	switch rtVer {
	case v1.Version:
		err = v1.Add(ioctx, rtName, gen, refs)
		if err != nil {
			err = fmt.Errorf("failed to add refs: %w", err)
		}
	default:
		err = errors.UnknownObjectVersion(rtVer)
	}

	return false, err
}

// Remove atomically removes references from `rtName` reference tracker.
// If the reftracker object holds no references after this removal, the whole
// object is deleted too, and `true` is returned. If the reftracker object
// doesn't exist, (true, nil) is returned.
func Remove(
	ioctx radoswrapper.IOContextW,
	rtName string,
	refs map[string]reftype.RefType,
) (bool, error) {
	if err := validateRemoveInput(rtName, refs); err != nil {
		return false, err
	}

	// Read reftracker version.

	rtVer, err := version.Read(ioctx, rtName)
	if err != nil {
		if goerrors.Is(err, rados.ErrNotFound) {
			// This reftracker doesn't exist. Assume it was already deleted.
			return true, nil
		}

		return false, fmt.Errorf("failed to read reftracker version: %w", err)
	}

	// Remove references from reftracker.

	gen, err := ioctx.GetLastVersion()
	if err != nil {
		return false, fmt.Errorf("failed to get RADOS object version: %w", err)
	}

	var deleted bool

	switch rtVer {
	case v1.Version:
		deleted, err = v1.Remove(ioctx, rtName, gen, refs)
		if err != nil {
			err = fmt.Errorf("failed to remove refs: %w", err)
		}
	default:
		err = errors.UnknownObjectVersion(rtVer)
	}

	return deleted, err
}

var (
	errNoRTName = goerrors.New("missing reftracker name")
	errNoRefs   = goerrors.New("missing refs")
)

func validateAddInput(rtName string, refs map[string]struct{}) error {
	if rtName == "" {
		return errNoRTName
	}

	if len(refs) == 0 {
		return errNoRefs
	}

	return nil
}

func validateRemoveInput(rtName string, refs map[string]reftype.RefType) error {
	if rtName == "" {
		return errNoRTName
	}

	if len(refs) == 0 {
		return errNoRefs
	}

	return nil
}
