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

package rbd

import (
	"context"
	"errors"
	"strconv"

	librbd "github.com/ceph/go-ceph/rbd"

	"github.com/ceph/ceph-csi/internal/util/log"
)

const (
	// Qos parameters name of StorageClass.
	baseIops         = "baseIops"
	maxIops          = "maxIops"
	baseReadIops     = "baseReadIops"
	maxReadIops      = "maxReadIops"
	baseWriteIops    = "baseWriteIops"
	maxWriteIops     = "maxWriteIops"
	baseBps          = "baseBps"
	maxBps           = "maxBps"
	baseReadBps      = "baseReadBps"
	maxReadBps       = "maxReadBps"
	baseWriteBps     = "baseWriteBps"
	maxWriteBps      = "maxWriteBps"
	iopsPerGiB       = "iopsPerGiB"
	readIopsPerGiB   = "readIopsPerGiB"
	writeIopsPerGiB  = "writeIopsPerGiB"
	bpsPerGiB        = "bpsPerGiB"
	readBpsPerGiB    = "readBpsPerGiB"
	writeBpsPerGiB   = "writeBpsPerGiB"
	baseVolSizeBytes = "baseVolSizeBytes"

	// Qos type name of rbd image.
	iopsLimit          = "rbd_qos_iops_limit"
	readIopsLimit      = "rbd_qos_read_iops_limit"
	writeIopsLimit     = "rbd_qos_write_iops_limit"
	bpsLimit           = "rbd_qos_bps_limit"
	readBpsLimit       = "rbd_qos_read_bps_limit"
	writeBpsLimit      = "rbd_qos_write_bps_limit"
	metadataConfPrefix = "conf_"

	// The params use to calc qos based on capacity.
	baseQosIopsLimit      = "rbd_base_qos_iops_limit"
	maxQosIopsLimit       = "rbd_max_qos_iops_limit"
	baseQosReadIopsLimit  = "rbd_base_qos_read_iops_limit"
	maxQosReadIopsLimit   = "rbd_max_qos_read_iops_limit"
	baseQosWriteIopsLimit = "rbd_base_qos_write_iops_limit"
	maxQosWriteIopsLimit  = "rbd_max_qos_write_iops_limit"
	baseQosBpsLimit       = "rbd_base_qos_bps_limit"
	maxQosBpsLimit        = "rbd_max_qos_bps_limit"
	baseQosReadBpsLimit   = "rbd_base_qos_read_bps_limit"
	maxQosReadBpsLimit    = "rbd_max_qos_read_bps_limit"
	baseQosWriteBpsLimit  = "rbd_base_qos_write_bps_limit"
	maxQosWriteBpsLimit   = "rbd_max_qos_write_bps_limit"
	iopsPerGiBLimit       = "rbd_iops_per_gib_limit"
	readIopsPerGiBLimit   = "rbd_read_iops_per_gib_limit"
	writeIopsPerGiBLimit  = "rbd_write_iops_per_gib_limit"
	bpsPerGiBLimit        = "rbd_bps_per_gib_limit"
	readBpsPerGiBLimit    = "rbd_read_bps_per_gib_limit"
	writeBpsPerGiBLimit   = "rbd_write_bps_per_gib_limit"
	baseQosVolSize        = "rbd_base_qos_vol_size"
)

type qosSpec struct {
	baseLimitType   string
	baseLimit       string
	perGiBLimitType string
	perGiBLimit     string
	maxLimitType    string
	maxLimit        string
	present         bool
}

// HasQoSParams checks if any RBD QoS parameters are present.
func HasQoSParams(params map[string]string) bool {
	rbdQosParams := parseQosParams(params)
	for _, qos := range rbdQosParams {
		if qos.present {
			return true
		}
	}

	return false
}

