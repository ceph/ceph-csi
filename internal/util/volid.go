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
CSIIdentifier contains the elements that form a CSI ID to be returned by the CSI plugin, and
contains enough information to decompose and extract required cluster and pool information to locate
the volume that relates to the CSI ID.

The CSI identifier is composed as elaborated in the comment against ComposeCSIID and thus,
DecomposeCSIID is the inverse of the same function.

The CSIIdentifier structure carries the following fields,
  - LocationID: 64 bit integer identifier determining the location of the volume on the Ceph cluster.
    It is the ID of the poolname or fsname, for RBD or CephFS backed volumes respectively.
  - EncodingVersion: Carries the version number of the encoding scheme used to encode the CSI ID,
    and is preserved for any future proofing w.r.t changes in the encoding scheme, and to retain
    ability to parse backward compatible encodings.
  - ClusterID: Is a unique ID per cluster that the CSI instance is serving and is restricted to
    lengths that can be accommodated in the encoding scheme.
  - ObjectUUID: Is the on-disk uuid of the object (image/snapshot) name, for the CSI volume that
    corresponds to this CSI ID.
*/
type CSIIdentifier struct {
	LocationID      int64
	EncodingVersion uint16
	ClusterID       string
	ObjectUUID      string
}

// This maximum comes from the CSI spec on max bytes allowed in the various CSI ID fields.
const maxVolIDLen = 128

const (
	knownFieldSize = 64
	uuidSize       = 36
)

/*
ComposeCSIID composes a CSI ID from passed in parameters.
Version 1 of the encoding scheme is as follows,

	[csi_id_version=1:4byte] + [-:1byte]
	[length of clusterID=1:4byte] + [-:1byte]
	[clusterID:36bytes (MAX)] + [-:1byte]
	[poolID:16bytes] + [-:1byte]
	[ObjectUUID:36bytes]

	Total of constant field lengths, including '-' field separators would hence be,
	4+1+4+1+1+16+1+36 = 64
*/
func (ci CSIIdentifier) ComposeCSIID() (string, error) {
	buf16 := make([]byte, 2)
	buf64 := make([]byte, 8)

	if (knownFieldSize + len(ci.ClusterID)) > maxVolIDLen {
		return "", errors.New("CSI ID encoding length overflow")
	}

	if len(ci.ObjectUUID) != uuidSize {
		return "", errors.New("CSI ID invalid object uuid")
	}

	binary.BigEndian.PutUint16(buf16, ci.EncodingVersion)
	versionEncodedHex := hex.EncodeToString(buf16)

	binary.BigEndian.PutUint16(buf16, uint16(len(ci.ClusterID)))
	clusterIDLength := hex.EncodeToString(buf16)

	binary.BigEndian.PutUint64(buf64, uint64(ci.LocationID))
	poolIDEncodedHex := hex.EncodeToString(buf64)

	return strings.Join([]string{
		versionEncodedHex, clusterIDLength, ci.ClusterID,
		poolIDEncodedHex, ci.ObjectUUID,
	}, "-"), nil
}

/*
DecomposeCSIID composes a CSIIdentifier from passed in string.
*/
func (ci *CSIIdentifier) DecomposeCSIID(composedCSIID string) error {
	bytesToProcess := uint16(len(composedCSIID))

	// if length is less that expected constant elements, then bail out!
	if bytesToProcess < knownFieldSize {
		return errors.New("failed to decode CSI identifier, string underflow")
	}

	buf16, err := hex.DecodeString(composedCSIID[0:4])
	if err != nil {
		return err
	}
	ci.EncodingVersion = binary.BigEndian.Uint16(buf16)
	// 4 for version encoding and 1 for '-' separator
	bytesToProcess -= 5

	buf16, err = hex.DecodeString(composedCSIID[5:9])
	if err != nil {
		return err
	}
	clusterIDLength := binary.BigEndian.Uint16(buf16)
	// 4 for length encoding and 1 for '-' separator
	bytesToProcess -= 5

	if bytesToProcess < (clusterIDLength + 1) {
		return errors.New("failed to decode CSI identifier, string underflow")
	}
	ci.ClusterID = composedCSIID[10 : 10+clusterIDLength]
	// additional 1 for '-' separator
	bytesToProcess -= (clusterIDLength + 1)
	nextFieldStartIdx := (10 + clusterIDLength + 1)

	// minLenToDecode is now 17 as composedCSIID should include
	// at least 16 for poolID encoding and 1 for '-' separator.
	const minLenToDecode = 17
	if bytesToProcess < minLenToDecode {
		return errors.New("failed to decode CSI identifier, string underflow")
	}
	buf64, err := hex.DecodeString(composedCSIID[nextFieldStartIdx : nextFieldStartIdx+16])
	if err != nil {
		return err
	}
	ci.LocationID = int64(binary.BigEndian.Uint64(buf64))
	// 16 for poolID encoding and 1 for '-' separator
	bytesToProcess -= 17
	nextFieldStartIdx += 17

	// has to be an exact match
	if bytesToProcess != uuidSize {
		return errors.New("failed to decode CSI identifier, string size mismatch")
	}
	ci.ObjectUUID = composedCSIID[nextFieldStartIdx : nextFieldStartIdx+uuidSize]

	return err
}
