package lock

import (
	"strings"
	"testing"
	"time"

	"github.com/ceph/ceph-csi/internal/util/radosmutex/lockstate"
)

func TestLockSerializationDeserialization(t *testing.T) {
	// Setup
	lockInstance := Lock{
		LockOwner:  "testUser",
		LockState:  lockstate.Unlocked,
		LockExpiry: time.Now().Add(10 * time.Minute),
	}

	// Test Successful Serialization and Deserialization
	serialized, err := lockInstance.ToBytes()
	if err != nil {
		t.Fatalf("Failed to serialize lock: %v", err)
	}

	deserializedLock := Lock{}
	err = deserializedLock.FromBytes(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize lock: %v", err)
	}

	t.Logf("Original LockOwner: %s", lockInstance.LockOwner)
	t.Logf("Deserialized LockOwner: %s", deserializedLock.LockOwner)
	t.Logf("Original LockState: %v", lockInstance.LockState)
	t.Logf("Deserialized LockState: %v", deserializedLock.LockState)
	t.Logf("Original LockExpiry: %v", lockInstance.LockExpiry)
	t.Logf("Deserialized LockExpiry: %v", deserializedLock.LockExpiry)

	// Check if the original and deserialized locks match

	if lockInstance.LockOwner != deserializedLock.LockOwner {
		t.Errorf("Serialized and deserialized lock instances do not match. LockOwner: Expected '%s', Got '%s'", lockInstance.LockOwner, deserializedLock.LockOwner)
	}

	if lockInstance.LockState != deserializedLock.LockState {
		t.Errorf("Serialized and deserialized lock instances do not match. LockState: Expected '%v', Got '%v'", lockInstance.LockState, deserializedLock.LockState)
	}

	if lockInstance.LockExpiry.Unix() != deserializedLock.LockExpiry.Unix() {
		t.Errorf("Serialized and deserialized lock instances do not match. LockExpiry: Expected '%v', Got '%v'", lockInstance.LockExpiry, deserializedLock.LockExpiry)
	}
}

func TestLockSerializationWithLongOwnerFails(t *testing.T) {
	lockInstance := Lock{
		LockOwner:  strings.Repeat("x", 300), // Creates a string longer than LockOwnerMaxSize
		LockState:  lockstate.Unlocked,
		LockExpiry: time.Now().Add(10 * time.Minute),
	}

	_, err := lockInstance.ToBytes()
	if err == nil || err.Error() != "lock owner exceeds max size of 256 bytes" {
		t.Errorf("Expected error due to long owner name, got %v", err)
	}
}

func TestLockSerializationWithEmptyFields(t *testing.T) {
	// Setup
	lockInstance := Lock{
		LockOwner:  "",
		LockState:  lockstate.Unlocked,
		LockExpiry: time.Time{},
	}

	serialized, err := lockInstance.ToBytes()
	if err != nil {
		t.Fatalf("Failed to serialize lock: %v", err)
	}

	deserializedLock := Lock{}
	err = deserializedLock.FromBytes(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize lock: %v", err)
	}

	if lockInstance.LockOwner != deserializedLock.LockOwner {
		t.Errorf("Serialized and deserialized lock instances do not match. LockOwner: Expected '%s', Got '%s'", lockInstance.LockOwner, deserializedLock.LockOwner)
	}

	if lockInstance.LockState != deserializedLock.LockState {
		t.Errorf("Serialized and deserialized lock instances do not match. LockState: Expected '%v', Got '%v'", lockInstance.LockState, deserializedLock.LockState)
	}

	if lockInstance.LockExpiry.Unix() != deserializedLock.LockExpiry.Unix() {
		t.Errorf("Serialized and deserialized lock instances do not match. LockExpiry: Expected '%v', Got '%v'", lockInstance.LockExpiry, deserializedLock.LockExpiry)
	}
}
