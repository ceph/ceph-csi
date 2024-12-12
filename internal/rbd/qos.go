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

	"github.com/ceph/ceph-csi/internal/util/log"

	librbd "github.com/ceph/go-ceph/rbd"
)

const (
	// Qos parameters name of StorageClass.
	baseReadIops            = "BaseReadIops"
	baseWriteIops           = "BaseWriteIops"
	baseReadBytesPerSecond  = "BaseReadBytesPerSecond"
	baseWriteBytesPerSecond = "BaseWriteBytesPerSecond"
	readIopsPerGiB          = "ReadIopsPerGiB"
	writeIopsPerGiB         = "WriteIopsPerGiB"
	readBpsPerGiB           = "ReadBpsPerGiB"
	writeBpsPerGiB          = "WriteBpsPerGiB"
	baseVolSizeBytes        = "BaseVolSizeBytes"

	// Qos type name of rbd image.
	readIopsLimit      = "rbd_qos_read_iops_limit"
	writeIopsLimit     = "rbd_qos_write_iops_limit"
	readBpsLimit       = "rbd_qos_read_bps_limit"
	writeBpsLimit      = "rbd_qos_write_bps_limit"
	metadataConfPrefix = "conf_"

	// The params use to calc qos based on capacity.
	baseQosReadIopsLimit  = "rbd_base_qos_read_iops_limit"
	baseQosWriteIopsLimit = "rbd_base_qos_write_iops_limit"
	baseQosReadBpsLimit   = "rbd_base_qos_read_bps_limit"
	baseQosWriteBpsLimit  = "rbd_base_qos_write_bps_limit"
	readIopsPerGiBLimit   = "rbd_read_iops_per_gib_limit"
	writeIopsPerGiBLimit  = "rbd_write_iops_per_gib_limit"
	readBpsPerGiBLimit    = "rbd_read_bps_per_gib_limit"
	writeBpsPerGiBLimit   = "rbd_write_bps_per_gib_limit"
	baseQosVolSize        = "rbd_base_qos_vol_size"
)

type qosSpec struct {
	baseLimitType   string
	baseLimit       string
	perGiBLimitType string
	perGiBLimit     string
	present         bool
}

func parseQosParams(
	scParams map[string]string,
) map[string]*qosSpec {
	rbdQosParameters := map[string]*qosSpec{
		baseReadIops:            {readIopsLimit, "", readIopsPerGiB, "", false},
		baseWriteIops:           {writeIopsLimit, "", writeIopsPerGiB, "", false},
		baseReadBytesPerSecond:  {readBpsLimit, "", readBpsPerGiB, "", false},
		baseWriteBytesPerSecond: {writeBpsLimit, "", writeBpsPerGiB, "", false},
	}
	for k, v := range scParams {
		if qos, ok := rbdQosParameters[k]; ok && v != "" {
			qos.baseLimit = v
			qos.present = true
			if perGiBLimit, ok := scParams[qos.perGiBLimitType]; ok && perGiBLimit != "" {
				qos.perGiBLimit = perGiBLimit
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
			err := rv.calcQosBasedOnCapacity(ctx, *qos)
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
	qos qosSpec,
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
	if qos.perGiBLimit != "" && rv.BaseVolSize != "" {
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
		} else {
			capacityQos := (rv.RequestedVolSize - baseVolSize) / int64(oneGB) * perGiBLimit
			finalQosLimit := baseLimit + capacityQos
			rv.QosParameters[qos.baseLimitType] = strconv.FormatInt(finalQosLimit, 10)
		}
	} else {
		rv.QosParameters[qos.baseLimitType] = qos.baseLimit
	}

	return nil
}

func (rv *rbdVolume) SaveQOS(
	ctx context.Context,
	scParams map[string]string,
) error {
	needSaveQosParameters := map[string]string{
		baseReadIops:            baseQosReadIopsLimit,
		baseWriteIops:           baseQosWriteIopsLimit,
		baseReadBytesPerSecond:  baseQosReadBpsLimit,
		baseWriteBytesPerSecond: baseQosWriteBpsLimit,
		readIopsPerGiB:          readIopsPerGiBLimit,
		writeIopsPerGiB:         writeIopsPerGiBLimit,
		readBpsPerGiB:           readBpsPerGiBLimit,
		writeBpsPerGiB:          writeBpsPerGiBLimit,
		baseVolSizeBytes:        baseQosVolSize,
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
	}{
		baseQosReadIopsLimit:  {readIopsLimit, readIopsPerGiBLimit},
		baseQosWriteIopsLimit: {writeIopsLimit, writeIopsPerGiBLimit},
		baseQosReadBpsLimit:   {readBpsLimit, readBpsPerGiBLimit},
		baseQosWriteBpsLimit:  {writeBpsLimit, writeBpsPerGiBLimit},
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
		rbdQosParameters[k] = qosSpec{param.rbdQosType, baseLimit, param.rbdQosPerGiBType, perGiBLimit, true}
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
		err = rv.calcQosBasedOnCapacity(ctx, param)
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
