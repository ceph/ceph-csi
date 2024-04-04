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

package version

import (
	"testing"

	"github.com/ceph/ceph-csi/internal/util/reftracker/radoswrapper"

	"github.com/stretchr/testify/require"
)

var (
	v1Bytes = []byte{0, 0, 0, 1}
	v1Value = uint32(1)

	wrongSizeVersionBytes = []byte{0, 0, 1}
)

func TestVersionBytes(t *testing.T) {
	t.Parallel()

	t.Run("ToBytes", func(ts *testing.T) {
		ts.Parallel()

		bs := ToBytes(v1Value)
		require.Equal(ts, v1Bytes, bs)
	})

	t.Run("FromBytes", func(ts *testing.T) {
		ts.Parallel()

		ver, err := FromBytes(v1Bytes)
		require.NoError(ts, err)
		require.Equal(ts, v1Value, ver)

		_, err = FromBytes(wrongSizeVersionBytes)
		require.Error(ts, err)
	})
}

func TestVersionRead(t *testing.T) {
	t.Parallel()

	const rtName = "hello-rt"

	var (
		validObj = radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
			Objs: map[string]*radoswrapper.FakeObj{
				rtName: {
					Oid: rtName,
					Xattrs: map[string][]byte{
						XattrName: v1Bytes,
					},
				},
			},
		})

		invalidObjs = []*radoswrapper.FakeIOContext{
			// Missing object.
			radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
				Objs: map[string]*radoswrapper.FakeObj{},
			}),
			// Missing xattr.
			radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
				Objs: map[string]*radoswrapper.FakeObj{
					rtName: {
						Oid: rtName,
						Xattrs: map[string][]byte{
							"some-other-xattr": v1Bytes,
						},
					},
				},
			}),
			// Wrongly sized version value.
			radoswrapper.NewFakeIOContext(&radoswrapper.FakeRados{
				Objs: map[string]*radoswrapper.FakeObj{
					rtName: {
						Oid: rtName,
						Xattrs: map[string][]byte{
							XattrName: wrongSizeVersionBytes,
						},
					},
				},
			}),
		}
	)

	ver, err := Read(validObj, rtName)
	require.NoError(t, err)
	require.Equal(t, v1Value, ver)

	for i := range invalidObjs {
		_, err = Read(invalidObjs[i], rtName)
		require.Error(t, err)
	}
}
