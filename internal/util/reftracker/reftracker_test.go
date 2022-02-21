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
	"testing"

	"github.com/ceph/ceph-csi/internal/util/reftracker/radoswrapper"
	"github.com/ceph/ceph-csi/internal/util/reftracker/reftype"

	"github.com/stretchr/testify/assert"
)

const rtName = "hello-rt"

func TestRTAdd(t *testing.T) {
	t.Parallel()

	// Verify input validation for reftracker name.
	t.Run("AddNoName", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
		created, err := Add(ioctx, "", nil)
		assert.Error(ts, err)
		assert.False(ts, created)
	})

	// Verify input validation for nil and empty refs.
	t.Run("AddNoRefs", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
		refs := []map[string]struct{}{
			nil,
			make(map[string]struct{}),
		}
		for _, ref := range refs {
			created, err := Add(ioctx, rtName, ref)
			assert.Error(ts, err)
			assert.False(ts, created)
		}
	})

	// Add multiple refs in a single Add().
	t.Run("AddBulk", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
		created, err := Add(ioctx, rtName, map[string]struct{}{
			"ref1": {},
			"ref2": {},
			"ref3": {},
		})
		assert.NoError(ts, err)
		assert.True(ts, created)
	})

	// Add refs where each Add() has some of the refs overlapping
	// with the previous call.
	t.Run("AddOverlapping", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
		created, err := Add(ioctx, rtName, map[string]struct{}{
			"ref1": {},
			"ref2": {},
		})
		assert.NoError(ts, err)
		assert.True(ts, created)

		refsTable := []map[string]struct{}{
			{"ref2": {}, "ref3": {}},
			{"ref3": {}, "ref4": {}},
			{"ref4": {}, "ref5": {}},
		}
		for _, refs := range refsTable {
			created, err = Add(ioctx, rtName, refs)
			assert.NoError(ts, err)
			assert.False(ts, created)
		}
	})
}

