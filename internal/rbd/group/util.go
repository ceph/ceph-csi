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

package group

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ceph/go-ceph/rados"

	"github.com/ceph/ceph-csi/internal/journal"
	rbderrors "github.com/ceph/ceph-csi/internal/rbd/errors"
	"github.com/ceph/ceph-csi/internal/util"
	"github.com/ceph/ceph-csi/internal/util/log"
)

type commonVolumeGroup struct {
	// id is a unique value for this volume group in the Ceph cluster, it
	// is used to find the group in the journal.
	id string

	// requestName is passed by the caller when a group is created.
	requestName string

	// name is used in RBD API calls as the name of this object
	name string

	// creationTime is the time the group was created
	creationTime *time.Time

	clusterID  string
	objectUUID string

	credentials *util.Credentials

	// temporary connection attributes
	conn  *util.ClusterConnection
	ioctx *rados.IOContext

	// required details to perform operations on the group
	monitors  string
	pool      string
	namespace string

	// csiDriver is the CSI drivername that is required to connect the journal
	csiDriver string
	// use getJournal() to make sure the journal is connected
	journal journal.VolumeGroupJournal
}

// generateVolumeGroup generates a commonVolumeGroup structure from the volumeGroup identifier.
func (cvg *commonVolumeGroup) generateVolumeGroup(ctx context.Context, csiID util.CSIIdentifier) error {
	mons, err := util.Mons(util.CsiConfigFile, csiID.ClusterID)
	if err != nil {
		return fmt.Errorf("failed to get MONs for cluster id %q: %w", csiID.ClusterID, err)
	}

	namespace, err := util.GetRBDRadosNamespace(util.CsiConfigFile, csiID.ClusterID)
	if err != nil {
		return fmt.Errorf("failed to get RADOS namespace for cluster id %q: %w", csiID.ClusterID, err)
	}

	pool, err := util.GetPoolName(mons, cvg.credentials, csiID.LocationID)
	if err != nil {
		return fmt.Errorf("failed to get pool for volume group id %q: %w", cvg.id, err)
	}

	cvg.monitors = mons
	cvg.namespace = namespace
	cvg.pool = pool

	// Check if the volume group exists in the journal.
	_, err = cvg.getVolumeGroupAttributes(ctx)
	if err != nil {
		return err
	}

	return nil
}

// generateVolumeGroupFromMapping checks the clusterID and poolID mapping and
// generates commonVolumeGroup structure for the mapped clusterID and poolID.
// If the mapping is not found, it returns ErrGroupNotFound.
func (cvg *commonVolumeGroup) generateVolumeGroupFromMapping(
	ctx context.Context,
	csiID util.CSIIdentifier,
	mapping *[]util.ClusterMappingInfo,
) error {
	var volumeGroupGenerationError error

	mcsiID := csiID
	existingClusterID := csiID.ClusterID
	existingPoolID := strconv.FormatInt(csiID.LocationID, 10)

	for _, cm := range *mapping {
		for key, val := range cm.ClusterIDMapping {
			mappedClusterID := util.GetMappedID(key, val, csiID.ClusterID)
			if mappedClusterID == "" {
				continue
			}

			log.DebugLog(ctx,
				"found new clusterID mapping %s for existing clusterID %s", mappedClusterID, existingClusterID)

			// Add mapped clusterID to Identifier
			mcsiID.ClusterID = mappedClusterID
			for _, pools := range cm.RBDpoolIDMappingInfo {
				for key, val := range pools {
					mappedPoolID := util.GetMappedID(key, val, existingPoolID)
					if mappedPoolID == "" {
						continue
					}
					log.DebugLog(ctx,
						"found new poolID mapping %s for existing poolID %s", mappedPoolID, existingPoolID)

					mPID, err := strconv.ParseInt(mappedPoolID, 10, 64)
					if err != nil {
						return err
					}
					mcsiID.LocationID = mPID
					volumeGroupGenerationError = cvg.generateVolumeGroup(ctx, mcsiID)
					if !ShouldRetryVolumeGroupGeneration(volumeGroupGenerationError) {
						return volumeGroupGenerationError
					}

					log.DebugLog(ctx, "volume group not found for poolID mapping %s: %v", cvg.id, mappedPoolID)
				}
			}
		}
	}

	return rbderrors.ErrGroupNotFound
}

