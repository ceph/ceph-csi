/*
Copyright 2019 The Ceph-CSI Authors.

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
package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo" // nolint
	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

// nolint: gocyclo,unparam
func validateCloneFromSnapshot(pvcPath, appPath, snapshotPath, pvcClonePath, appClonePath string, total int, validateRbd, validatecephfs bool, f *framework.Framework) {

	pvc, err := loadPVC(pvcPath)
	if err != nil {
		Fail(err.Error())
	}

	app, err := loadApp(appPath)
	if err != nil {
		Fail(err.Error())
	}
	pvcClone, err := loadPVC(pvcClonePath)
	if err != nil {
		Fail(err.Error())
	}
	appClone, err := loadApp(appClonePath)
	if err != nil {
		Fail(err.Error())
	}

	pvc.Namespace = f.UniqueName
	app.Namespace = f.UniqueName
	pvcClone.Namespace = f.UniqueName
	appClone.Namespace = f.UniqueName
	err = createPVCAndApp("", f, pvc, app)
	if err != nil {
		Fail(err.Error())
	}
	images := []string{}
	if validateRbd {
		// validate created backend rbd images
		images = listRBDImages(f)
		if len(images) != 1 {
			e2elog.Logf("backend image count %d expected image count %d", len(images), 1)
			Fail("validate backend image failed")
		}
	}
	// if validatecephfs {
	// TODO add validation for cephfs
	// }
	snap := getSnapshot(snapshotPath)
	snap.Namespace = f.UniqueName
	snap.Spec.Source.Name = pvc.Name
	snap.Spec.Source.Kind = "PersistentVolumeClaim"
	for i := 0; i < total; i++ {
		snap.Name = fmt.Sprintf("%s%d", f.UniqueName, i)
		err = createSnapshot(&snap, deployTimeout)
		if err != nil {
			Fail(err.Error())
		}
	}
	if validateRbd {
		snapList, snapErr := listSnapshots(f, defaultPool, images[0])
		if snapErr != nil {
			Fail(snapErr.Error())
		}
		// check any stale snapshots present on parent volume
		if len(snapList) != 0 {
			e2elog.Logf("stale snapshot count = %v %d on parent image %s", snapList, len(snapList), images[0])
			Fail("validate backend snapshot failed")
		}

		// total images to be present at backend
		// parentPVC+total snapshots
		// validate created backend rbd images
		images = listRBDImages(f)
		count := 1 + total
		if len(images) != count {
			e2elog.Logf("backend image creation not matching pvc count, image count = %d pvc count %d : image %v", len(images), count, images)
			Fail("validate multiple snapshot failed")
		}
	}
	// if validatecephfs {
	// TODO add validation for cephfs
	// }

	group := "snapshot.storage.k8s.io"
	dataSourceRef := &v1.TypedLocalObjectReference{
		APIGroup: &group,
		Kind:     "VolumeSnapshot",
		Name:     fmt.Sprintf("%s%d", f.UniqueName, 0),
	}
	pvcClone.Spec.DataSource = dataSourceRef
	// create PVC clone
	for i := 0; i < total; i++ {
		name := fmt.Sprintf("%s%d", f.UniqueName, 0)
		err = createPVCAndApp(name, f, pvcClone, appClone)
		if err != nil {
			Fail(err.Error())
		}
	}

	if validateRbd {
		// total images to be present at backend
		// parentPVC+total*snapshot+total*pvc
		// validate created backend rbd images
		images = listRBDImages(f)
		count := 1 + total + total
		if len(images) != count {
			e2elog.Logf("backend image creation not matching pvc count, image count = %v pvc count %d", len(images), count)
			Fail("validate multiple snapshot failed")
		}
	}
	// if validatecephfs {
	// TODO add validation for cephfs
	// }
	image := images[0]
	// delete  all  snapshots
	for i := 0; i < total; i++ {
		snap.Name = fmt.Sprintf("%s%d", f.UniqueName, i)
		err = deleteSnapshot(&snap, deployTimeout)
		if err != nil {
			Fail(err.Error())
		}
	}
	if validateRbd {
		snapList, snapErr := listSnapshots(f, defaultPool, image)
		if snapErr != nil {
			Fail(snapErr.Error())
		}
		if len(snapList) != 0 {
			e2elog.Logf("stale snapshot in backend, count = %v ", len(snapList))
			Fail("validate backend snapshot failed")
		}

		// validate created backend rbd images
		images = listRBDImages(f)
		if len(images) != 1 {
			e2elog.Logf("backend image count %d expected image count %d", len(images), 1)
			Fail("validate backend image failed")
		}
	}
	// if validatecephfs {
	// TODO add validation for cephfs
	// }

	for i := 0; i < total; i++ {
		name := fmt.Sprintf("%s%d", f.UniqueName, i)
		err = deletePVCAndApp(name, f, pvcClone, appClone)
		if err != nil {
			Fail(err.Error())
		}
	}

	err = deletePVCAndApp("", f, pvc, app)
	if err != nil {
		Fail(err.Error())
	}
	if validateRbd {
		// validate created backend rbd images
		images = listRBDImages(f)
		if len(images) != 0 {
			e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
			Fail("validate backend image failed")
		}
	}
	// if validatecephfs {
	// TODO add validation for cephfs
	// }
}
