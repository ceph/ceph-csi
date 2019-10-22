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
func validateCloneFromPVC(pvcPath, appPath, pvcClonePath string, total int, validateRbd, validatecephfs bool, f *framework.Framework) {

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

	pvc.Namespace = f.UniqueName
	app.Namespace = f.UniqueName
	pvcClone.Namespace = f.UniqueName
	err = createPVCAndApp("", f, pvc, app)
	if err != nil {
		Fail(err.Error())
	}

	if validateRbd {
		// validate created backend rbd images
		images := listRBDImages(f)
		if len(images) != 1 {
			e2elog.Logf("backend image count %d expected image count %d", len(images), 1)
			Fail("validate backend image failed")
		}
	}
	// if validatecephfs {
	// TODO add validation for cephfs
	// }

	dataSourceRef := &v1.TypedLocalObjectReference{
		Kind: "PersistentVolumeClaim",
		Name: pvc.Name,
	}
	pvcClone.Spec.DataSource = dataSourceRef
	// create PVC clone
	for i := 0; i < total; i++ {
		name := fmt.Sprintf("%s-%d", f.UniqueName, i)
		err = createPVCAndApp(name, f, pvcClone, app)
		if err != nil {
			Fail(err.Error())
		}
	}

	if validateRbd {
		// total images to be present at backend
		// parentPVC+clonedPVC
		// validate created backend rbd images
		images := listRBDImages(f)
		count := 1 + total
		if len(images) != count {
			e2elog.Logf("backend image creation not matching pvc count, image count = %v pvc count %d", len(images), count)
			Fail("validate multiple clone PVC failed")
		}
	}
	// if validatecephfs {
	// TODO add validation for cephfs
	// }

	for i := 0; i < total; i++ {
		name := fmt.Sprintf("%s-%d", f.UniqueName, i)
		err = deletePVCAndApp(name, f, pvcClone, app)
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
		images := listRBDImages(f)
		if len(images) != 0 {
			e2elog.Logf("backend image count %d expected image count %d", len(images), 0)
			Fail("validate backend image failed")
		}
	}
	// if validatecephfs {
	// TODO add validation for cephfs
	// }
}
