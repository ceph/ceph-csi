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

package radosmutex

import (
	"context"
	goerrors "errors"
	"fmt"

	"github.com/ceph/ceph-csi/internal/util/log"
	"github.com/ceph/ceph-csi/internal/util/radosmutex/errors"
	"github.com/ceph/ceph-csi/internal/util/radosmutex/retryoptions"
	v1 "github.com/ceph/ceph-csi/internal/util/radosmutex/v1"
	"github.com/ceph/ceph-csi/internal/util/reftracker/radoswrapper"
	"github.com/ceph/ceph-csi/internal/util/reftracker/reftype"
	"github.com/ceph/ceph-csi/internal/util/reftracker/version"

	"github.com/ceph/go-ceph/rados"
)

func CreateOrAquireLock(
	ctx context.Context,
	ioctx radoswrapper.IOContextW,
	lockName string,
	lockRequestor string,
	retryOptions retryoptions.RetryOptions,
) (bool, error) {
	if err := validateLockInput(lockName, lockRequestor); err != nil {
		log.DebugLog(ctx, "Failed Input validation in in creating lock")
		return false, err
	}

	lockVer, err := version.Read(ioctx, lockName)

	if err != nil {
		if goerrors.Is(err, rados.ErrNotFound) {
			if err = v1.Init(ctx, ioctx, lockName, lockRequestor); err != nil {
				log.DebugLog(ctx, "failed to initialize lock: %w", err)
				return false, fmt.Errorf("failed to initialize lock: %w", err)
			}

			return true, nil
		}

		return false, fmt.Errorf("failed to read lock version: %w", err)
	}

	switch lockVer {
	case v1.Version:
		_, err = v1.TryToAquireLock(ctx, ioctx, lockName, lockRequestor, retryOptions)
		if err != nil {
			err = fmt.Errorf("failed to add lock: %w", err)
		}
	default:
		err = errors.UnknownObjectVersion(lockVer)
	}

	return true, err
}

func ReleaseLock(
	ctx context.Context,
	ioctx radoswrapper.IOContextW,
	lockName string,
	lockRequestor string,
) (bool, error) {
	if err := validateLockInput(lockName, lockRequestor); err != nil {
		return false, err
	}

	// Read lock version.

	rtVer, err := version.Read(ioctx, lockName)
	if err != nil {
		if goerrors.Is(err, rados.ErrNotFound) {
			// This lock doesn't exist. Assume it was already deleted.
			return true, nil
		}

		return false, fmt.Errorf("failed to read reftracker version: %w", err)
	}

	gen, err := ioctx.GetLastVersion()
	if err != nil {
		return false, fmt.Errorf("failed to get RADOS object version: %w", err)
	}

	switch rtVer {
	case v1.Version:
		err = v1.ReleaseLock(ctx, ioctx, lockName, lockRequestor, gen)
		if err != nil {
			err = fmt.Errorf("failed to remove refs: %w", err)
		}
	default:
		err = errors.UnknownObjectVersion(rtVer)
	}

	return true, err
}

func Remove(
	ctx context.Context,
	ioctx radoswrapper.IOContextW,
	lockName string,
	refs map[string]reftype.RefType,
) (bool, error) {
	if err := validateRemoveInput(lockName); err != nil {
		return false, err
	}

	// Read lock version.

	rtVer, err := version.Read(ioctx, lockName)
	if err != nil {
		if goerrors.Is(err, rados.ErrNotFound) {
			// This lock doesn't exist. Assume it was already deleted.
			return true, nil
		}

		return false, fmt.Errorf("failed to read reftracker version: %w", err)
	}

	gen, err := ioctx.GetLastVersion()
	if err != nil {
		return false, fmt.Errorf("failed to get RADOS object version: %w", err)
	}

	switch rtVer {
	case v1.Version:
		err = v1.DeleteLock(ctx, ioctx, lockName, gen)
		if err != nil {
			err = fmt.Errorf("failed to remove refs: %w", err)
		}
	default:
		err = errors.UnknownObjectVersion(rtVer)
	}

	return true, err
}

var (
	errLockName = goerrors.New("missing lock name")
	errNoOwner  = goerrors.New("missing lock requestor")
)

func validateLockInput(lockName string, lockRequestor string) error {
	if lockName == "" {
		return errLockName
	}

	if lockRequestor == "" {
		return errNoOwner
	}

	return nil
}

func validateRemoveInput(rtName string) error {
	if rtName == "" {
		return errLockName
	}

	return nil
}