func parseQosParams(
	scParams map[string]string,
) map[string]*qosSpec {
	rbdQosParameters := map[string]*qosSpec{
		baseIops:      {iopsLimit, "", iopsPerGiB, "", maxIops, "", false},
		baseReadIops:  {readIopsLimit, "", readIopsPerGiB, "", maxReadIops, "", false},
		baseWriteIops: {writeIopsLimit, "", writeIopsPerGiB, "", maxWriteIops, "", false},
		baseBps:       {bpsLimit, "", bpsPerGiB, "", maxBps, "", false},
		baseReadBps:   {readBpsLimit, "", readBpsPerGiB, "", maxReadBps, "", false},
		baseWriteBps:  {writeBpsLimit, "", writeBpsPerGiB, "", maxWriteBps, "", false},
	}
	for k, v := range scParams {
		if qos, ok := rbdQosParameters[k]; ok && v != "" {
			qos.baseLimit = v
			qos.present = true
			if perGiBLimit, ok := scParams[qos.perGiBLimitType]; ok && perGiBLimit != "" {
				qos.perGiBLimit = perGiBLimit
			}
			if maxLimit, ok := scParams[qos.maxLimitType]; ok && maxLimit != "" {
				qos.maxLimit = maxLimit
			}
		}
	}

	return rbdQosParameters
}

