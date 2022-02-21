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

package v1

import (
	goerrors "errors"
	"fmt"

	"github.com/ceph/ceph-csi/internal/util/reftracker/errors"
	"github.com/ceph/ceph-csi/internal/util/reftracker/radoswrapper"
	"github.com/ceph/ceph-csi/internal/util/reftracker/reftype"
	"github.com/ceph/ceph-csi/internal/util/reftracker/version"

	"github.com/ceph/go-ceph/rados"
)

/*

Version 1 layout:
-----------------

If not specified otherwise, all values are stored in big-endian order.

    byte idx      type         name
    --------     ------       ------
     0 ..  3     uint32       refcount

    `refcount`: Number of references held by the reftracker object. The actual
                reference keys are stored in an OMap of the RADOS object.

    OMap entry layout:

        Key:

            reftracker key.

        Value:

            byte idx      type         name
            --------     ------       ------
             0 ..  3     uint32        type

            `type`: reference type defined in reftracker/reftype.

*/

type readResult struct {
	// Total number of references held by the reftracker object.
	total refCount
	// Refs whose keys matched the request.
	foundRefs map[string]reftype.RefType
}

// Atomically initializes a new reftracker object.
func Init(
	ioctx radoswrapper.IOContextW,
	rtName string,
	refs map[string]struct{},
) error {
	// Prepare refcount and OMap key-value pairs.

	refsToAddBytes := make(map[string][]byte, len(refs))

	for ref := range refs {
		refsToAddBytes[ref] = reftype.ToBytes(reftype.Normal)
	}

	// Perform the write.

	w := ioctx.CreateWriteOp()
	defer w.Release()

	w.Create(rados.CreateExclusive)
	w.SetXattr(version.XattrName, version.ToBytes(Version))
	w.SetOmap(refsToAddBytes)
	w.WriteFull(refCount(len(refsToAddBytes)).toBytes())

	return errors.FailedObjectWrite(w.Operate(rtName))
}

// Atomically adds refs to an existing reftracker object.
func Add(
	ioctx radoswrapper.IOContextW,
	rtName string,
	gen uint64,
	refs map[string]struct{},
) error {
	// Read the reftracker object to figure out which refs to add.

	readRes, err := readObjectByKeys(ioctx, rtName, gen, refsMapToKeysSlice(refs))
	if err != nil {
		return errors.FailedObjectRead(err)
	}

	// Build list of refs to add.
	// Add only refs that are missing in the reftracker object.

	refsToAdd := make(map[string][]byte)

	for ref := range refs {
		if _, found := readRes.foundRefs[ref]; !found {
			refsToAdd[ref] = reftype.ToBytes(reftype.Normal)
		}
	}

	if len(refsToAdd) == 0 {
		// Nothing to do.
		return nil
	}

	// Calculate new refcount.

	rcToAdd := refCount(len(refsToAdd))
	newRC := readRes.total + rcToAdd

	if newRC < readRes.total {
		return goerrors.New("addition would overflow uint32 refcount")
	}

	// Write the data.

	w := ioctx.CreateWriteOp()
	defer w.Release()

	w.AssertVersion(gen)
	w.WriteFull(newRC.toBytes())
	w.SetOmap(refsToAdd)

	return errors.FailedObjectWrite(w.Operate(rtName))
}