func TestRTRemove(t *testing.T) {
	t.Parallel()

	// Verify input validation for nil and empty refs.
	t.Run("RemoveNoRefs", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
		refs := []map[string]reftype.RefType{
			nil,
			make(map[string]reftype.RefType),
		}
		for _, ref := range refs {
			created, err := Remove(ioctx, rtName, ref)
			assert.Error(ts, err)
			assert.False(ts, created)
		}
	})

	// Attempt to remove refs in a non-existent reftracker object should result
	// in success, with deleted=true,err=nil.
	t.Run("RemoveNotExists", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
		deleted, err := Remove(ioctx, "xxx", map[string]reftype.RefType{
			"ref1": reftype.Normal,
		})
		assert.NoError(ts, err)
		assert.True(ts, deleted)
	})

	// Removing only non-existent refs should not result in reftracker object
	// deletion.
	t.Run("RemoveNonExistentRefs", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())

		created, err := Add(ioctx, rtName, map[string]struct{}{
			"ref1": {},
			"ref2": {},
			"ref3": {},
		})
		assert.NoError(ts, err)
		assert.True(ts, created)

		deleted, err := Remove(ioctx, rtName, map[string]reftype.RefType{
			"refX": reftype.Normal,
			"refY": reftype.Normal,
			"refZ": reftype.Normal,
		})
		assert.NoError(ts, err)
		assert.False(ts, deleted)
	})

	// Removing all refs plus some surplus should result in reftracker object
	// deletion.
	t.Run("RemoveNonExistentRefs", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())

		created, err := Add(ioctx, rtName, map[string]struct{}{
			"ref": {},
		})
		assert.NoError(ts, err)
		assert.True(ts, created)

		deleted, err := Remove(ioctx, rtName, map[string]reftype.RefType{
			"refX": reftype.Normal,
			"refY": reftype.Normal,
			"ref":  reftype.Normal,
			"refZ": reftype.Normal,
		})
		assert.NoError(ts, err)
		assert.True(ts, deleted)
	})

	// Bulk removal of all refs should result in reftracker object deletion.
	t.Run("RemoveBulk", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
		keys := []string{"ref1", "ref2", "ref3"}
		refsToAdd := make(map[string]struct{})
		refsToRemove := make(map[string]reftype.RefType)
		for _, k := range keys {
			refsToAdd[k] = struct{}{}
			refsToRemove[k] = reftype.Normal
		}

		created, err := Add(ioctx, rtName, refsToAdd)
		assert.NoError(ts, err)
		assert.True(ts, created)

		deleted, err := Remove(ioctx, rtName, refsToRemove)
		assert.NoError(ts, err)
		assert.True(ts, deleted)
	})

	// Removal of all refs one-by-one should result in reftracker object deletion
	// in the last Remove() call.
	t.Run("RemoveSingle", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())

		created, err := Add(ioctx, rtName, map[string]struct{}{
			"ref1": {},
			"ref2": {},
			"ref3": {},
		})
		assert.NoError(ts, err)
		assert.True(ts, created)

		for _, k := range []string{"ref3", "ref2"} {
			deleted, errRemove := Remove(ioctx, rtName, map[string]reftype.RefType{
				k: reftype.Normal,
			})
			assert.NoError(ts, errRemove)
			assert.False(ts, deleted)
		}

		// Remove the last reference. It should remove the whole reftracker object too.
		deleted, err := Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref1": reftype.Normal,
		})
		assert.NoError(ts, err)
		assert.True(ts, deleted)
	})

	// Cycle through reftracker object twice.
	t.Run("AddRemoveAddRemove", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
		refsToAdd := map[string]struct{}{
			"ref1": {},
			"ref2": {},
			"ref3": {},
		}
		refsToRemove := map[string]reftype.RefType{
			"ref1": reftype.Normal,
			"ref2": reftype.Normal,
			"ref3": reftype.Normal,
		}

		for i := 0; i < 2; i++ {
			created, err := Add(ioctx, rtName, refsToAdd)
			assert.NoError(ts, err)
			assert.True(ts, created)

			deleted, err := Remove(ioctx, rtName, refsToRemove)
			assert.NoError(ts, err)
			assert.True(ts, deleted)
		}
	})

	// Check for respecting idempotency by making multiple additions with overlapping keys
	// and removing only ref keys that were distinct.
	t.Run("AddOverlappingRemoveBulk", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
		created, err := Add(ioctx, rtName, map[string]struct{}{
			"ref1": {},
			"ref2": {},
		})
		assert.True(ts, created)
		assert.NoError(ts, err)
		refsTable := []map[string]struct{}{
			{"ref2": {}, "ref3": {}},
			{"ref3": {}, "ref4": {}},
			{"ref4": {}, "ref5": {}},
		}
		for _, refs := range refsTable {
			created, err = Add(ioctx, rtName, refs)
			assert.False(ts, created)
			assert.NoError(ts, err)
		}

		deleted, err := Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref1": reftype.Normal,
			"ref2": reftype.Normal,
			"ref3": reftype.Normal,
			"ref4": reftype.Normal,
			"ref5": reftype.Normal,
		})
		assert.NoError(ts, err)
		assert.True(ts, deleted)
	})
}

