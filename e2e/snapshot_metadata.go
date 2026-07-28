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
	"time"

	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/kubernetes-csi/external-snapshot-metadata/pkg/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

			err = createRBDSnapshotClass(f)
			Expect(err).ShouldNot(HaveOccurred(), "failed to create snapshot class")

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