func (cvg *commonVolumeGroup) initCommonVolumeGroup(
	ctx context.Context,
	id string,
	csiDriver string,
	creds *util.Credentials,
) error {
	var volumeGroupGenerationError error
	csiID := util.CSIIdentifier{}

	err := csiID.DecomposeCSIID(id)
	if err != nil {
		return fmt.Errorf("failed to decompose volume group id %q: %w", id, err)
	}

	cvg.csiDriver = csiDriver
	cvg.credentials = creds
	cvg.id = id
	cvg.clusterID = csiID.ClusterID
	cvg.objectUUID = csiID.ObjectUUID
	// cvg.monitors, cvg.namespace, cvg.pool are set in generateVolumeGroup

	volumeGroupGenerationError = cvg.generateVolumeGroup(ctx, csiID)
	// If the error is not a retryable error, return from here.
	if volumeGroupGenerationError != nil && !ShouldRetryVolumeGroupGeneration(volumeGroupGenerationError) {
		return volumeGroupGenerationError
	}

	// If Volume Group doesn't exists or the error is a retryable error,
	// we should try to get the cluster mapping and generate the volume group from the mapping.
	if ShouldRetryVolumeGroupGeneration(volumeGroupGenerationError) {
		mapping, err := util.GetClusterMappingInfo(csiID.ClusterID)
		if err != nil {
			return err
		}
		if mapping != nil {
			err = cvg.generateVolumeGroupFromMapping(ctx, csiID, mapping)
			if err != nil {
				return err
			}
		}
	}

	log.DebugLog(ctx, "object for volume group %q has been initialized", cvg.id)

	return nil
}

func (cvg *commonVolumeGroup) Destroy(ctx context.Context) {
	if cvg.ioctx != nil {
		cvg.ioctx.Destroy()
		cvg.ioctx = nil
	}

	if cvg.conn != nil {
		cvg.conn.Destroy()
		cvg.conn = nil
	}

	if cvg.credentials != nil {
		// credentials should only be removed with DeleteCredentials()
		// by the caller that allocated them
		cvg.credentials = nil
	}

	if cvg.journal != nil {
		cvg.journal.Destroy()
		cvg.journal = nil
	}
}

// getVolumeGroupAttributes fetches the attributes from the journal, sets some
// of the common values for the VolumeGroup and returns the attributes struct
// for further consumption (like checking the VolumeMap).
func (cvg *commonVolumeGroup) getVolumeGroupAttributes(ctx context.Context) (*journal.VolumeGroupAttributes, error) {
	j, err := cvg.getJournal(ctx)
	if err != nil {
		return nil, err
	}

	attrs, err := j.GetVolumeGroupAttributes(ctx, cvg.pool, cvg.objectUUID)
	if err != nil {
		if !errors.Is(err, util.ErrKeyNotFound) && !errors.Is(err, util.ErrPoolNotFound) {
			return nil, fmt.Errorf("failed to get attributes for volume group id %q: %w", cvg.id, err)
		}

		attrs = &journal.VolumeGroupAttributes{}
	}

	if attrs.GroupName == "" {
		log.ErrorLog(ctx, "volume group with id %v not found", cvg.id)

		return nil, rbderrors.ErrGroupNotFound
	}

	cvg.requestName = attrs.RequestName
	cvg.name = attrs.GroupName
	cvg.creationTime = attrs.CreationTime

	return attrs, nil
}

// String returns the image-spec (pool/{namespace}/{name}) format of the group.
func (cvg *commonVolumeGroup) String() string {
	if cvg.namespace != "" && cvg.pool != "" && cvg.name != "" {
		return fmt.Sprintf("%s/%s/%s", cvg.pool, cvg.namespace, cvg.name)
	}

	if cvg.name != "" && cvg.pool != "" {
		return fmt.Sprintf("%s/%s", cvg.pool, cvg.name)
	}

	return fmt.Sprintf("<unidentified group %v>", *cvg)
}

// GetID returns the CSI-Addons VolumeGroupId of the VolumeGroup.
func (cvg *commonVolumeGroup) GetID(ctx context.Context) (string, error) {
	if cvg.id == "" {
		return "", errors.New("BUG: ID is not set")
	}

	return cvg.id, nil
}

// GetName returns the name in the backend storage for the VolumeGroup.
func (cvg *commonVolumeGroup) GetName(ctx context.Context) (string, error) {
	if cvg.name == "" {
		return "", errors.New("BUG: name is not set")
	}

	return cvg.name, nil
}

// GetRequestName returns the requestName of the VolumeGroup.
func (cvg *commonVolumeGroup) GetRequestName(ctx context.Context) (string, error) {
	if cvg.requestName == "" {
		return "", errors.New("BUG: requestName is not set")
	}

	return cvg.requestName, nil
}

// GetPool returns the name of the pool that holds the VolumeGroup.
func (cvg *commonVolumeGroup) GetPool(ctx context.Context) (string, error) {
	if cvg.pool == "" {
		return "", errors.New("BUG: pool is not set")
	}

	return cvg.pool, nil
}

// GetClusterID returns the name of the pool that holds the VolumeGroup.
func (cvg *commonVolumeGroup) GetClusterID(ctx context.Context) (string, error) {
	if cvg.clusterID == "" {
		return "", errors.New("BUG: clusterID is not set")
	}

	return cvg.clusterID, nil
}

