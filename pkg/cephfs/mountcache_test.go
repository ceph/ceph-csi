package cephfs

import (
	"testing"
)

func init() {
}

func TestMountOneCacheEntry(t *testing.T) {
}

func TestRemountHisMountedPath(t *testing.T) {
}

func TestNodeStageVolume(t *testing.T) {
}

func TestNodeUnStageVolume(t *testing.T) {
}

func TestNodePublishVolume(t *testing.T) {
}

func TestNodeUnpublishVolume(t *testing.T) {
}

func TestEncodeDecodeCredentials(t *testing.T) {
	secrets := make(map[string]string)
	secrets["user_1"] = "value_1"
	enSecrets := encodeCredentials(secrets)
	deSecrets := decodeCredentials(enSecrets)
	for key, value := range secrets {
		if deSecrets[key] != value {
			t.Errorf("key %s value %s  not equal %s after encode decode", key, value, deSecrets[key])
		}
	}
}
