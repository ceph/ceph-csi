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

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"
)

// rbdSnapshotLimits reads overrides from the running driver's deployment. The
// defaults match cmd/cephcsi.go; Helm tests normally set much smaller limits.
func rbdSnapshotLimits(f *framework.Framework) (int, int, error) {
	soft, hard := 250, 450
	d, err := f.ClientSet.AppsV1().Deployments(cephCSINamespace).Get(
		context.TODO(), rbdDeployment.getDeploymentName(), metav1.GetOptions{})
	if err != nil {
		return 0, 0, err
	}
	for _, container := range d.Spec.Template.Spec.Containers {
		if container.Name != rbdContainerName {
			continue
		}
		args := append(append([]string{}, container.Command...), container.Args...)
		for i, arg := range args {
			name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if name != "minsnapshotsonimage" && name != "maxsnapshotsonimage" {
				continue
			}
			if !hasValue && i+1 < len(args) {
				value = args[i+1]
			}
			limit, err := strconv.Atoi(value)
			if err != nil {
				return 0, 0, fmt.Errorf("invalid %s: %w", name, err)
			}
			if name == "minsnapshotsonimage" {
				soft = limit
			} else {
				hard = limit
			}
		}

		return soft, hard, nil
	}

	return 0, 0, fmt.Errorf("container %s not found in deployment %s", rbdContainerName, d.Name)
}

// validateChildFlattenLocation constructs a child directly in Ceph to isolate
// flattening from cross-StorageClass provisioning. Creating a CSI snapshot of
// the source must flatten that child using its own pool and namespace.
func validateChildFlattenLocation(f *framework.Framework, differentPool, differentNamespace bool) {
	soft, hard, err := rbdSnapshotLimits(f)
	Expect(err).NotTo(HaveOccurred())
	if soft >= hard {
		Skip("child flattening regression requires minsnapshotsonimage < maxsnapshotsonimage")
	}

	run := func(command string) string {
		stdout, stderr, err := execCommandInToolBoxPod(f, command, rookNamespace)
		Expect(err).NotTo(HaveOccurred(), "command: %s, stderr: %s", command, stderr)

		return stdout
	}
	pool := defaultRBDPool
	if differentPool {
		pool = f.UniqueName + "-flatten"
		Expect(createPool(f, pool)).To(Succeed())
		defer func() { Expect(deletePool(pool, false, f)).To(Succeed()) }()
	}
	namespace := radosNamespace
	if differentNamespace {
		namespace = f.UniqueName + "-flatten"
	}
	if namespace != "" && (differentPool || differentNamespace) {
		options := fmt.Sprintf("--pool %s --namespace %s", pool, namespace)
		run("rbd namespace create " + options)
		defer func() { run("rbd namespace remove " + options) }()
	}

	Expect(createRBDSnapshotClass(f)).To(Succeed())
	defer func() { Expect(deleteRBDSnapshotClass()).To(Succeed()) }()
	pvc, err := loadPVC(pvcPath)
	Expect(err).NotTo(HaveOccurred())
	pvc.Namespace = f.UniqueName
	Expect(createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)).To(Succeed())
	defer func() { Expect(deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)).To(Succeed()) }()
	info, err := getImageInfoFromPVC(pvc.Namespace, pvc.Name, f)
	Expect(err).NotTo(HaveOccurred())
	source := imageSpec(defaultRBDPool, info.imageName)

	// Only one real clone is needed. Empty snapshots bring the source just
	// above the soft limit without creating hundreds of PVCs.
	defer func() {
		run(fmt.Sprintf(
			"for i in $(seq 0 %d); do rbd snap rm %s@flatten-$i || exit 1; done", soft, source))
	}()
	run(fmt.Sprintf(
		"for i in $(seq 0 %d); do rbd snap create %s@flatten-$i || exit 1; done", soft, source))
	child := fmt.Sprintf("%s/%s-temp", pool, info.imageName)
	if namespace != "" {
		child = fmt.Sprintf("%s/%s/%s-temp", pool, namespace, info.imageName)
	}
	run(fmt.Sprintf("rbd clone --rbd-default-clone-format=2 %s@flatten-0 %s", source, child))
	defer func() { run("rbd rm " + child) }()

	hasParent := func() (bool, error) {
		stdout, stderr, err := execCommandInToolBoxPod(f, "rbd info --format=json "+child, rookNamespace)
		if err != nil {
			return false, fmt.Errorf("failed to inspect %s: %w: %s", child, err, stderr)
		}
		var image struct {
			Parent *json.RawMessage `json:"parent"`
		}
		if err = json.Unmarshal([]byte(stdout), &image); err != nil {
			return false, err
		}

		return image.Parent != nil, nil
	}
	parent, err := hasParent()
	Expect(err).NotTo(HaveOccurred())
	Expect(parent).To(BeTrue(), "child must have a parent before the CSI request")

	snap := getSnapshot(snapshotPath)
	snap.Namespace = f.UniqueName
	snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
	defer func() { Expect(deleteSnapshot(&snap, deployTimeout)).To(Succeed()) }()
	Expect(createSnapshot(&snap, deployTimeout)).To(Succeed())

	err = wait.PollUntilContextTimeout(context.TODO(), poll, time.Duration(deployTimeout)*time.Minute,
		true, func(_ context.Context) (bool, error) {
			parent, err := hasParent()

			return !parent, err
		})
	Expect(err).NotTo(HaveOccurred(), "child %s must lose its parent after flattening", child)
}