// getConnection returns the ClusterConnection for the volume group if it
// exists, otherwise it will open a new one.
// Destroy should be used to close the ClusterConnection.
func (cvg *commonVolumeGroup) getConnection(ctx context.Context) (*util.ClusterConnection, error) {
	if cvg.conn != nil {
		return cvg.conn, nil
	}

	if cvg.credentials == nil {
		log.DebugLog(ctx, "missing credentials for common volume group %q: %s", cvg, util.CallStack())

		return nil, errors.New("can not connect to cluster without credentials")
	}

	conn := &util.ClusterConnection{}
	err := conn.Connect(cvg.monitors, cvg.credentials)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MONs %q: %w", cvg.monitors, err)
	}

	cvg.conn = conn
	log.DebugLog(ctx, "connection established for volume group %q", cvg.id)

	return conn, nil
}

func (cvg *commonVolumeGroup) getJournal(ctx context.Context) (journal.VolumeGroupJournal, error) {
	if cvg.journal != nil {
		return cvg.journal, nil
	}

	if cvg.credentials == nil {
		log.DebugLog(ctx, "missing credentials for common volume group %q: %s", cvg, util.CallStack())

		return nil, errors.New("can not connect the journal without credentials")
	}

	journalConfig := journal.NewCSIVolumeGroupJournalWithNamespace(cvg.csiDriver, cvg.namespace)

	j, err := journalConfig.Connect(cvg.monitors, cvg.namespace, cvg.credentials)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to journal: %w", err)
	}

	cvg.journal = j

	return j, nil
}

// GetIOContext returns the IOContext for the volume group if it exists,
// otherwise it will allocate a new one.
// Destroy should be used to free the IOContext.
func (cvg *commonVolumeGroup) GetIOContext(ctx context.Context) (*rados.IOContext, error) {
	if cvg.ioctx != nil {
		return cvg.ioctx, nil
	}

	conn, err := cvg.getConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to connect: %w", rbderrors.ErrGroupNotConnected, err)
	}

	ioctx, err := conn.GetIoctx(cvg.pool)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get IOContext: %w", rbderrors.ErrGroupNotConnected, err)
	}

	if cvg.namespace != "" {
		ioctx.SetNamespace(cvg.namespace)
	}

	cvg.ioctx = ioctx
	log.DebugLog(ctx, "iocontext created for volume group %q in pool %q", cvg.id, cvg.pool)

	return ioctx, nil
}

// Delete removes the volume group from the journal.
func (cvg *commonVolumeGroup) Delete(ctx context.Context) error {
	name, err := cvg.GetName(ctx)
	if err != nil {
		return fmt.Errorf("failed to get name for volume group %q: %w", cvg, err)
	}

	reqName, err := cvg.GetRequestName(ctx)
	if err != nil {
		return fmt.Errorf("failed to get request name for volume group %q: %w", cvg, err)
	}

	pool, err := cvg.GetPool(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pool for volume group %q: %w", cvg, err)
	}

	j, err := cvg.getJournal(ctx)
	if err != nil {
		return err
	}

	err = j.UndoReservation(ctx, pool, name, reqName)
	if err != nil /* TODO? !errors.Is(..., err) */ {
		return fmt.Errorf("failed to undo the reservation for volume group %q: %w", cvg, err)
	}

	return nil
}

// GetCreationTime fetches the creation time of the volume group from the
// journal and returns it.
func (cvg *commonVolumeGroup) GetCreationTime(ctx context.Context) (*time.Time, error) {
	if cvg.creationTime == nil {
		// getVolumeGroupAttributes sets .creationTime (and a few other attributes)
		_, err := cvg.getVolumeGroupAttributes(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get volume attributes for id %q: %w", cvg, err)
		}
	}

	return cvg.creationTime, nil
}

// ShouldRetryVolumeGroupGeneration determines whether the process of finding or generating
// volumegroups should continue based on the type of error encountered.
//
// It checks if the given error matches any of the following known errors:
//   - ErrPoolNotFound: The rbd pool where the volumegroup/omap is expected doesn't exist.
//   - ErrGroupNotFound: The volumegroup doesn't exist in the rbd pool.
//   - rados.ErrPermissionDenied: Permissions to access the pool is denied.
//
// If any of these errors are encountered, the function returns `true`, indicating
// that the volumegroup search should continue because of known error. Otherwise, it
// returns `false`, meaning the search should stop.
//
// This helper function is used in scenarios where multiple attempts may be made
// to retrieve or generate volumegroup information, and we want to gracefully handle
// specific failure cases while retrying for others.
func ShouldRetryVolumeGroupGeneration(err error) bool {
	if err == nil {
		return false // No error, do not retry
	}
	// Continue searching for specific known errors
	return (errors.Is(err, util.ErrPoolNotFound) ||
		errors.Is(err, rbderrors.ErrGroupNotFound) ||
		errors.Is(err, rados.ErrPermissionDenied))
}
