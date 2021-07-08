package e2e

import (
	"fmt"  //nolint:goimports
	"sync" //nolint:goimports

	snapapi "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

type configItem struct {
	ID         int // Unique identification
	f          *framework.Framework
	obj        *objects // Objects which are set for callback
	callbackFn string
	retErr     chan error // error channel to receive errors
}

type objects struct {
	snap *snapapi.VolumeSnapshot
	pvc  *v1.PersistentVolumeClaim
	app  *v1.Pod
}

func executeOperations(ch <-chan configItem) {
	for proc := range ch {
		switch proc.callbackFn {
		case "createsnap":
			mySnap := *(proc.obj.snap)
			mySnap.Name = fmt.Sprintf("%s%d", proc.f.UniqueName, proc.ID)
			err := createSnapshot(&mySnap, deployTimeout)
			proc.retErr <- err
		case "deletepvcandapp":
			name := fmt.Sprintf("%s%d", proc.f.UniqueName, proc.ID)
			if proc.obj != nil && proc.obj.pvc != nil {
				proc.obj.pvc.Spec.DataSource.Name = name
			}
			err := deletePVCAndApp(name, proc.f, proc.obj.pvc, proc.obj.app)
			proc.retErr <- err
		default:
			proc.retErr <- fmt.Errorf("unknown callback function")
		}
	}
}

func dispatch(
	fwk *framework.Framework, passedObj *objects, callbackFunc string, sendCh chan configItem, tc int) []configItem {
	configs := make([]configItem, tc)
	for i := 0; i < tc; i++ {
		proc := configItem{ID: i, f: fwk, obj: passedObj, callbackFn: callbackFunc, retErr: make(chan error)}
		configs[i] = proc
		sendCh <- proc
	}
	// todo: discard
	return configs
}

func getRoutines(c int) chan configItem {
	procCh := make(chan configItem, c)
	for i := 0; i < c; i++ {
		go executeOperations(procCh)
	}
	return procCh
}

func waitForAll(totalCount int, processes []configItem, cfunc string) (int, error) {
	var newWg sync.WaitGroup
	failed := 0
	for i := 0; i < totalCount; i++ {
		newWg.Add(1)
		job := i
		go func(ch <-chan error) {
			defer newWg.Done()
			result := <-ch
			if result != nil {
				e2elog.Logf("%v failed with error: %v on process: %d ", cfunc, result, job)
				failed++
			} else {
				e2elog.Logf("successfully executed job for process: %d", job)
			}
		}(processes[i].retErr)
	}
	newWg.Wait()
	if failed != 0 {
		return failed, fmt.Errorf("creating snapshots failed, %d errors were logged", failed)
	}
	return 0, nil
}
