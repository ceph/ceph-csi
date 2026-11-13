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
	"fmt"
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

// nbdQoSHandler implements QoSHandler for traditional NBD QoS (rbd-nbd mounter).
type nbdQoSHandler struct {
	volume *rbdVolume
}

// newNBDQoSHandler creates a new NBD QoS handler.
func newNBDQoSHandler(volume *rbdVolume) QoSHandler {
	return &nbdQoSHandler{volume: volume}
}

// nbdQoSKnownKeys lists all parameter keys recognized by the NBD QoS handler.
var nbdQoSKnownKeys = []string{
	baseIops, maxIops, baseReadIops, maxReadIops, baseWriteIops, maxWriteIops,
	baseBps, maxBps, baseReadBps, maxReadBps, baseWriteBps, maxWriteBps,
	iopsPerGiB, readIopsPerGiB, writeIopsPerGiB,
	bpsPerGiB, readBpsPerGiB, writeBpsPerGiB,
	baseVolSizeBytes,
}

// HasParams checks if traditional NBD QoS parameters are present in the request.
func (h *nbdQoSHandler) HasParams(params map[string]string) bool {
	return HasQoSParams(params)
}

// Validate validates traditional NBD QoS parameters.
func (h *nbdQoSHandler) Validate(params map[string]string) error {
	return validateNBDQoSParams(params)
}

// validateNBDQoSParams validates traditional NBD QoS parameters.
// Ensures all numeric values are valid and positive.
func validateNBDQoSParams(params map[string]string) error {
	for _, key := range nbdQoSKnownKeys {
		if val, ok := params[key]; ok && val != "" {
			parsed, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %s, must be a valid integer", key, val)
			}
			// Allow zero for base limits (can be omitted), but not negative
			if parsed < 0 {
				return fmt.Errorf("invalid value for %s: %s, must be non-negative", key, val)
			}
		}
	}

	return nil
}

// Apply sets and applies traditional NBD QoS to the rbd-nbd device.
func (h *nbdQoSHandler) Apply(ctx context.Context, params map[string]string) error {
	if err := h.volume.SetQOS(ctx, params); err != nil {
		return err
	}

	if err := h.volume.ApplyQOS(ctx); err != nil {
		return err
	}

	return h.volume.SaveQOS(ctx, params)
}

// nbdQoSMetadataKeys returns every image-metadata key used for traditional NBD
// QoS: the rbd config overrides (conf_rbd_qos_*) set by ApplyQOS and the
// capacity-based QoS metadata (rbd_base_qos_*, rbd_max_qos_*, rbd_*_per_gib_limit,
// rbd_base_qos_vol_size) saved by SaveQOS. Clearing these keys drops both the
// applied limits and the inputs used to recalculate them on expansion.
func nbdQoSMetadataKeys() []string {
	return []string{
		// rbd config overrides set by ApplyQOS.
		metadataConfPrefix + iopsLimit,
		metadataConfPrefix + readIopsLimit,
		metadataConfPrefix + writeIopsLimit,
		metadataConfPrefix + bpsLimit,
		metadataConfPrefix + readBpsLimit,
		metadataConfPrefix + writeBpsLimit,
		// capacity-based QoS metadata saved by SaveQOS.
		baseQosIopsLimit, maxQosIopsLimit,
		baseQosReadIopsLimit, maxQosReadIopsLimit,
		baseQosWriteIopsLimit, maxQosWriteIopsLimit,
		baseQosBpsLimit, maxQosBpsLimit,
		baseQosReadBpsLimit, maxQosReadBpsLimit,
		baseQosWriteBpsLimit, maxQosWriteBpsLimit,
		iopsPerGiBLimit, readIopsPerGiBLimit, writeIopsPerGiBLimit,
		bpsPerGiBLimit, readBpsPerGiBLimit, writeBpsPerGiBLimit,
		baseQosVolSize,
	}
}

// removeQOS removes every traditional NBD QoS key from the image: the rbd config
// overrides (conf_rbd_qos_*) set by ApplyQOS and the capacity-based QoS metadata
// (rbd_base_qos_*, rbd_max_qos_*, rbd_*_per_gib_limit, rbd_base_qos_vol_size)
// saved by SaveQOS.
func (rv *rbdVolume) removeQOS(ctx context.Context) error {
	image, err := rv.open()
	if err != nil {
		return err
	}
	defer image.Close() //nolint:errcheck // not a critical failure

	for _, k := range nbdQoSMetadataKeys() {
		err := image.RemoveMetadata(k)
		if err != nil && !errors.Is(err, librbd.ErrNotExist) {
			log.ErrorLog(ctx, "failed to remove QoS metadata %s: %v", k, err)

			return fmt.Errorf("failed to remove QoS metadata %s: %w", k, err)
		}
	}

	return nil
}

func parseQosParams(
	mutableParameters map[string]string,
) map[string]*qosSpec {
	rbdQosParameters := map[string]*qosSpec{
		baseIops:      {iopsLimit, "", iopsPerGiB, "", maxIops, "", false},
		baseReadIops:  {readIopsLimit, "", readIopsPerGiB, "", maxReadIops, "", false},
		baseWriteIops: {writeIopsLimit, "", writeIopsPerGiB, "", maxWriteIops, "", false},
		baseBps:       {bpsLimit, "", bpsPerGiB, "", maxBps, "", false},
		baseReadBps:   {readBpsLimit, "", readBpsPerGiB, "", maxReadBps, "", false},
		baseWriteBps:  {writeBpsLimit, "", writeBpsPerGiB, "", maxWriteBps, "", false},
	}
	for k, v := range mutableParameters {
		if qos, ok := rbdQosParameters[k]; ok && v != "" {
			qos.baseLimit = v
			qos.present = true
			if perGiBLimit, ok := mutableParameters[qos.perGiBLimitType]; ok && perGiBLimit != "" {
				qos.perGiBLimit = perGiBLimit
			}
			if maxLimit, ok := mutableParameters[qos.maxLimitType]; ok && maxLimit != "" {
				qos.maxLimit = maxLimit
			}
		}
	}

	return rbdQosParameters
}

func (rv *rbdVolume) SetQOS(
	ctx context.Context,
	mutableParameters map[string]string,
) error {
	rv.BaseVolSize = ""
	if v, ok := mutableParameters[baseVolSizeBytes]; ok && v != "" {
		rv.BaseVolSize = v
	}

	rbdQosParameters := parseQosParams(mutableParameters)
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
	mutableParameters map[string]string,
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
	for k, v := range mutableParameters {
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
