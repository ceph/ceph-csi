package rbd

import (
	"testing"

	"github.com/ceph/ceph-csi/pkg/util"
)

type testCachePersister struct {
	volumes   map[string]rbdVolume
	snapshots map[string]rbdSnapshot
}

func (t *testCachePersister) Create(identifier string, data interface{}) error {
	return nil
}

func (t *testCachePersister) Get(identifier string, data interface{}) error {
	return nil
}

func (t *testCachePersister) ForAll(pattern string, destObj interface{}, f util.ForAllFunc) error {

	switch pattern {
	case "csi-rbd-vol-":
		for identifier, vol := range t.volumes {
			*destObj.(*rbdVolume) = vol
			if err := f(identifier); err != nil {
				return err
			}
		}
	case "csi-rbd-(.*)-snap-":
		for identifier, snap := range t.snapshots {
			*destObj.(*rbdSnapshot) = snap
			if err := f(identifier); err != nil {
				return err
			}
		}
	}

	return nil
}

func (t *testCachePersister) Delete(identifier string) error {
	return nil
}

func TestLoadExDataFromMetadataStore(t *testing.T) {
	cs := &ControllerServer{
		MetadataStore: &testCachePersister{
			volumes: map[string]rbdVolume{
				"item1": {
					VolID: "1",
				},
				"item2": {
					VolID: "2",
				},
			},
			snapshots: map[string]rbdSnapshot{
				"item1": {
					SnapID: "1",
				},
				"item2": {
					SnapID: "2",
				},
			},
		},
	}

	if err := cs.LoadExDataFromMetadataStore(); err != nil {
		t.Error(err)
	}

	if rbdVolumes["item1"] == rbdVolumes["item2"] {
		t.Error("rbd volume entries contain pointer to same volume")
	}

	if rbdSnapshots["item1"] == rbdSnapshots["item2"] {
		t.Error("rbd snapshot entries contain pointer to same snapshot")
	}
}
