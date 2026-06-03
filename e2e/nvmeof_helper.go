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
	"fmt"
	"sync"

	"github.com/google/uuid"
	"k8s.io/kubernetes/test/e2e/framework"
)

// concurrentPodsResult holds the result of concurrent Pod operations.
type concurrentPodsResult struct {
	// uniqueName is the base name used for all Pods
	uniqueName string
	// totalCount is the number of Pods created/deleted
	totalCount int
	// errors contains any errors that occurred during operations
	errors []error
	// failed is the count of failed operations
	failed int
}

// String returns a human-readable summary of the result.
func (r *concurrentPodsResult) String() string {
	if r.failed == 0 {
		return fmt.Sprintf("All %d operations succeeded", r.totalCount)
	}

	return fmt.Sprintf("%d out of %d operations failed", r.failed, r.totalCount)
}

// HasErrors returns true if any operations failed.
func (r *concurrentPodsResult) HasErrors() bool {
	return r.failed > 0
}

// LogErrors logs all errors to the framework.
func (r *concurrentPodsResult) LogErrors() {
	for i, err := range r.errors {
		if err != nil {
			framework.Logf("Operation %s-%d failed: %v", r.uniqueName, i, err)
		}
	}
}

// createConcurrentPods creates multiple Pods using existing PVCs.
// This triggers NodeStage operations (Group A in GroupLock).
//
// Parameters:
//   - totalCount: number of Pods to create
//   - pvcBaseName: base name of existing PVCs to use
//   - pvcStartIndex: starting index for PVC names (e.g., 0 uses pvcBaseName-0, pvcBaseName-1, ...)
//   - appPath: path to Pod template YAML
//   - f: test framework
//
// Returns:
//   - *concurrentPodsResult: result containing uniqueName, errors, and counts
func createConcurrentPods(
	totalCount int,
	pvcBaseName string,
	pvcStartIndex int,
	appPath string,
	f *framework.Framework,
) *concurrentPodsResult {
	app, err := loadApp(appPath)
	if err != nil {
		return &concurrentPodsResult{
			totalCount: totalCount,
			errors:     []error{fmt.Errorf("failed to load app: %w", err)},
			failed:     1,
		}
	}
	app.Namespace = f.UniqueName

	uniqueName := uuid.NewString()
	framework.Logf("Creating %d Pods (base name: %s) using PVCs %s-%d to %s-%d",
		totalCount, uniqueName, pvcBaseName, pvcStartIndex, pvcBaseName, pvcStartIndex+totalCount-1)

	var wg sync.WaitGroup
	wgErrs := make([]error, totalCount)

	wg.Add(totalCount)
	for i := range totalCount {
		go func(n int) {
			defer wg.Done()
			// Deep copy to avoid data races on shared pod spec
			pod := app.DeepCopy()
			podName := fmt.Sprintf("%s-%d", uniqueName, n)
			pvcName := fmt.Sprintf("%s-%d", pvcBaseName, pvcStartIndex+n)

			// Update pod to use the specific PVC
			pod.Name = podName
			pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvcName

			wgErrs[n] = createApp(f.ClientSet, pod, deployTimeout)
		}(i)
	}

	wg.Wait()

	// Count failures
	failed := 0
	for _, err := range wgErrs {
		if err != nil {
			failed++
		}
	}

	return &concurrentPodsResult{
		uniqueName: uniqueName,
		totalCount: totalCount,
		errors:     wgErrs,
		failed:     failed,
	}
}

// deleteConcurrentPods deletes multiple Pods.
// This triggers NodeUnstage operations (Group B in GroupLock).
//
// Parameters:
//   - result: result from createConcurrentPods containing uniqueName
//   - f: test framework
//
// Returns:
//   - *concurrentPodsResult: result containing errors and counts
func deleteConcurrentPods(
	result *concurrentPodsResult,
	f *framework.Framework,
) *concurrentPodsResult {
	framework.Logf("Deleting %d Pods (base name: %s)", result.totalCount, result.uniqueName)

	var wg sync.WaitGroup
	wgErrs := make([]error, result.totalCount)

	wg.Add(result.totalCount)
	for i := range result.totalCount {
		go func(n int) {
			defer wg.Done()
			podName := fmt.Sprintf("%s-%d", result.uniqueName, n)
			wgErrs[n] = deletePod(podName, f.UniqueName, f.ClientSet, deployTimeout)
		}(i)
	}

	wg.Wait()

	// Count failures
	failed := 0
	for _, err := range wgErrs {
		if err != nil {
			failed++
		}
	}

	return &concurrentPodsResult{
		uniqueName: result.uniqueName,
		totalCount: result.totalCount,
		errors:     wgErrs,
		failed:     failed,
	}
}

