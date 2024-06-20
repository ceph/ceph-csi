package lock

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ceph/ceph-csi/internal/util/radosmutex/lockstate"
)

// Lock represents a lock with an owner, state, and expiry time.
type Lock struct {
	LockOwner  string
	LockState  lockstate.LockState
	LockExpiry time.Time
}

const (
	// LockOwnerMaxSize defines the maximum size of the lock owner string in bytes.
	LockOwnerMaxSize = 256
	// TimeStampSize defines the size of the timestamp in bytes.
	TimeStampSize = 8
)

// ToBytes serializes the Lock structure into a byte slice.
func (l *Lock) ToBytes() ([]byte, error) {
	ownerBytes := []byte(l.LockOwner)
	if len(ownerBytes) > LockOwnerMaxSize {
		return nil, fmt.Errorf("lock owner exceeds max size of %d bytes", LockOwnerMaxSize)
	}

	buffer := new(bytes.Buffer)

	// Write the length of the lock owner string
	if err := binary.Write(buffer, binary.LittleEndian, int16(len(ownerBytes))); err != nil {
		return nil, err
	}

	// Write the lock owner string
	if _, err := buffer.Write(ownerBytes); err != nil {
		return nil, err
	}

	// Write the lock state
	if _, err := buffer.Write(lockstate.ToBytes(l.LockState)); err != nil {
		return nil, err
	}

	// Write the lock expiry timestamp
	expiryBytes := make([]byte, TimeStampSize)
	binary.LittleEndian.PutUint64(expiryBytes, uint64(l.LockExpiry.Unix()))
	if _, err := buffer.Write(expiryBytes); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// FromBytes deserializes the byte slice into a Lock structure.
func (l *Lock) FromBytes(data []byte) error {
	buffer := bytes.NewReader(data)

	// Read the length of the lock owner string
	var ownerLength int16
	if err := binary.Read(buffer, binary.LittleEndian, &ownerLength); err != nil {
		return err
	}

	// Read the lock owner string
	ownerBytes := make([]byte, ownerLength)
	if _, err := buffer.Read(ownerBytes); err != nil {
		return err
	}
	l.LockOwner = string(ownerBytes)

	// Read the lock state
	stateBytes := make([]byte, lockstate.LockStateSize)
	if _, err := buffer.Read(stateBytes); err != nil {
		return err
	}
	lockState, err := lockstate.FromBytes(stateBytes)
	if err != nil {
		return err
	}
	l.LockState = lockState

	// Read the lock expiry timestamp
	expiryBytes := make([]byte, TimeStampSize)
	if _, err := buffer.Read(expiryBytes); err != nil {
		return err
	}
	expiryUnix := int64(binary.LittleEndian.Uint64(expiryBytes))
	l.LockExpiry = time.Unix(expiryUnix, 0)

	return nil
}
