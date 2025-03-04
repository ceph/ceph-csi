/*
Copyright 2025 The Ceph-CSI Authors.

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

package crypto

type EncryptionType int

const (
	// EncryptionTypeInvalid signals invalid or unsupported configuration.
	EncryptionTypeInvalid EncryptionType = iota
	// EncryptionTypeNone disables encryption.
	EncryptionTypeNone
	// EncryptionTypeBlock enables block encryption.
	EncryptionTypeBlock
	// EncryptionTypeBlock enables file encryption (fscrypt).
	EncryptionTypeFile
)

const (
	encryptionTypeBlockString = "block"
	encryptionTypeFileString  = "file"
)

func ParseEncryptionType(typeStr string) EncryptionType {
	switch typeStr {
	case encryptionTypeBlockString:
		return EncryptionTypeBlock
	case encryptionTypeFileString:
		return EncryptionTypeFile
	case "":
		return EncryptionTypeNone
	default:
		return EncryptionTypeInvalid
	}
}

func (encType EncryptionType) String() string {
	switch encType {
	case EncryptionTypeBlock:
		return encryptionTypeBlockString
	case EncryptionTypeFile:
		return encryptionTypeFileString
	case EncryptionTypeNone:
		return ""
	case EncryptionTypeInvalid:
		return "INVALID"
	default:
		return "UNKNOWN"
	}
}
