/*
Copyright 2026 The Ceph-CSI Authors.

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
package persistentvolume

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/ceph/ceph-csi/internal/util/log"
)

// storageClassSupportsDiffIterate gates feature changes on the current
// StorageClass, rather than the possibly stale imageFeatures copied to the PV.
func (r *ReconcilePersistentVolume) storageClassSupportsDiffIterate(
	ctx context.Context, pv *corev1.PersistentVolume,
) (bool, error) {
	if pv.Spec.StorageClassName == "" {
		log.WarningLog(ctx, "skipping DiffIterate repair for PV %s: no StorageClass", pv.Name)

		return false, nil
	}
	sc := &storagev1.StorageClass{}
	err := r.client.Get(ctx, types.NamespacedName{Name: pv.Spec.StorageClassName}, sc)
	if apierrors.IsNotFound(err) {
		log.WarningLog(ctx, "skipping DiffIterate repair for PV %s: StorageClass %s not found",
			pv.Name, pv.Spec.StorageClassName)

		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get StorageClass %s: %w", pv.Spec.StorageClassName, err)
	}
	if sc.Provisioner != pv.Spec.CSI.Driver {
		log.WarningLog(ctx, "skipping DiffIterate repair for PV %s: StorageClass %s provisioner does not match",
			pv.Name, sc.Name)

		return false, nil
	}
	features := make(map[string]bool)
	for feature := range strings.SplitSeq(sc.Parameters["imageFeatures"], ",") {
		features[strings.TrimSpace(feature)] = true
	}
	for _, feature := range []string{"exclusive-lock", "object-map", "fast-diff"} {
		if !features[feature] {
			log.WarningLog(ctx,
				"skipping DiffIterate repair for PV %s: recreate StorageClass %s with exclusive-lock, object-map and fast-diff",
				pv.Name, sc.Name)

			return false, nil
		}
	}

	return true, nil
}