// mixedCreateDeletePodsOnly performs mixed create and delete operations on Pods only.
// This function accurately tests the GroupLock in NodeServer by separating Controller
// operations (PVC) from Node operations (Pod).
//
// Flow:
//  1. Create all PVCs sequentially and validate they're Bound
//  2. Create initial batch of Pods using first set of PVCs (using helper function)
//  3. For each subsequent batch:
//     - Create new batch of Pods using next set of PVCs (NodeStage - Group A, using helper)
//     - Delete previous batch of Pods concurrently (NodeUnstage - Group B, using helper)
//     - Wait for pods to be Running
//  4. Delete final batch of Pods (using helper function)
//  5. Delete all PVCs sequentially
//
// This approach is more accurate for testing GroupLock because:
//   - PVC operations involve ControllerServer only
//   - Pod operations involve NodeServer (NodeStage/NodeUnstage) - where GroupLock lives
//
// Parameters:
//   - totalCount: total number of PVCs to create (must be evenly divisible by batchSize)
//   - batchSize: number of Pods per batch
//   - pvcPath: path to PVC template YAML
//   - appPath: path to Pod template YAML
//   - storageClassName: StorageClass name to use
//   - f: test framework
//
// Returns:
//   - error: any error that occurred during the operations
func mixedCreateDeletePodsOnly(
	totalCount, batchSize int,
	pvcPath, appPath, storageClassName string,
	f *framework.Framework,
) error {
	if batchSize <= 0 {
		return fmt.Errorf("batchSize must be greater than 0")
	}
	if totalCount%batchSize != 0 {
		return fmt.Errorf("totalCount (%d) must be evenly divisible by batchSize (%d)", totalCount, batchSize)
	}

	numBatches := totalCount / batchSize
	framework.Logf("Starting Pods-only test: %d total PVCs, %d batches of %d Pods",
		totalCount, numBatches, batchSize)

	// Step 1: Create all PVCs sequentially (not in parallel) and validate
	framework.Logf("Creating %d PVCs sequentially...", totalCount)

	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return fmt.Errorf("failed to load PVC: %w", err)
	}
	pvc.Namespace = f.UniqueName
	pvc.Spec.StorageClassName = &storageClassName

	pvcBaseName := uuid.NewString()

	// cleanupPodsAndPVCs cleans up pods (if result provided) and all PVCs
	cleanupPodsAndPVCs := func(podResult *concurrentPodsResult) {
		if podResult != nil {
			framework.Logf("Cleaning up %d pods...", podResult.totalCount)
			_ = deleteConcurrentPods(podResult, f)
		}

		framework.Logf("Cleaning up %d PVCs...", totalCount)
		for i := range totalCount {
			pvcName := fmt.Sprintf("%s-%d", pvcBaseName, i)
			pvcCopy := pvc.DeepCopy()
			pvcCopy.Name = pvcName
			_ = deletePVCAndValidatePV(f.ClientSet, pvcCopy, deployTimeout)
		}
	}

	// Create PVCs one by one
	for i := range totalCount {
		pvcName := fmt.Sprintf("%s-%d", pvcBaseName, i)
		pvcCopy := pvc.DeepCopy()
		pvcCopy.Name = pvcName

		framework.Logf("Creating PVC %d/%d: %s", i+1, totalCount, pvcName)
		err = createPVCAndvalidatePV(f.ClientSet, pvcCopy, deployTimeout)
		if err != nil {
			return fmt.Errorf("failed to create PVC %s: %w", pvcName, err)
		}
	}

	framework.Logf("Successfully created all %d PVCs", totalCount)

	var allErrors []error

	// Step 2: Create initial batch of Pods
	framework.Logf("Creating initial batch of %d Pods using PVCs %s-0 to %s-%d",
		batchSize, pvcBaseName, pvcBaseName, batchSize-1)

	previousResult := createConcurrentPods(batchSize, pvcBaseName, 0, appPath, f)
	if previousResult.HasErrors() {
		previousResult.LogErrors()

		// cleanup and return error immediately
		framework.Logf("Initial batch had %d failures, cleaning up...", previousResult.failed)
		cleanupPodsAndPVCs(previousResult)

		return fmt.Errorf("initial batch failed: %d out of %d pods failed to create",
			previousResult.failed, previousResult.totalCount)
	}

	// Wait for initial batch pods to be Running
	framework.Logf("Waiting for initial batch pods to be Running...")
	for i := range batchSize {
		podName := fmt.Sprintf("%s-%d", previousResult.uniqueName, i)
		err = waitForPodInRunningState(podName, f.UniqueName, f.ClientSet, deployTimeout, noError)
		if err != nil {
			// Cleanup: delete pods first, then PVCs
			framework.Logf("Initial batch failed to reach Running, cleaning up...")
			cleanupPodsAndPVCs(previousResult)

			return fmt.Errorf("initial batch pod %s did not reach Running state: %w", podName, err)
		}
	}

	// Step 3: Process remaining batches with mixed create/delete
	// Start from 1 because batch 0 was already pre-created above
	for batch := 1; batch < numBatches; batch++ {
		var createResult *concurrentPodsResult
		var deleteResult *concurrentPodsResult

		// Use WaitGroup to run create and delete truly concurrently
		var wg sync.WaitGroup

		// Calculate PVC offset for this batch (batch * batchSize)
		pvcOffset := batch * batchSize

		// Start creating current batch
		wg.Add(1)
		go func(batchNum int, offset int) {
			defer wg.Done()
			framework.Logf("Batch %d/%d: Creating %d Pods using PVCs %s-%d to %s-%d",
				batchNum+1, numBatches, batchSize, pvcBaseName, offset, pvcBaseName, offset+batchSize-1)

			createResult = createConcurrentPods(batchSize, pvcBaseName, offset, appPath, f)
		}(batch, pvcOffset)

		// Delete previous batch concurrently
		wg.Add(1)
		go func(batchNum int, prevResult *concurrentPodsResult) {
			defer wg.Done()
			framework.Logf("Batch %d/%d: Deleting previous batch (%d Pods) CONCURRENTLY with creates",
				batchNum+1, numBatches, prevResult.totalCount)

			deleteResult = deleteConcurrentPods(prevResult, f)
		}(batch, previousResult)

		// Wait for both operations to complete
		wg.Wait()

		// Check errors from create operation
		if createResult.HasErrors() {
			createResult.LogErrors()
			for _, err := range createResult.errors {
				if err != nil {
					allErrors = append(allErrors, err)
				}
			}
		}

		// Check errors from delete operation
		if deleteResult.HasErrors() {
			deleteResult.LogErrors()
			for _, err := range deleteResult.errors {
				if err != nil {
					allErrors = append(allErrors, err)
				}
			}
		}

		// Wait for current batch pods to be Running before next iteration
		// This provides explicit validation that pods are ready, even though
		// createApp() already waits internally.It is just a fast check.
		framework.Logf("Batch %d/%d: Waiting for current batch (%s) pods to be Running",
			batch+1, numBatches, createResult.uniqueName)
		for i := range batchSize {
			podName := fmt.Sprintf("%s-%d", createResult.uniqueName, i)
			err = waitForPodInRunningState(podName, f.UniqueName, f.ClientSet, deployTimeout, noError)
			if err != nil {
				// Cleanup current batch pods and all remaining batches' PVCs, then return error
				framework.Logf("Batch %d/%d: Pod %s did not reach Running, cleaning up...",
					batch+1, numBatches, podName)
				cleanupPodsAndPVCs(createResult)

				return fmt.Errorf("batch %d/%d: pod %s did not reach Running state: %w",
					batch+1, numBatches, podName, err)
			}
		}

		// Save current batch for next iteration
		previousResult = createResult
	}

	// Step 4: Delete final batch of Pods
	if previousResult != nil {
		framework.Logf("Deleting final batch (%d Pods)", previousResult.totalCount)
		finalDeleteResult := deleteConcurrentPods(previousResult, f)

		if finalDeleteResult.HasErrors() {
			finalDeleteResult.LogErrors()
			for _, err := range finalDeleteResult.errors {
				if err != nil {
					allErrors = append(allErrors, err)
				}
			}
		}
	}

	// Step 5: Delete all PVCs sequentially
	framework.Logf("Deleting all %d PVCs sequentially...", totalCount)
	for i := range totalCount {
		pvcName := fmt.Sprintf("%s-%d", pvcBaseName, i)
		pvcCopy := pvc.DeepCopy()
		pvcCopy.Name = pvcName

		framework.Logf("Deleting PVC %d/%d: %s", i+1, totalCount, pvcName)
		err = deletePVCAndValidatePV(f.ClientSet, pvcCopy, deployTimeout)
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf("failed to delete PVC %s: %w", pvcName, err))
		}
	}

	if len(allErrors) > 0 {
		return fmt.Errorf("%d operations failed during Pods-only test", len(allErrors))
	}

	framework.Logf("Pods-only test completed successfully: %d PVCs, %d batches of %d Pods",
		totalCount, numBatches, batchSize)

	return nil
}
