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
	"maps"
	"slices"
	"time"

	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	smsBlockSize   = 4 // MB
	smsBlockBytes  = smsBlockSize * 1024 * 1024
	smsDevicePath  = "/dev/xvda"
	smsMaxResults  = 256
	smsVolCapacity = 1073741824 // 1Gi
)

var _ = Describe("RBD", func() {
	f := framework.NewDefaultFramework("snapshot-metadata")
	f.NamespacePodSecurityEnforceLevel = "privileged"

	Context("[SnapshotMetadata]", Ordered, func() {
		var (
			clientSet     kubernetes.Interface
			dynClient     dynamic.Interface
			infra         *smsInfra
			conn          *grpc.ClientConn
			smsClient     api.SnapshotMetadataClient
			token         string
			stopPF        func()
			testNamespace string
			vgsHelper     *rbdVolumeGroupSnapshot
			// vgsClassName is only set when the VolumeGroupSnapshotClass
			// was created by this suite.
			vgsClassName string
			// vgsSkipReason is set when the VolumeGroupSnapshot tests can
			// not run, these tests are skipped then.
			vgsSkipReason string
		)

		BeforeAll(func() {
			if !testRBD || !operatorDeployment {
				Skip("snapshot metadata tests require --test-rbd and --operator-deployment")
			}

			clientSet = f.ClientSet
			var err error
			dynClient, err = dynamic.NewForConfig(f.ClientConfig())
			Expect(err).ShouldNot(HaveOccurred())

			testNamespace = "sms-e2e-test"
			err = createNamespace(clientSet, testNamespace)
			Expect(err).ShouldNot(HaveOccurred(), "failed to create test namespace")

			// RBD prerequisites: node labels, configmap, secrets
			err = addLabelsToNodes(f, map[string]string{
				nodeRegionLabel:          regionValue,
				nodeZoneLabel:            zoneValue,
				crushLocationRegionLabel: crushLocationRegionValue,
				crushLocationZoneLabel:   crushLocationZoneValue,
			})
			Expect(err).ShouldNot(HaveOccurred(), "failed to add node labels")

			// Best-effort restart of node plugin so it re-registers CSINode topology
			if restartErr := retryKubectlArgs(cephCSINamespace, kubectlPatch,
				deployTimeout, "daemonsets", operatorRBDDaemonsetName,
				"-p", `{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"`+time.Now().Format(time.RFC3339)+`"}}}}}`); restartErr != nil {
				framework.Logf("Warning: failed to restart node plugin (may not exist yet): %v", restartErr)
			} else {
				err = waitForDaemonSets(operatorRBDDaemonsetName, cephCSINamespace, f.ClientSet, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred(), "node plugin rollout failed")
			}

			err = createConfigMap(rbdDirPath, clientSet, f)
			Expect(err).ShouldNot(HaveOccurred(), "failed to create configmap")

			key, err := createCephUser(f, keyringRBDProvisionerUsername, rbdProvisionerCaps("", ""))
			Expect(err).ShouldNot(HaveOccurred(), "failed to create provisioner user")
			err = createRBDSecret(f, rbdProvisionerSecretName, keyringRBDProvisionerUsername, key)
			Expect(err).ShouldNot(HaveOccurred(), "failed to create provisioner secret")

			key, err = createCephUser(f, keyringRBDNodePluginUsername, rbdNodePluginCaps("", ""))
			Expect(err).ShouldNot(HaveOccurred(), "failed to create node plugin user")
			err = createRBDSecret(f, rbdNodePluginSecretName, keyringRBDNodePluginUsername, key)
			Expect(err).ShouldNot(HaveOccurred(), "failed to create node plugin secret")

			// SMS infra: patches OperatorConfig → triggers ctrlplugin restart
			infra, err = setupSnapshotMetadataInfra(clientSet, dynClient, cephCSINamespace, testNamespace)
			Expect(err).ShouldNot(HaveOccurred(), "failed to setup SMS infra")

			// SC + snapshot class after pod restart so provisioner topology is fresh
			err = createRBDStorageClass(clientSet, f, defaultSCName, nil, nil, deletePolicy)
			Expect(err).ShouldNot(HaveOccurred(), "failed to create storage class")

			// VolumeGroupSnapshot member snapshots have no VolumeSnapshotClass
			// name set, the sidecar falls back to the default class of the
			// driver to find the snapshotter secrets. Only make the RBD
			// snapshot class the default when the driver has no default
			// class yet, a second default class breaks that lookup.
			defaultClasses, err := listDefaultSnapshotClasses(smsDriverName)
			Expect(err).ShouldNot(HaveOccurred(), "failed to list default snapshot classes")
			err = createRBDSnapshotClassWithDefault(f, len(defaultClasses) == 0)
			Expect(err).ShouldNot(HaveOccurred(), "failed to create snapshot class")

			// The VolumeGroupSnapshot prerequisites are optional, when they
			// are not met only the VolumeGroupSnapshot tests are skipped.
			vgsSkipReason = checkDefaultSnapshotClasses(defaultClasses)
			if vgsSkipReason == "" {
				vgsHelper, vgsClassName, vgsSkipReason = setupGroupSnapshotClass(
					f, clientSet, testNamespace, defaultSCName)
			}
			if vgsSkipReason != "" {
				framework.Logf("VolumeGroupSnapshot tests will be skipped: %s", vgsSkipReason)
			}

			var localAddr string
			localAddr, stopPF, err = startPortForward(cephCSINamespace, smsServiceName, smsLocalPort, smsLocalPort)
			Expect(err).ShouldNot(HaveOccurred(), "failed to start port-forward")

			conn, smsClient, err = newSidecarGRPCClient(localAddr, infra.caCertPEM, cephCSINamespace)
			Expect(err).ShouldNot(HaveOccurred(), "failed to create gRPC client")

			token, err = requestSAToken(clientSet, smsTestSAName, testNamespace, smsAudience)
			Expect(err).ShouldNot(HaveOccurred(), "failed to get SA token")
		})

		AfterAll(func() {
			if !operatorDeployment {
				return
			}
			if conn != nil {
				_ = conn.Close()
			}
			if stopPF != nil {
				stopPF()
			}
			if vgsHelper != nil && vgsClassName != "" {
				if err := vgsHelper.DeleteVolumeGroupSnapshotClass(vgsClassName); err != nil {
					framework.Logf("Warning: failed to delete group snapshot class: %v", err)
				}
			}
			if clientSet != nil && dynClient != nil {
				cleanupSnapshotMetadataInfra(clientSet, dynClient, cephCSINamespace, testNamespace)
			}
			if err := deleteRBDSnapshotClass(); err != nil {
				framework.Logf("Warning: failed to delete snapshot class: %v", err)
			}
			if err := deleteResource(rbdExamplePath + "storageclass.yaml"); err != nil {
				framework.Logf("Warning: failed to delete storage class: %v", err)
			}
			if clientSet != nil {
				ctx := context.TODO()
				if err := clientSet.CoreV1().Secrets(cephCSINamespace).Delete(
					ctx, rbdProvisionerSecretName, metav1.DeleteOptions{}); err != nil && !apierrs.IsNotFound(err) {
					framework.Logf("Warning: failed to delete provisioner secret: %v", err)
				}
				if err := clientSet.CoreV1().Secrets(cephCSINamespace).Delete(
					ctx, rbdNodePluginSecretName, metav1.DeleteOptions{}); err != nil && !apierrs.IsNotFound(err) {
					framework.Logf("Warning: failed to delete node secret: %v", err)
				}
			}
			if err := deleteCephUser(f, keyringRBDProvisionerUsername); err != nil {
				framework.Logf("Warning: failed to delete provisioner ceph user: %v", err)
			}
			if err := deleteCephUser(f, keyringRBDNodePluginUsername); err != nil {
				framework.Logf("Warning: failed to delete node plugin ceph user: %v", err)
			}
			if err := deleteConfigMap(rbdDirPath); err != nil {
				framework.Logf("Warning: failed to delete configmap: %v", err)
			}
			if testNamespace != "" {
				if err := deleteNamespace(clientSet, testNamespace); err != nil {
					framework.Logf("Warning: failed to delete namespace %s: %v", testNamespace, err)
				}
			}
			if err := deleteNodeLabels(clientSet, []string{
				nodeRegionLabel, nodeZoneLabel,
				nodeCSIRegionLabel, nodeCSIZoneLabel,
				crushLocationRegionLabel, crushLocationZoneLabel,
			}); err != nil {
				framework.Logf("Warning: failed to delete node labels: %v", err)
			}
		})

		// TC-1: GetMetadataAllocated returns correct blocks
		It("should return allocated blocks for a snapshot", func() {
			pvc, snap := createBlockVolumeWithSnapshot(f, clientSet, testNamespace,
				"snap-alloc", []int{0, 1, 2, 3})
			defer cleanupBlockVolumeAndSnapshot(clientSet, pvc, &snap)

			ctx := context.TODO()
			blocks, volCap, err := collectAllocatedBlocks(
				ctx, smsClient, token, testNamespace, snap.Name, 0, smsMaxResults)
			Expect(err).ShouldNot(HaveOccurred())

			expectedOffsets := []int64{0, smsBlockBytes, 2 * smsBlockBytes, 3 * smsBlockBytes}
			verifyBlockMetadata(blocks, expectedOffsets, smsBlockBytes)
			Expect(volCap).To(Equal(int64(smsVolCapacity)))
		})

		// TC-2: GetMetadataDelta returns only changed blocks
		It("should return changed blocks between two snapshots", func() {
			pvc, snapBase := createBlockVolumeWithSnapshot(f, clientSet, testNamespace,
				"snap-base", []int{0, 1, 2, 3})
			defer cleanupBlockVolumeAndSnapshot(clientSet, pvc, &snapBase)

			baseHandle, err := getSnapshotHandle(testNamespace, snapBase.Name)
			Expect(err).ShouldNot(HaveOccurred())

			app2, err := loadApp(rawAppPath)
			Expect(err).ShouldNot(HaveOccurred())
			app2.Name = "raw-block-pod-delta"
			app2.Namespace = testNamespace
			app2.Spec.Volumes[0].PersistentVolumeClaim = &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.Name,
			}
			err = createApp(clientSet, app2, deployTimeout)
			Expect(err).ShouldNot(HaveOccurred())

			err = writeBlocksAtOffsets(f, app2.Name, testNamespace, smsDevicePath, smsBlockSize, []int{4, 5})
			Expect(err).ShouldNot(HaveOccurred())

			err = deletePod(app2.Name, testNamespace, clientSet, deployTimeout)
			Expect(err).ShouldNot(HaveOccurred())

			snapTarget := getSnapshot(snapshotPath)
			snapTarget.Name = "snap-target"
			snapTarget.Namespace = testNamespace
			snapTarget.Spec.Source.PersistentVolumeClaimName = &pvc.Name
			err = createSnapshot(&snapTarget, deployTimeout)
			Expect(err).ShouldNot(HaveOccurred())
			defer func() {
				if delErr := deleteSnapshot(&snapTarget, deployTimeout); delErr != nil {
					framework.Logf("Warning: failed to delete target snapshot: %v", delErr)
				}
			}()

			ctx := context.TODO()
			blocks, err := collectDeltaBlocks(
				ctx, smsClient, token, testNamespace,
				baseHandle, snapTarget.Name, 0, smsMaxResults)
			Expect(err).ShouldNot(HaveOccurred())

			expectedOffsets := []int64{4 * smsBlockBytes, 5 * smsBlockBytes}
			verifyBlockMetadata(blocks, expectedOffsets, smsBlockBytes)
		})

		// TC-3: Starting offset skips earlier blocks
		It("should skip blocks before starting-offset", func() {
			pvc, snap := createBlockVolumeWithSnapshot(f, clientSet, testNamespace,
				"snap-offset", []int{0, 1, 2, 3, 4, 5, 6, 7})
			defer cleanupBlockVolumeAndSnapshot(clientSet, pvc, &snap)

			ctx := context.TODO()
			startingOffset := int64(4 * smsBlockBytes)
			blocks, _, err := collectAllocatedBlocks(
				ctx, smsClient, token, testNamespace, snap.Name, startingOffset, smsMaxResults)
			Expect(err).ShouldNot(HaveOccurred())

			expectedOffsets := []int64{
				4 * smsBlockBytes, 5 * smsBlockBytes,
				6 * smsBlockBytes, 7 * smsBlockBytes,
			}
			verifyBlockMetadata(blocks, expectedOffsets, smsBlockBytes)
		})

		// TC-4: Empty volume snapshot
		It("should return empty for snapshot of empty volume", func() {
			pvc, snap := createBlockVolumeWithSnapshot(f, clientSet, testNamespace,
				"snap-empty", nil)
			defer cleanupBlockVolumeAndSnapshot(clientSet, pvc, &snap)

			ctx := context.TODO()
			blocks, _, err := collectAllocatedBlocks(
				ctx, smsClient, token, testNamespace, snap.Name, 0, smsMaxResults)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(blocks).To(BeEmpty(), "expected 0 blocks for empty volume snapshot")
		})

		// TC-5: No changes between snapshots
		It("should return empty delta when no changes between snapshots", func() {
			pvc, snapBase := createBlockVolumeWithSnapshot(f, clientSet, testNamespace,
				"snap-nochange-base", []int{0, 1})
			defer func() {
				_ = deletePVCAndValidatePV(clientSet, pvc, deployTimeout)
			}()

			baseHandle, err := getSnapshotHandle(testNamespace, snapBase.Name)
			Expect(err).ShouldNot(HaveOccurred())

			snapTarget := getSnapshot(snapshotPath)
			snapTarget.Name = "snap-nochange-target"
			snapTarget.Namespace = testNamespace
			snapTarget.Spec.Source.PersistentVolumeClaimName = &pvc.Name
			err = createSnapshot(&snapTarget, deployTimeout)
			Expect(err).ShouldNot(HaveOccurred())
			defer cleanupBlockVolumeAndSnapshot(clientSet, nil, &snapTarget)
			defer cleanupBlockVolumeAndSnapshot(clientSet, nil, &snapBase)

			ctx := context.TODO()
			blocks, err := collectDeltaBlocks(
				ctx, smsClient, token, testNamespace,
				baseHandle, snapTarget.Name, 0, smsMaxResults)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(blocks).To(BeEmpty(), "expected 0 delta blocks when no changes")
		})

		// TC-6: Max-results batching
		It("should batch responses according to max-results", func() {
			pvc, snap := createBlockVolumeWithSnapshot(f, clientSet, testNamespace,
				"snap-batch", []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
			defer cleanupBlockVolumeAndSnapshot(clientSet, pvc, &snap)

			ctx := context.TODO()
			maxResults := int32(3)
			responses, err := collectAllocatedResponses(
				ctx, smsClient, token, testNamespace, snap.Name, 0, maxResults)
			Expect(err).ShouldNot(HaveOccurred())

			totalBlocks := 0
			for _, resp := range responses {
				batchLen := len(resp.GetBlockMetadata())
				Expect(batchLen).To(BeNumerically("<=", int(maxResults)),
					"batch exceeded maxResults: got %d", batchLen)
				totalBlocks += batchLen
			}
			Expect(totalBlocks).To(Equal(10), "total blocks mismatch")
		})

		// TC-7: Invalid audience rejected
		It("should reject request with invalid audience", func() {
			wrongToken, err := requestSAToken(clientSet, smsTestSAName, testNamespace, "wrong-audience")
			Expect(err).ShouldNot(HaveOccurred())

			ctx := context.TODO()
			_, _, err = collectAllocatedBlocks(
				ctx, smsClient, wrongToken, testNamespace, "any-snap", 0, smsMaxResults)
			Expect(err).Should(HaveOccurred())

			st, ok := status.FromError(err)
			Expect(ok).To(BeTrue(), "expected gRPC status error")
			Expect(st.Code()).To(Equal(codes.Unauthenticated),
				"expected Unauthenticated, got %s", st.Code())
		})

		// TC-8: Missing RBAC rejected
		It("should reject request from SA without RBAC", func() {
			ctx := context.TODO()
			noRBACSA := "sms-no-rbac"

			sa := &v1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      noRBACSA,
					Namespace: testNamespace,
				},
			}
			_, err := clientSet.CoreV1().ServiceAccounts(testNamespace).Create(ctx, sa, metav1.CreateOptions{})
			Expect(err).ShouldNot(HaveOccurred())

			defer func() {
				delErr := clientSet.CoreV1().ServiceAccounts(testNamespace).Delete(ctx, noRBACSA, metav1.DeleteOptions{})
				if delErr != nil && !apierrs.IsNotFound(delErr) {
					framework.Logf("Warning: failed to delete SA %s: %v", noRBACSA, delErr)
				}
			}()

			noRBACToken, err := requestSAToken(clientSet, noRBACSA, testNamespace, smsAudience)
			Expect(err).ShouldNot(HaveOccurred())

			_, _, err = collectAllocatedBlocks(
				ctx, smsClient, noRBACToken, testNamespace, "any-snap", 0, smsMaxResults)
			Expect(err).Should(HaveOccurred())

			st, ok := status.FromError(err)
			Expect(ok).To(BeTrue(), "expected gRPC status error")
			Expect(st.Code()).To(Equal(codes.PermissionDenied),
				"expected PermissionDenied, got %s", st.Code())
		})

		// TC-9: GetMetadataAllocated returns correct blocks for VolumeGroupSnapshot members
		It("should return allocated blocks for VolumeGroupSnapshot member snapshots", func() {
			if vgsSkipReason != "" {
				Skip(vgsSkipReason)
			}

			labels := map[string]string{"sms-vgs": "alloc"}
			writeOffsets := map[string][]int{
				"sms-vgs-alloc-0": {0, 1},
				"sms-vgs-alloc-1": {2, 3, 4},
			}

			var pvcs []*v1.PersistentVolumeClaim
			defer func() {
				for _, pvc := range pvcs {
					cleanupBlockVolumeAndSnapshot(clientSet, pvc, nil)
				}
			}()
			for name, offsets := range writeOffsets {
				pvc, err := loadPVC(rawPvcPath)
				Expect(err).ShouldNot(HaveOccurred())
				pvc.Namespace = testNamespace
				pvc.Labels = labels

				app, err := loadApp(rawAppPath)
				Expect(err).ShouldNot(HaveOccurred())
				app.Namespace = testNamespace

				err = createPVCAndApp(name, f, pvc, app, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
				pvcs = append(pvcs, pvc)

				err = writeBlocksAtOffsets(f, app.Name, testNamespace, smsDevicePath, smsBlockSize, offsets)
				Expect(err).ShouldNot(HaveOccurred())

				err = deletePod(app.Name, testNamespace, clientSet, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
			}

			vgs, err := vgsHelper.CreateVolumeGroupSnapshot("sms-vgs-alloc", vgsClassName, labels)
			Expect(err).ShouldNot(HaveOccurred())
			defer func() {
				if delErr := vgsHelper.DeleteVolumeGroupSnapshot(vgs.Name); delErr != nil {
					framework.Logf("Warning: failed to delete VolumeGroupSnapshot: %v", delErr)
				}
			}()

			members, err := getGroupSnapshotMembers(testNamespace, vgs, slices.Collect(maps.Keys(writeOffsets)))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(members).To(HaveLen(len(writeOffsets)))

			ctx := context.TODO()
			for pvcName, offsets := range writeOffsets {
				snapName, ok := members[pvcName]
				Expect(ok).To(BeTrue(), "no member snapshot found for PVC %s", pvcName)

				blocks, volCap, err := collectAllocatedBlocks(
					ctx, smsClient, token, testNamespace, snapName, 0, smsMaxResults)
				Expect(err).ShouldNot(HaveOccurred())

				expectedOffsets := make([]int64, 0, len(offsets))
				for _, o := range offsets {
					expectedOffsets = append(expectedOffsets, int64(o)*smsBlockBytes)
				}
				verifyBlockMetadata(blocks, expectedOffsets, smsBlockBytes)
				Expect(volCap).To(Equal(int64(smsVolCapacity)))
			}
		})

		// TC-10: GetMetadataDelta returns only changed blocks between VolumeGroupSnapshot members
		It("should return changed blocks between VolumeGroupSnapshot member snapshots", func() {
			if vgsSkipReason != "" {
				Skip(vgsSkipReason)
			}

			labels := map[string]string{"sms-vgs": "delta"}
			baseOffsets := map[string][]int{
				"sms-vgs-delta-0": {0, 1},
				"sms-vgs-delta-1": {2, 3},
			}
			changedOffsets := map[string][]int{
				"sms-vgs-delta-0": {4},
				"sms-vgs-delta-1": {5, 6},
			}

			var pvcs []*v1.PersistentVolumeClaim
			defer func() {
				for _, pvc := range pvcs {
					cleanupBlockVolumeAndSnapshot(clientSet, pvc, nil)
				}
			}()
			for name, offsets := range baseOffsets {
				pvc, err := loadPVC(rawPvcPath)
				Expect(err).ShouldNot(HaveOccurred())
				pvc.Namespace = testNamespace
				pvc.Labels = labels

				app, err := loadApp(rawAppPath)
				Expect(err).ShouldNot(HaveOccurred())
				app.Namespace = testNamespace

				err = createPVCAndApp(name, f, pvc, app, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
				pvcs = append(pvcs, pvc)

				err = writeBlocksAtOffsets(f, app.Name, testNamespace, smsDevicePath, smsBlockSize, offsets)
				Expect(err).ShouldNot(HaveOccurred())

				err = deletePod(app.Name, testNamespace, clientSet, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
			}

			vgsBase, err := vgsHelper.CreateVolumeGroupSnapshot("sms-vgs-delta-base", vgsClassName, labels)
			Expect(err).ShouldNot(HaveOccurred())
			defer func() {
				if delErr := vgsHelper.DeleteVolumeGroupSnapshot(vgsBase.Name); delErr != nil {
					framework.Logf("Warning: failed to delete base VolumeGroupSnapshot: %v", delErr)
				}
			}()

			for name, offsets := range changedOffsets {
				app, err := loadApp(rawAppPath)
				Expect(err).ShouldNot(HaveOccurred())
				app.Name = name + "-delta"
				app.Namespace = testNamespace
				app.Spec.Volumes[0].PersistentVolumeClaim = &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: name,
				}
				err = createApp(clientSet, app, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())

				err = writeBlocksAtOffsets(f, app.Name, testNamespace, smsDevicePath, smsBlockSize, offsets)
				Expect(err).ShouldNot(HaveOccurred())

				err = deletePod(app.Name, testNamespace, clientSet, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
			}

			vgsTarget, err := vgsHelper.CreateVolumeGroupSnapshot("sms-vgs-delta-target", vgsClassName, labels)
			Expect(err).ShouldNot(HaveOccurred())
			defer func() {
				if delErr := vgsHelper.DeleteVolumeGroupSnapshot(vgsTarget.Name); delErr != nil {
					framework.Logf("Warning: failed to delete target VolumeGroupSnapshot: %v", delErr)
				}
			}()

			baseMembers, err := getGroupSnapshotMembers(testNamespace, vgsBase, slices.Collect(maps.Keys(baseOffsets)))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(baseMembers).To(HaveLen(len(baseOffsets)))

			targetMembers, err := getGroupSnapshotMembers(testNamespace, vgsTarget, slices.Collect(maps.Keys(baseOffsets)))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(targetMembers).To(HaveLen(len(baseOffsets)))

			ctx := context.TODO()
			for pvcName, offsets := range changedOffsets {
				baseName, ok := baseMembers[pvcName]
				Expect(ok).To(BeTrue(), "no base member snapshot found for PVC %s", pvcName)
				targetName, ok := targetMembers[pvcName]
				Expect(ok).To(BeTrue(), "no target member snapshot found for PVC %s", pvcName)

				baseHandle, err := getSnapshotHandle(testNamespace, baseName)
				Expect(err).ShouldNot(HaveOccurred())

				blocks, err := collectDeltaBlocks(
					ctx, smsClient, token, testNamespace,
					baseHandle, targetName, 0, smsMaxResults)
				Expect(err).ShouldNot(HaveOccurred())

				expectedOffsets := make([]int64, 0, len(offsets))
				for _, o := range offsets {
					expectedOffsets = append(expectedOffsets, int64(o)*smsBlockBytes)
				}
				verifyBlockMetadata(blocks, expectedOffsets, smsBlockBytes)
			}
		})

		// TC-11: VolumeGroupSnapshot members restore the data at the time of the group snapshot
		It("should restore point-in-time data from VolumeGroupSnapshot member snapshots", func() {
			if vgsSkipReason != "" {
				Skip(vgsSkipReason)
			}

			labels := map[string]string{"sms-vgs": "restore"}
			// write a marker at a different offset to each PVC
			markerOffsets := map[string]int{
				"sms-vgs-restore-0": 0,
				"sms-vgs-restore-1": 1,
			}

			var pvcs []*v1.PersistentVolumeClaim
			defer func() {
				for _, pvc := range pvcs {
					cleanupBlockVolumeAndSnapshot(clientSet, pvc, nil)
				}
			}()
			markers := map[string]string{}
			for name, offset := range markerOffsets {
				pvc, err := loadPVC(rawPvcPath)
				Expect(err).ShouldNot(HaveOccurred())
				pvc.Namespace = testNamespace
				pvc.Labels = labels

				app, err := loadApp(rawAppPath)
				Expect(err).ShouldNot(HaveOccurred())
				app.Namespace = testNamespace

				err = createPVCAndApp(name, f, pvc, app, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
				pvcs = append(pvcs, pvc)

				err = writeBlocksAtOffsets(f, app.Name, testNamespace, smsDevicePath, smsBlockSize, []int{offset})
				Expect(err).ShouldNot(HaveOccurred())

				markers[name], err = blockChecksum(f, app.Name, testNamespace, smsDevicePath, smsBlockSize, offset)
				Expect(err).ShouldNot(HaveOccurred())

				err = deletePod(app.Name, testNamespace, clientSet, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
			}
			Expect(markers["sms-vgs-restore-0"]).NotTo(Equal(markers["sms-vgs-restore-1"]),
				"markers of the PVCs are not distinct")

			vgs, err := vgsHelper.CreateVolumeGroupSnapshot("sms-vgs-restore", vgsClassName, labels)
			Expect(err).ShouldNot(HaveOccurred())
			defer func() {
				if delErr := vgsHelper.DeleteVolumeGroupSnapshot(vgs.Name); delErr != nil {
					framework.Logf("Warning: failed to delete VolumeGroupSnapshot: %v", delErr)
				}
			}()

			members, err := getGroupSnapshotMembers(testNamespace, vgs, slices.Collect(maps.Keys(markerOffsets)))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(members).To(HaveLen(len(markerOffsets)))

			// overwrite the markers in the source PVCs after the group snapshot
			for name, offset := range markerOffsets {
				app, err := loadApp(rawAppPath)
				Expect(err).ShouldNot(HaveOccurred())
				app.Name = name + "-overwrite"
				app.Namespace = testNamespace
				app.Spec.Volumes[0].PersistentVolumeClaim = &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: name,
				}
				err = createApp(clientSet, app, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())

				err = writeBlocksAtOffsets(f, app.Name, testNamespace, smsDevicePath, smsBlockSize, []int{offset})
				Expect(err).ShouldNot(HaveOccurred())

				sum, err := blockChecksum(f, app.Name, testNamespace, smsDevicePath, smsBlockSize, offset)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(sum).NotTo(Equal(markers[name]), "marker of PVC %s was not overwritten", name)

				err = deletePod(app.Name, testNamespace, clientSet, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
			}

			// restore each member and compare its marker with the one
			// written before the group snapshot
			var restored []*v1.PersistentVolumeClaim
			defer func() {
				for _, pvc := range restored {
					cleanupBlockVolumeAndSnapshot(clientSet, pvc, nil)
				}
			}()
			apiGroup := snapapi.GroupName
			for name, offset := range markerOffsets {
				snapName, ok := members[name]
				Expect(ok).To(BeTrue(), "no member snapshot found for PVC %s", name)

				pvc, err := loadPVC(rawPvcPath)
				Expect(err).ShouldNot(HaveOccurred())
				pvc.Namespace = testNamespace
				pvc.Spec.DataSource = &v1.TypedLocalObjectReference{
					APIGroup: &apiGroup,
					Kind:     "VolumeSnapshot",
					Name:     snapName,
				}

				app, err := loadApp(rawAppPath)
				Expect(err).ShouldNot(HaveOccurred())
				app.Namespace = testNamespace

				err = createPVCAndApp(name+"-restore", f, pvc, app, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
				restored = append(restored, pvc)

				sum, err := blockChecksum(f, app.Name, testNamespace, smsDevicePath, smsBlockSize, offset)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(sum).To(Equal(markers[name]),
					"restored PVC %s does not contain the data of PVC %s at the time of the group snapshot",
					pvc.Name, name)

				err = deletePod(app.Name, testNamespace, clientSet, deployTimeout)
				Expect(err).ShouldNot(HaveOccurred())
			}
		})
	})
})

// createBlockVolumeWithSnapshot creates a raw block PVC, writes data at the
// given 4MB offsets, deletes the pod, and creates a snapshot.
func createBlockVolumeWithSnapshot(
	f *framework.Framework,
	clientSet kubernetes.Interface,
	namespace, snapName string,
	writeOffsets []int,
) (*v1.PersistentVolumeClaim, snapapi.VolumeSnapshot) {
	pvc, err := loadPVC(rawPvcPath)
	Expect(err).ShouldNot(HaveOccurred())
	pvc.Namespace = namespace

	app, err := loadApp(rawAppPath)
	Expect(err).ShouldNot(HaveOccurred())
	app.Namespace = namespace

	err = createPVCAndApp("", f, pvc, app, deployTimeout)
	Expect(err).ShouldNot(HaveOccurred())

	if len(writeOffsets) > 0 {
		err = writeBlocksAtOffsets(f, app.Name, namespace, smsDevicePath, smsBlockSize, writeOffsets)
		Expect(err).ShouldNot(HaveOccurred())
	}

	err = deletePod(app.Name, namespace, clientSet, deployTimeout)
	Expect(err).ShouldNot(HaveOccurred())

	snap := getSnapshot(snapshotPath)
	snap.Name = snapName
	snap.Namespace = namespace
	snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
	err = createSnapshot(&snap, deployTimeout)
	Expect(err).ShouldNot(HaveOccurred())

	return pvc, snap
}

// cleanupBlockVolumeAndSnapshot deletes snapshot and PVC.
func cleanupBlockVolumeAndSnapshot(
	clientSet kubernetes.Interface,
	pvc *v1.PersistentVolumeClaim,
	snap *snapapi.VolumeSnapshot,
) {
	if snap != nil {
		err := deleteSnapshot(snap, deployTimeout)
		if err != nil {
			framework.Logf("Warning: failed to delete snapshot %s: %v", snap.Name, err)
		}
	}

	if pvc != nil {
		err := deletePVCAndValidatePV(clientSet, pvc, deployTimeout)
		if err != nil {
			framework.Logf("Warning: failed to delete PVC %s: %v", pvc.Name, err)
		}
	}
}