// Atomically removes refs from reftracker object. If the object wouldn't hold
// any references after the removal, the whole object is deleted instead.
func Remove(
	ioctx radoswrapper.IOContextW,
	rtName string,
	gen uint64,
	refs map[string]reftype.RefType,
) (bool, error) {
	// Read the reftracker object to figure out which refs to remove.

	readRes, err := readObjectByKeys(ioctx, rtName, gen, typedRefsMapToKeysSlice(refs))
	if err != nil {
		return false, errors.FailedObjectRead(err)
	}

	// Build lists of refs to remove, replace, and add.
	// There are three cases that need to be handled:
	// (1) removing reftype.Normal refs,
	// (2) converting refs that were reftype.Normal into reftype.Mask,
	// (3) adding a new reftype.Mask key.

	var (
		refsToRemove []string
		refsToSet    = make(map[string][]byte)
		rcToSubtract refCount
	)

	for ref, refType := range refs {
		if matchedRefType, found := readRes.foundRefs[ref]; found {
			if refType == reftype.Normal {
				// Case (1): regular removal of Normal ref.
				refsToRemove = append(refsToRemove, ref)
				if matchedRefType == reftype.Normal {
					// If matchedRef was reftype.Mask, it would have already been
					// subtracted from the refcount.
					rcToSubtract++
				}
			} else if refType == reftype.Mask && matchedRefType == reftype.Normal {
				// Case (2): convert Normal ref to Mask.
				// Since this ref is now reftype.Mask, rcToSubtract needs to be adjusted
				// too -- so that this ref is not counted in.
				refsToSet[ref] = reftype.ToBytes(reftype.Mask)
				rcToSubtract++
			}
		} else {
			if refType == reftype.Mask {
				// Case (3): add a new Mask ref.
				// reftype.Mask doesn't contribute refcount so no change to rcToSubtract.
				refsToSet[ref] = reftype.ToBytes(reftype.Mask)
			} // else: No such ref was found, so there's nothing to remove.
		}
	}

	if len(refsToRemove) == 0 && len(refsToSet) == 0 {
		// Nothing to do.
		return false, nil
	}

	// Calculate new refcount.

	if rcToSubtract > readRes.total {
		// BUG: this should never happen!
		return false, fmt.Errorf("refcount underflow, reftracker object corrupted")
	}

	newRC := readRes.total - rcToSubtract
	// If newRC is zero, it means all refs that the reftracker object held will be
	// now gone, and the object must be deleted.
	deleted := newRC == 0

	// Write the data.

	w := ioctx.CreateWriteOp()
	defer w.Release()

	w.AssertVersion(gen)

	if deleted {
		w.Remove()
	} else {
		w.WriteFull(newRC.toBytes())
		w.RmOmapKeys(refsToRemove)
		w.SetOmap(refsToSet)
	}

	if err := w.Operate(rtName); err != nil {
		return false, errors.FailedObjectWrite(err)
	}

	return deleted, nil
}

// Tries to find `keys` in reftracker object and returns the result. Failing to
// find any particular key does not result in an error.
func readObjectByKeys(
	ioctx radoswrapper.IOContextW,
	rtName string,
	gen uint64,
	keys []string,
) (*readResult, error) {
	// Read data from object.

	rcBytes := make([]byte, refCountSize)

	r := ioctx.CreateReadOp()
	defer r.Release()

	r.AssertVersion(gen)
	r.Read(0, rcBytes)
	s := r.GetOmapValuesByKeys(keys)

	if err := r.Operate(rtName); err != nil {
		return nil, errors.TryRADOSAborted(err)
	}

	// Convert it from byte slices to type-safe values.

	var (
		rc   refCount
		refs = make(map[string]reftype.RefType)
		err  error
	)

	rc, err = refCountFromBytes(rcBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse refcount: %w", err)
	}

	for {
		kvPair, err := s.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to iterate over OMap: %w", err)
		}

		if kvPair == nil {
			break
		}

		refType, err := reftype.FromBytes(kvPair.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to parse reftype: %w", err)
		}

		refs[kvPair.Key] = refType
	}

	return &readResult{
		total:     rc,
		foundRefs: refs,
	}, nil
}

func refsMapToKeysSlice(m map[string]struct{}) []string {
	s := make([]string, 0, len(m))
	for k := range m {
		s = append(s, k)
	}

	return s
}

func typedRefsMapToKeysSlice(m map[string]reftype.RefType) []string {
	s := make([]string, 0, len(m))
	for k := range m {
		s = append(s, k)
	}

	return s
}