func (rv *rbdVolume) SetQOS(
	ctx context.Context,
	scParams map[string]string,
) error {
	rv.BaseVolSize = ""
	if v, ok := scParams[baseVolSizeBytes]; ok && v != "" {
		rv.BaseVolSize = v
	}

	rbdQosParameters := parseQosParams(scParams)
	for _, qos := range rbdQosParameters {
		if qos.present {
			err := rv.calcQosBasedOnCapacity(ctx, qos)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (rv *rbdVolume) ApplyQOS(
	ctx context.Context,
) error {
	for k, v := range rv.QosParameters {
		err := rv.SetMetadata(metadataConfPrefix+k, v)
		if err != nil {
			log.ErrorLog(ctx, "failed to set rbd qos, %s: %s. %v", k, v, err)

			return err
		}
	}

	return nil
}

func (rv *rbdVolume) calcQosBasedOnCapacity(
	ctx context.Context,
	qos *qosSpec,
) error {
	if rv.QosParameters == nil {
		rv.QosParameters = make(map[string]string)
	}

	// Don't set qos if base qos limit empty.
	if qos.baseLimit == "" {
		return nil
	}
	baseLimit, err := strconv.ParseInt(qos.baseLimit, 10, 64)
	if err != nil {
		log.ErrorLog(ctx, "failed to parse %s: %s. %v", qos.baseLimitType, qos.baseLimit, err)

		return err
	}

	// if present qosPerGB and baseVolSize, we will set qos based on capacity,
	// otherwise, we only set base qos limit.
	if qos.perGiBLimit == "" || rv.BaseVolSize == "" {
		rv.QosParameters[qos.baseLimitType] = qos.baseLimit

		return nil
	}

	perGiBLimit, err := strconv.ParseInt(qos.perGiBLimit, 10, 64)
	if err != nil {
		log.ErrorLog(ctx, "failed to parse %s: %s. %v", qos.perGiBLimitType, qos.perGiBLimit, err)

		return err
	}

	baseVolSize, err := strconv.ParseInt(rv.BaseVolSize, 10, 64)
	if err != nil {
		log.ErrorLog(ctx, "failed to parse %s: %s. %v", baseVolSizeBytes, rv.BaseVolSize, err)

		return err
	}

	if rv.RequestedVolSize <= baseVolSize {
		rv.QosParameters[qos.baseLimitType] = qos.baseLimit

		return nil
	}

	capacityQos := (rv.RequestedVolSize - baseVolSize) / int64(oneGB) * perGiBLimit
	finalQosLimit := baseLimit + capacityQos
	if qos.maxLimit != "" {
		maxLimit, err := strconv.ParseInt(qos.maxLimit, 10, 64)
		if err != nil {
			log.ErrorLog(ctx, "failed to parse %s: %s. %v", qos.maxLimitType, qos.maxLimit, err)

			return err
		}
		if finalQosLimit > maxLimit {
			finalQosLimit = maxLimit
		}
	}
	rv.QosParameters[qos.baseLimitType] = strconv.FormatInt(finalQosLimit, 10)

	return nil
}

func (rv *rbdVolume) SaveQOS(
	ctx context.Context,
	scParams map[string]string,
) error {
	needSaveQosParameters := map[string]string{
		baseIops:         baseQosIopsLimit,
		maxIops:          maxQosIopsLimit,
		baseReadIops:     baseQosReadIopsLimit,
		maxReadIops:      maxQosReadIopsLimit,
		baseWriteIops:    baseQosWriteIopsLimit,
		maxWriteIops:     maxQosWriteIopsLimit,
		baseBps:          baseQosBpsLimit,
		maxBps:           maxQosBpsLimit,
		baseReadBps:      baseQosReadBpsLimit,
		maxReadBps:       maxQosReadBpsLimit,
		baseWriteBps:     baseQosWriteBpsLimit,
		maxWriteBps:      maxQosWriteBpsLimit,
		iopsPerGiB:       iopsPerGiBLimit,
		readIopsPerGiB:   readIopsPerGiBLimit,
		writeIopsPerGiB:  writeIopsPerGiBLimit,
		bpsPerGiB:        bpsPerGiBLimit,
		readBpsPerGiB:    readBpsPerGiBLimit,
		writeBpsPerGiB:   writeBpsPerGiBLimit,
		baseVolSizeBytes: baseQosVolSize,
	}
	for k, v := range scParams {
		if param, ok := needSaveQosParameters[k]; ok && v != "" {
			err := rv.SetMetadata(param, v)
			if err != nil {
				log.ErrorLog(ctx, "failed to save qos. %s: %s, %v", k, v, err)

				return err
			}
		}
	}

	return nil
}

func (rv *rbdVolume) getRbdImageQOS(
	ctx context.Context,
) (map[string]qosSpec, error) {
	QosParams := map[string]struct {
		rbdQosType       string
		rbdQosPerGiBType string
		rbdQosMaxType    string
	}{
		baseQosIopsLimit:      {iopsLimit, iopsPerGiBLimit, maxQosIopsLimit},
		baseQosReadIopsLimit:  {readIopsLimit, readIopsPerGiBLimit, maxQosReadIopsLimit},
		baseQosWriteIopsLimit: {writeIopsLimit, writeIopsPerGiBLimit, maxQosWriteIopsLimit},
		baseQosBpsLimit:       {bpsLimit, bpsPerGiBLimit, maxQosBpsLimit},
		baseQosReadBpsLimit:   {readBpsLimit, readBpsPerGiBLimit, maxQosReadBpsLimit},
		baseQosWriteBpsLimit:  {writeBpsLimit, writeBpsPerGiBLimit, maxQosWriteBpsLimit},
	}
	rbdQosParameters := make(map[string]qosSpec)
	for k, param := range QosParams {
		baseLimit, err := rv.GetMetadata(k)
		if err != nil && !errors.Is(err, librbd.ErrNotFound) {
			log.ErrorLog(ctx, "failed to get metadata: %s. %v", k, err)

			return nil, err
		}
		if baseLimit == "" {
			// if base qos dose not exist, skipping.
			continue
		}
		perGiBLimit, err := rv.GetMetadata(param.rbdQosPerGiBType)
		if err != nil && !errors.Is(err, librbd.ErrNotFound) {
			log.ErrorLog(ctx, "failed to get metadata: %s. %v", param.rbdQosPerGiBType, err)

			return nil, err
		}
		maxLimit, err := rv.GetMetadata(param.rbdQosMaxType)
		if err != nil && !errors.Is(err, librbd.ErrNotFound) {
			log.ErrorLog(ctx, "failed to get metadata: %s. %v", param.rbdQosMaxType, err)

			return nil, err
		}
		rbdQosParameters[k] = qosSpec{
			param.rbdQosType,
			baseLimit,
			param.rbdQosPerGiBType,
			perGiBLimit,
			param.rbdQosMaxType,
			maxLimit, true,
		}
	}

	baseVolSize, err := rv.GetMetadata(baseQosVolSize)
	if err != nil && !errors.Is(err, librbd.ErrNotFound) {
		log.ErrorLog(ctx, "failed to get metadata: %s. %v", baseQosVolSize, err)

		return nil, err
	}
	rv.BaseVolSize = baseVolSize

	return rbdQosParameters, nil
}

func (rv *rbdVolume) AdjustQOS(
	ctx context.Context,
) error {
	rbdQosParameters, err := rv.getRbdImageQOS(ctx)
	if err != nil {
		return err
	}
	for _, param := range rbdQosParameters {
		err = rv.calcQosBasedOnCapacity(ctx, &param)
		if err != nil {
			return err
		}
	}
	err = rv.ApplyQOS(ctx)
	if err != nil {
		return err
	}

	return nil
}
