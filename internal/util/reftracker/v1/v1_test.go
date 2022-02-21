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
	"testing"

	"github.com/ceph/ceph-csi/internal/util/reftracker/errors"
	"github.com/ceph/ceph-csi/internal/util/reftracker/radoswrapper"
	"github.com/ceph/ceph-csi/internal/util/reftracker/reftype"

	"github.com/stretchr/testify/assert"
)

func TestV1Read(t *testing.T) {
	t.Parallel()

	const rtName = "hello-rt"

	var (
		gen = uint64(0)

		validObj = radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
			Objs: map[string]*radoswrapper.FakeObj{
				rtName: {
					Oid:  rtName,
					Data: []byte{0, 0, 0, 0},
					Omap: make(map[string][]byte),
				},
			},
		})

		invalidObjs = []*radoswrapper.FakeIOContext{
			// Missing object.
			radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados()),
			// Bad generation number.
			radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
				Objs: map[string]*radoswrapper.FakeObj{
					rtName: {
						Ver:  123,
						Oid:  rtName,
						Data: []byte{0, 0, 0, 0},
					},
				},
			}),
			// Refcount overflow.
			radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
				Objs: map[string]*radoswrapper.FakeObj{
					rtName: {
						Oid:  rtName,
						Data: []byte{0xFF, 0xFF, 0xFF, 0xFF},
					},
				},
			}),
		}

		refsToAdd = map[string]struct{}{"ref1": {}}
	)

	err := Add(validObj, rtName, gen, refsToAdd)
	assert.NoError(t, err)

	for i := range invalidObjs {
		err = Add(invalidObjs[i], rtName, gen, refsToAdd)
		assert.Error(t, err)
	}

	// Check for correct error type for wrong gen num.
	err = Add(invalidObjs[1], rtName, gen, refsToAdd)
	assert.Error(t, err)
	assert.True(t, goerrors.Is(err, errors.ErrObjectOutOfDate))
}

func TestV1Init(t *testing.T) {
	t.Parallel()

	const rtName = "hello-rt"

	var (
		emptyRados = radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
			Objs: map[string]*radoswrapper.FakeObj{},
		})

		alreadyExists = radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
			Objs: map[string]*radoswrapper.FakeObj{
				rtName: {},
			},
		})

		refsToInit = map[string]struct{}{"ref1": {}}
	)

	err := Init(emptyRados, rtName, refsToInit)
	assert.NoError(t, err)

	err = Init(alreadyExists, rtName, refsToInit)
	assert.Error(t, err)
}

func TestV1Add(t *testing.T) {
	t.Parallel()

	const rtName = "hello-rt"

	var (
		shouldSucceed = []struct {
			before    *radoswrapper.FakeObj
			refsToAdd map[string]struct{}
			after     *radoswrapper.FakeObj
		}{
			// Add a new ref.
			{
				before: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(1).toBytes(),
				},
				refsToAdd: map[string]struct{}{
					"ref2": {},
				},
				after: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 1,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
						"ref2": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(2).toBytes(),
				},
			},
			// Try to add a ref that's already tracked.
			{
				before: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(1).toBytes(),
				},
				refsToAdd: map[string]struct{}{
					"ref1": {},
				},
				after: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(1).toBytes(),
				},
			},
			// Try to add a ref that's masked.
			{
				before: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
						"ref2": reftype.ToBytes(reftype.Mask),
					},
					Data: refCount(1).toBytes(),
				},
				refsToAdd: map[string]struct{}{
					"ref1": {},
				},
				after: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
						"ref2": reftype.ToBytes(reftype.Mask),
					},
					Data: refCount(1).toBytes(),
				},
			},
		}

		shouldFail = []*radoswrapper.FakeIOContext{
			// Missing object.
			radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados()),
			// Bad generation number.
			radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
				Objs: map[string]*radoswrapper.FakeObj{
					rtName: {
						Ver:  123,
						Oid:  rtName,
						Data: []byte{0, 0, 0, 0},
					},
				},
			}),
			// Refcount overflow.
			radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
				Objs: map[string]*radoswrapper.FakeObj{
					rtName: {
						Oid:  rtName,
						Data: []byte{0xFF, 0xFF, 0xFF, 0xFF},
					},
				},
			}),
		}
	)

	for i := range shouldSucceed {
		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
		ioctx.Rados.Objs[rtName] = shouldSucceed[i].before

		err := Add(ioctx, rtName, 0, shouldSucceed[i].refsToAdd)
		assert.NoError(t, err)
		assert.Equal(t, shouldSucceed[i].after, ioctx.Rados.Objs[rtName])
	}

	for i := range shouldFail {
		err := Add(shouldFail[i], rtName, 0, map[string]struct{}{"ref1": {}})
		assert.Error(t, err)
	}

	// Check for correct error type for wrong gen num.
	err := Add(shouldFail[1], rtName, 0, map[string]struct{}{"ref1": {}})
	assert.Error(t, err)
	assert.True(t, goerrors.Is(err, errors.ErrObjectOutOfDate))
}

