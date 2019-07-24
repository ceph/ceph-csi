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
	"fmt"
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
	VolNamePrefix   string
}

// This maximum comes from the CSI spec on max bytes allowed in the various CSI ID fields
const maxVolIDLen = 128

// volIDVersion{V1,V2...} is the version number of volume ID encoding scheme
const (
	_ = iota
	// EncodingV1 is to support decoding of the ID encoded using version v1
	EncodingV1
	// EncodingV2 supports volumenameprefix encoding
	EncodingV2
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

/*
ComposeCSIID composes a CSI ID from passed in parameters.
Version 2 of the encoding scheme is as follows,
	[csi_id_version=1:4byte] + [-:1byte]
	[length of clusterID=1:4byte] + [-:1byte]
	[clusterID:36bytes (MAX)] + [-:1byte]
	[poolID:16bytes] + [-:1byte]
	[length of volNamePrefix=1:4byte] + [-:1byte]
	[volNamePrefix:36bytes (MAX)] + [-:1byte]
	[ObjectUUID:36bytes]

	Total of constant field lengths, including '-' field separators would hence be,
	4+1+4+1+4+1+1+16+1+36 = 69
*/
const (
	knownV1FieldSize = 64
	knownV2FieldSize = 69
	uuidSize         = 36
	volNamePrefix    = "volumenameprefix"
	snapNamePrefix   = "snapshotnameprefix"
)

// ComposeCSIID composes a CSI ID from passed in parameters
func (ci CSIIdentifier) ComposeCSIID() (string, error) {
	buf16 := make([]byte, 2)
	buf64 := make([]byte, 8)

	ci.EncodingVersion = EncodingV2
	if (knownV2FieldSize+len(ci.ClusterID))+len(ci.VolNamePrefix) > maxVolIDLen {
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

	// remove '-' added for prefix and store
	ci.VolNamePrefix = ci.VolNamePrefix[:len(ci.VolNamePrefix)-1]

	binary.BigEndian.PutUint16(buf16, uint16(len(ci.VolNamePrefix)))
	volNamePrefixLength := hex.EncodeToString(buf16)

	return strings.Join([]string{versionEncodedHex, clusterIDLength, ci.ClusterID,
		poolIDEncodedHex, volNamePrefixLength, ci.VolNamePrefix, ci.ObjectUUID}, "-"), nil
}

/*
DecomposeV2CSIID composes a CSIIdentifier from passed in string
*/
func (ci *CSIIdentifier) DecomposeV2CSIID(composedCSIID string) (err error) {
	bytesToProcess := uint16(len(composedCSIID))

	// if length is less that expected constant elements, then bail out!
	if bytesToProcess < knownV2FieldSize {
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
	nextFieldStartIdx := 10 + clusterIDLength + 1
	if bytesToProcess < 17 {
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
	buf16, err = hex.DecodeString(composedCSIID[nextFieldStartIdx : nextFieldStartIdx+4])
	if err != nil {
		return err
	}
	volNamePrefixLength := binary.BigEndian.Uint16(buf16)
	// 4 for length encoding and 1 for '-' separator
	bytesToProcess -= 5
	nextFieldStartIdx += 5
	if bytesToProcess < (volNamePrefixLength + 1) {
		return errors.New("failed to decode CSI identifier, string underflow")
	}
	ci.VolNamePrefix = composedCSIID[nextFieldStartIdx : nextFieldStartIdx+volNamePrefixLength]

	// add - if to the prefix as we removed it while encoding it
	ci.VolNamePrefix += "-"
	// 16 for poolID encoding and 1 for '-' separator
	// additional 1 for '-' separator
	bytesToProcess -= (volNamePrefixLength + 1)
	nextFieldStartIdx += volNamePrefixLength + 1
	// has to be an exact match
	if bytesToProcess != uuidSize {
		return errors.New("failed to decode CSI identifier, string size mismatch")
	}

	ci.ObjectUUID = composedCSIID[nextFieldStartIdx : nextFieldStartIdx+uuidSize]

	return err
}

/*
DecomposeCSIID composes a CSIIdentifier from passed in string and also based
on the Encoding version of the CSIID
*/
func (ci *CSIIdentifier) DecomposeCSIID(composedCSIID string) (err error) {

	encodingVersion, err := ci.getVolEncodingVersion(composedCSIID)
	if err != nil {
		return err
	}
	if encodingVersion == EncodingV1 {
		err := ci.DecomposeV1CSIID(composedCSIID)
		if err == nil {
			ci.VolNamePrefix = "csi-vol-"
		}
		return err
	}
	return ci.DecomposeV2CSIID(composedCSIID)
}

/*
DecomposeV1CSIID composes a CSIIdentifier from passed in string
*/
func (ci *CSIIdentifier) DecomposeV1CSIID(composedCSIID string) (err error) {
	bytesToProcess := uint16(len(composedCSIID))

	// if length is less that expected constant elements, then bail out!
	if bytesToProcess < knownV1FieldSize {
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
	nextFieldStartIdx := 10 + clusterIDLength + 1

	if bytesToProcess < 17 {
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

// GetVolNamePrefix checks volumenameprefix is present in storageclass
// parameters and returns key is present in parameters and custom volume name
// if present
func GetVolNamePrefix(param map[string]string) (string, error) {
	name, ok := param[volNamePrefix]
	if ok {
		ok = isValidName(name)
		if !ok {
			return "", fmt.Errorf("%s is not in valid format", name)
		}
		return name, nil
	}
	return "", nil
}

// GetSnapNamePrefix checks snapnameprefix is present in snapshotclass
// parameters and returns the custom vol name
func GetSnapNamePrefix(param map[string]string) (string, error) {
	name, ok := param[snapNamePrefix]
	if ok {
		ok = isValidName(name)
		if !ok {
			return "", fmt.Errorf("%s is not in valid format", name)
		}
		return name, nil
	}
	return "", nil

}

// getVolEncodingVersion returns the encoding version, this will be helpful to
// decode older volumes with version 1 and new volume with version 2 supports
// volume encoding
func (ci *CSIIdentifier) getVolEncodingVersion(composedCSIID string) (uint16, error) {
	// if length is less that expected constant elements, then bail out!
	bytesToProcess := uint16(len(composedCSIID))
	if bytesToProcess < knownV1FieldSize {
		return 0, errors.New("failed to decode CSI identifier, string underflow")

	}
	buf16, err := hex.DecodeString(composedCSIID[0:4])
	if err != nil {
		return 0, err
	}
	encodingVersion := binary.BigEndian.Uint16(buf16)
	return encodingVersion, nil

}

// GenerateID generates a volume ID based on passed in parameters and version, to be returned
// to the CO system
func GenerateID(monitors string, cr *Credentials, pool, clusterID, objUUID, namePrefix string) (string, error) {
	poolID, err := GetPoolID(monitors, cr, pool)
	if err != nil {
		return "", err
	}

	// generate the volume ID to return to the CO system
	vi := CSIIdentifier{
		LocationID:    poolID,
		ClusterID:     clusterID,
		VolNamePrefix: namePrefix,
		ObjectUUID:    objUUID,
	}

	volID, err := vi.ComposeCSIID()

	return volID, err
}
