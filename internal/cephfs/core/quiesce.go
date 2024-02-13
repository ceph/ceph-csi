/*
Copyright 2024 The Ceph-CSI Authors.

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

package core

import (
	"context"

	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/ceph/go-ceph/cephfs/admin"
)

type QuiesceState string

const (
	Released  QuiesceState = "RELEASED"
	Quiescing QuiesceState = "QUIESCING"
	Quiesced  QuiesceState = "QUIESCED"
)

// GetQuiesceState returns the quiesce state of the filesystem.
func GetQuiesceState(set admin.QuiesceState) QuiesceState {
	var state QuiesceState
	switch set.Name {
	case "RELEASED":
		state = Released
	case "QUIESCING":
		state = Quiescing
	case "QUIESCED":
		state = Quiesced
	default:
		state = QuiesceState(set.Name)
	}

	return state
}

type FSQuiesceClient interface {
	// Destroy destroys the connection used for FSAdmin.
	Destroy()
	// FSQuiesce quiesces the subvolumes in the filesystem.
	FSQuiesce(
		ctx context.Context,
		reserveName string,
	) (*admin.QuiesceInfo, error)
	// GetVolumes returns the list of volumes in the filesystem that are to be
	// quiesced.
	GetVolumes() []Volume
	// FSQuiesceWithExpireTimeout quiesces the subvolumes in the filesystem
	// with an expiration timeout. it should be used after FSQuiesce to reset
	// the expire timeout. This helps in keeping the subvolumes in the
	// filesystem in quiesced state until all snapshots are taken.
	FSQuiesceWithExpireTimeout(ctx context.Context,
		reserveName string,
	) (*admin.QuiesceInfo, error)
	// ResetFSQuiesce resets the quiesce timeout for the subvolumes in
	// the filesystem.
	ResetFSQuiesce(ctx context.Context,
		reserveName string,
	) (*admin.QuiesceInfo, error)
	// ReleaseFSQuiesce releases the quiesce on the subvolumes in the
	// filesystem.
	ReleaseFSQuiesce(ctx context.Context,
		reserveName string,
	) (*admin.QuiesceInfo, error)
}

type Volume struct {
	VolumeID  string
	ClusterID string
}

type fsQuiesce struct {
	connection *util.ClusterConnection
	fsName     string
	volumes    []Volume
	// subVolumeGroupMapping is a map of subvolumes to groups.
	subVolumeGroupMapping map[string][]string
	fsa                   *admin.FSAdmin
}

// NewFSQuiesce returns a new instance of fsQuiesce. It
// take the filesystem name, the list of volumes to be quiesced, the mapping of
// subvolumes to groups and the cluster connection as input.
func NewFSQuiesce(
	fsName string,
	volumes []Volume,
	mapping map[string][]string,
	conn *util.ClusterConnection,
) (FSQuiesceClient, error) {
	fsa, err := conn.GetFSAdmin()
	if err != nil {
		return nil, err
	}

	return &fsQuiesce{
		connection:            conn,
		fsName:                fsName,
		volumes:               volumes,
		subVolumeGroupMapping: mapping,
		fsa:                   fsa,
	}, nil
}

// Destroy destroys the connection used for FSAdmin.
func (fq *fsQuiesce) Destroy() {
	if fq.connection != nil {
		fq.connection.Destroy()
	}
}

// GetVolumes returns the list of volumes in the filesystem that are to be
// quiesced.
func (fq *fsQuiesce) GetVolumes() []Volume {
	return fq.volumes
}

// getMembers returns the list of names in the format
// group/subvolume that are to be quiesced. This is the format that the
// ceph fs quiesce expects.
// Example: ["group1/subvolume1", "group1/subvolume2", "group2/subvolume1"].
func (fq *fsQuiesce) getMembers() []string {
	volName := []string{}
	for svg, sb := range fq.subVolumeGroupMapping {
		for _, s := range sb {
			name := svg + "/" + s
			volName = append(volName, name)
		}
	}

	return volName
}

func (fq *fsQuiesce) FSQuiesce(
	ctx context.Context,
	reserveName string,
) (*admin.QuiesceInfo, error) {
	opt := &admin.FSQuiesceOptions{
		Timeout:    180,
		AwaitFor:   0,
		Expiration: 180,
	}
	log.DebugLog(ctx,
		"FSQuiesce for reserveName %s: members:%v options:%v",
		reserveName,
		fq.getMembers(),
		opt)
	resp, err := fq.fsa.FSQuiesce(fq.fsName, admin.NoGroup, fq.getMembers(), reserveName, opt)
	if resp != nil {
		qInfo := resp.Sets[reserveName]

		return &qInfo, nil
	}

	log.ErrorLog(ctx, "failed to quiesce filesystem %s", err)

	return nil, err
}

func (fq *fsQuiesce) FSQuiesceWithExpireTimeout(ctx context.Context,
	reserveName string,
) (*admin.QuiesceInfo, error) {
	opt := &admin.FSQuiesceOptions{
		Timeout:    180,
		AwaitFor:   0,
		Expiration: 180,
	}
	log.DebugLog(ctx,
		"FSQuiesceWithExpireTimeout for reserveName %s: members:%v options:%v",
		reserveName,
		fq.getMembers(),
		opt)
	resp, err := fq.fsa.FSQuiesce(fq.fsName, admin.NoGroup, fq.getMembers(), reserveName, opt)
	if resp != nil {
		qInfo := resp.Sets[reserveName]

		return &qInfo, nil
	}

	log.ErrorLog(ctx, "failed to quiesce filesystem with expire timeout %s", err)

	return nil, err
}

func (fq *fsQuiesce) ResetFSQuiesce(ctx context.Context,
	reserveName string,
) (*admin.QuiesceInfo, error) {
	opt := &admin.FSQuiesceOptions{
		Reset:      true,
		AwaitFor:   0,
		Timeout:    180,
		Expiration: 180,
	}
	// Reset the filesystem quiesce so that the timer will be reset, and we can
	// reuse the same reservation if it has already failed or timed out.
	log.DebugLog(ctx,
		"ResetFSQuiesce for reserveName %s: members:%v options:%v",
		reserveName,
		fq.getMembers(),
		opt)
	resp, err := fq.fsa.FSQuiesce(fq.fsName, admin.NoGroup, fq.getMembers(), reserveName, opt)
	if resp != nil {
		qInfo := resp.Sets[reserveName]

		return &qInfo, nil
	}

	log.ErrorLog(ctx, "failed to reset timeout for quiesce filesystem %s", err)

	return nil, err
}

func (fq *fsQuiesce) ReleaseFSQuiesce(ctx context.Context,
	reserveName string,
) (*admin.QuiesceInfo, error) {
	opt := &admin.FSQuiesceOptions{
		AwaitFor: 0,
		Release:  true,
	}
	log.DebugLog(ctx,
		"ReleaseFSQuiesce for reserveName %s: members:%v options:%v",
		reserveName,
		fq.getMembers(),
		opt)
	resp, err := fq.fsa.FSQuiesce(fq.fsName, admin.NoGroup, []string{}, reserveName, opt)
	if resp != nil {
		qInfo := resp.Sets[reserveName]

		return &qInfo, nil
	}

	log.ErrorLog(ctx, "failed to release quiesce of filesystem %s", err)

	return nil, err
}