func TestV1Remove(t *testing.T) {
	t.Parallel()

	const rtName = "hello-rt"

	var (
		shouldSucceed = []struct {
			before       *radoswrapper.FakeObj
			refsToRemove map[string]reftype.RefType
			after        *radoswrapper.FakeObj
			deleted      bool
		}{
			// Remove without deleting the reftracker object.
			{
				before: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
						"ref2": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(2).toBytes(),
				},
				refsToRemove: map[string]reftype.RefType{
					"ref1": reftype.Normal,
				},
				after: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 1,
					Omap: map[string][]byte{
						"ref2": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(1).toBytes(),
				},
				deleted: false,
			},
			// Remove and delete the reftracker object.
			{
				before: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(1).toBytes(),
				},
				refsToRemove: map[string]reftype.RefType{
					"ref1": reftype.Normal,
				},
				after:   nil,
				deleted: true,
			},
			// Remove and delete the reftracker object.
			{
				before: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(1).toBytes(),
				},
				refsToRemove: map[string]reftype.RefType{
					"ref1": reftype.Normal,
				},
				after:   nil,
				deleted: true,
			},
			// Mask a ref without deleting reftracker object.
			{
				before: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
						"ref2": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(2).toBytes(),
				},
				refsToRemove: map[string]reftype.RefType{
					"ref2": reftype.Mask,
				},
				after: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 1,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
						"ref2": reftype.ToBytes(reftype.Mask),
					},
					Data: refCount(1).toBytes(),
				},
				deleted: false,
			},
			// Mask a ref and delete reftracker object.
			{
				before: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(1).toBytes(),
				},
				refsToRemove: map[string]reftype.RefType{
					"ref1": reftype.Mask,
				},
				after:   nil,
				deleted: true,
			},
			// Add a masking ref.
			{
				before: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(1).toBytes(),
				},
				refsToRemove: map[string]reftype.RefType{
					"ref2": reftype.Mask,
				},
				after: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 1,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
						"ref2": reftype.ToBytes(reftype.Mask),
					},
					Data: refCount(1).toBytes(),
				},
				deleted: false,
			},
			// Try to remove non-existent ref.
			{
				before: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(1).toBytes(),
				},
				refsToRemove: map[string]reftype.RefType{
					"ref2": reftype.Normal,
				},
				after: &radoswrapper.FakeObj{
					Oid: rtName,
					Ver: 0,
					Omap: map[string][]byte{
						"ref1": reftype.ToBytes(reftype.Normal),
					},
					Data: refCount(1).toBytes(),
				},
				deleted: false,
			},
		}

		// Bad generation number.
		badGen = radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
			Objs: map[string]*radoswrapper.FakeObj{
				rtName: {
					Ver: 123,
				},
			},
		})
	)

	for i := range shouldSucceed {
		ioctx := radoswrapper.NewFakeIOContext(radoswrapper.NewFakeRados())
		ioctx.Rados.Objs[rtName] = shouldSucceed[i].before

		deleted, err := Remove(ioctx, rtName, 0, shouldSucceed[i].refsToRemove)
		assert.NoError(t, err)
		assert.Equal(t, shouldSucceed[i].deleted, deleted)
		assert.Equal(t, shouldSucceed[i].after, ioctx.Rados.Objs[rtName])
	}

	_, err := Remove(badGen, rtName, 0, map[string]reftype.RefType{"ref": reftype.Normal})
	assert.Error(t, err)
	assert.True(t, goerrors.Is(err, errors.ErrObjectOutOfDate))
}