func TestRTMask(t *testing.T) {
	t.Parallel()

	// Bulk masking all refs should result in reftracker object deletion.
	t.Run("MaskAllBulk", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
		keys := []string{"ref1", "ref2", "ref3"}
		refsToAdd := make(map[string]struct{})
		refsToRemove := make(map[string]reftype.RefType)
		for _, k := range keys {
			refsToAdd[k] = struct{}{}
			refsToRemove[k] = reftype.Mask
		}

		created, err := Add(ioctx, rtName, refsToAdd)
		assert.NoError(ts, err)
		assert.True(ts, created)

		deleted, err := Remove(ioctx, rtName, refsToRemove)
		assert.NoError(ts, err)
		assert.True(ts, deleted)
	})

	// Masking all refs one-by-one should result in reftracker object deletion in
	// the last Remove() call.
	t.Run("RemoveSingle", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())

		created, err := Add(ioctx, rtName, map[string]struct{}{
			"ref1": {},
			"ref2": {},
			"ref3": {},
		})
		assert.NoError(ts, err)
		assert.True(ts, created)

		for _, k := range []string{"ref3", "ref2"} {
			deleted, errRemove := Remove(ioctx, rtName, map[string]reftype.RefType{
				k: reftype.Mask,
			})
			assert.NoError(ts, errRemove)
			assert.False(ts, deleted)
		}

		// Remove the last reference. It should delete the whole reftracker object
		// too.
		deleted, err := Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref1": reftype.Mask,
		})
		assert.NoError(ts, err)
		assert.True(ts, deleted)
	})

	// Bulk removing two (out of 3) refs and then masking the ref that's left
	// should result in reftracker object deletion in the last Remove() call.
	t.Run("RemoveBulkMaskSingle", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())

		created, err := Add(ioctx, rtName, map[string]struct{}{
			"ref1": {},
			"ref2": {},
			"ref3": {},
		})
		assert.NoError(ts, err)
		assert.True(ts, created)

		deleted, err := Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref1": reftype.Normal,
			"ref2": reftype.Normal,
		})
		assert.NoError(ts, err)
		assert.False(ts, deleted)

		deleted, err = Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref3": reftype.Mask,
		})
		assert.NoError(ts, err)
		assert.True(ts, deleted)
	})

	// Bulk masking two (out of 3) refs and then removing the ref that's left
	// should result in reftracker object deletion in the last Remove() call.
	t.Run("MaskSingleRemoveBulk", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())

		created, err := Add(ioctx, rtName, map[string]struct{}{
			"ref1": {},
			"ref2": {},
			"ref3": {},
		})
		assert.NoError(ts, err)
		assert.True(ts, created)

		deleted, err := Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref1": reftype.Mask,
			"ref2": reftype.Mask,
		})
		assert.NoError(ts, err)
		assert.False(ts, deleted)

		deleted, err = Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref3": reftype.Normal,
		})
		assert.NoError(ts, err)
		assert.True(ts, deleted)
	})

	// Verify that masking refs hides them from future Add()s.
	t.Run("MaskAndAdd", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())

		created, err := Add(ioctx, rtName, map[string]struct{}{
			"ref1": {},
			"ref2": {},
			"ref3": {},
		})
		assert.NoError(ts, err)
		assert.True(ts, created)

		deleted, err := Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref1": reftype.Mask,
			"ref2": reftype.Mask,
		})
		assert.NoError(ts, err)
		assert.False(ts, deleted)

		created, err = Add(ioctx, rtName, map[string]struct{}{
			"ref1": {},
			"ref2": {},
		})
		assert.NoError(ts, err)
		assert.False(ts, created)

		deleted, err = Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref3": reftype.Normal,
		})
		assert.NoError(ts, err)
		assert.True(ts, deleted)
	})

	// Verify that masked refs may be removed with reftype.Normal and re-added.
	t.Run("MaskRemoveAdd", func(ts *testing.T) {
		ts.Parallel()

		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())

		created, err := Add(ioctx, rtName, map[string]struct{}{
			"ref1": {},
			"ref2": {},
			"ref3": {},
		})
		assert.NoError(ts, err)
		assert.True(ts, created)

		deleted, err := Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref1": reftype.Mask,
			"ref2": reftype.Mask,
		})
		assert.NoError(ts, err)
		assert.False(ts, deleted)

		deleted, err = Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref1": reftype.Normal,
			"ref2": reftype.Normal,
		})
		assert.NoError(ts, err)
		assert.False(ts, deleted)

		created, err = Add(ioctx, rtName, map[string]struct{}{
			"ref1": {},
			"ref2": {},
		})
		assert.NoError(ts, err)
		assert.False(ts, created)

		deleted, err = Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref3": reftype.Normal,
		})
		assert.NoError(ts, err)
		assert.False(ts, deleted)

		deleted, err = Remove(ioctx, rtName, map[string]reftype.RefType{
			"ref1": reftype.Normal,
			"ref2": reftype.Normal,
		})
		assert.NoError(ts, err)
		assert.True(ts, deleted)
	})
}
