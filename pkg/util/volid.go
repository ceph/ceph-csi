/*
Copyright 2019 Ceph-CSI authors.

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

package util

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
)

/*
VolumeIdentifier contains the elements that form a volume ID to be returned by CSI, and contains
enough information to decompose and extract required cluster and pool information to locate the
volume that relates to the volume ID.

The volume identifier is composed as elaborated in the comment against ComposeVolID and thus,
DecomposeVolID is the inverse of the same function.

The VolumeIdentifier structure carries the following fields,
- PoolID: 32 bit integer of the pool that the volume belongs to, where the ID comes from Ceph pool
  identifier for the corresponding pool name.
- EncodingVersion: Carries the version number of the encoding scheme used to encode the volume ID,
  and is preserved for any future proofing w.r.t changes in the encoding scheme, and to retain
  ability to parse backward compatible encodings.
- ClusterID: Is a unique ID per cluster that the CSI instance is serving and is restricted to
  lengths that can be accommodated in the encoding scheme.
- ImageName: Is the on-disk image name of the CSI volume that corresponds to this volume ID.
*/
type VolumeIdentifier struct {
	PoolID          uint32
	EncodingVersion uint16
	ClusterID       string
	ImageName       string
}

// This maximum comes from the CSI spec on max bytes allowed in the VolumeID field
const maxVolIDLen = 128

/*
ComposeVolID composes a volume ID from passed in parameters.
Version 1 of the encoding scheme is as follows,
	[volume_id_version=1:4byte] + [-:1byte]
	[length of clusterID=1:4byte] + [-:1byte]
	[clusterID:36bytes (MAX)] + [-:1byte]
	[poolID:8bytes] + [-:1byte]
	[length of ImageName=1:4byte] + [-:1byte]
	[ImageName: (should not overflow total length of maxVolIDLen)]

	Total of constant field lengths, including '-' field separators would hence be,
	4+1+4+1+1+8+1+4+1 = 25
*/
func (vi VolumeIdentifier) ComposeVolID() (string, error) {
	buf16 := make([]byte, 2)
	buf32 := make([]byte, 4)

	if (25 + len(vi.ClusterID) + len(vi.ImageName)) > maxVolIDLen {
		return "", errors.New("volume ID encoding length overflow")
	}

	binary.BigEndian.PutUint16(buf16, vi.EncodingVersion)
	versionEncodedHex := hex.EncodeToString(buf16)

	binary.BigEndian.PutUint16(buf16, uint16(len(vi.ClusterID)))
	clusterIDLength := hex.EncodeToString(buf16)

	binary.BigEndian.PutUint32(buf32, vi.PoolID)
	poolIDEncodedHex := hex.EncodeToString(buf32)

	binary.BigEndian.PutUint16(buf16, uint16(len(vi.ImageName)))
	imageNameLength := hex.EncodeToString(buf16)

	return strings.Join([]string{versionEncodedHex, clusterIDLength, vi.ClusterID,
		poolIDEncodedHex, imageNameLength, vi.ImageName}, "-"), nil
}

/*
DecomposeVolID composes a VolumeIdentifier from passed in string
*/
func (vi *VolumeIdentifier) DecomposeVolID(composedVolID string) (err error) {
	bytesToProcess := uint16(len(composedVolID))

	// if length is less that expected constant elements, then bail out!
	if bytesToProcess < 25 {
		return errors.New("failed to decode volume identifier, string underflow")
	}

	buf16, err := hex.DecodeString(composedVolID[0:4])
	if err != nil {
		return err
	}
	vi.EncodingVersion = binary.BigEndian.Uint16(buf16)
	// 4 for version encoding and 1 for '-' separator
	bytesToProcess -= 5

	buf16, err = hex.DecodeString(composedVolID[5:9])
	if err != nil {
		return err
	}
	clusterIDLength := binary.BigEndian.Uint16(buf16)
	// 4 for length encoding and 1 for '-' separator
	bytesToProcess -= 5

	if bytesToProcess < (clusterIDLength + 1) {
		return errors.New("failed to decode volume identifier, string underflow")
	}
	vi.ClusterID = composedVolID[10 : 10+clusterIDLength]
	// additional 1 for '-' separator
	bytesToProcess -= (clusterIDLength + 1)
	nextFieldStartIdx := 10 + clusterIDLength + 1

	if bytesToProcess < 9 {
		return errors.New("failed to decode volume identifier, string underflow")
	}
	buf32, err := hex.DecodeString(composedVolID[nextFieldStartIdx : nextFieldStartIdx+8])
	if err != nil {
		return err
	}
	vi.PoolID = binary.BigEndian.Uint32(buf32)
	// 8 for length encoding and 1 for '-' separator
	bytesToProcess -= 9
	nextFieldStartIdx = nextFieldStartIdx + 9

	if bytesToProcess < 5 {
		return errors.New("failed to decode volume identifier, string underflow")
	}
	buf16, err = hex.DecodeString(composedVolID[nextFieldStartIdx : nextFieldStartIdx+4])
	if err != nil {
		return err
	}
	imageNameLength := binary.BigEndian.Uint16(buf16)
	// 4 for length encoding and 1 for '-' separator
	bytesToProcess -= 5
	nextFieldStartIdx = nextFieldStartIdx + 5

	// has to be an exact match
	if bytesToProcess < imageNameLength {
		return errors.New("failed to decode volume identifier, string underflow")
	}
	if bytesToProcess > imageNameLength {
		return errors.New("failed to decode volume identifier, string overflow")
	}
	vi.ImageName = composedVolID[nextFieldStartIdx : nextFieldStartIdx+imageNameLength]

	return err
}
