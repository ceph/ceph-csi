package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	v1 "k8s.io/api/core/v1"
	scv1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

/* #nosec:G101, values not credentials, just a reference to the location.*/
const (
	defaultNs = "default"

	// vaultBackendPath is the default VAULT_BACKEND_PATH for secrets
	vaultBackendPath = "secret/"
	// vaultPassphrasePath is an advanced configuration option, only
	// available for the VaultKMS (not VaultTokensKMS) provider.
	vaultPassphrasePath = "ceph-csi/"

	rookToolBoxPodLabel = "app=rook-ceph-tools"
	rbdMountOptions     = "mountOptions"

	retainPolicy = v1.PersistentVolumeReclaimRetain
	// deletePolicy is the default policy in E2E.
	deletePolicy = v1.PersistentVolumeReclaimDelete
	// Default key and label for Listoptions
	appKey   = "app"
	appLabel = "write-data-in-pod"

	// vaultTokens KMS type
	vaultTokens = "vaulttokens"
)

var (
	// cli flags
	deployTimeout    int
	deployCephFS     bool
	deployRBD        bool
	testCephFS       bool
	testRBD          bool
	upgradeTesting   bool
	upgradeVersion   string
	cephCSINamespace string
	rookNamespace    string
	radosNamespace   string
	ns               string
	vaultAddr        string
	poll             = 2 * time.Second
)

func initResouces() {
	ns = fmt.Sprintf("--namespace=%v", cephCSINamespace)
	vaultAddr = fmt.Sprintf("http://vault.%s.svc.cluster.local:8200", cephCSINamespace)
}

func getMons(ns string, c kubernetes.Interface) ([]string, error) {
	opt := metav1.ListOptions{
		LabelSelector: "app=rook-ceph-mon",
	}
	services := make([]string, 0)

	svcList, err := c.CoreV1().Services(ns).List(context.TODO(), opt)
	if err != nil {
		return services, err
	}
	for i := range svcList.Items {
		s := fmt.Sprintf("%s.%s.svc.cluster.local:%d", svcList.Items[i].Name, svcList.Items[i].Namespace, svcList.Items[i].Spec.Ports[0].Port)
		services = append(services, s)
	}
	return services, nil
}

func getStorageClass(path string) (scv1.StorageClass, error) {
	sc := scv1.StorageClass{}
	err := unmarshal(path, &sc)
	return sc, err
}

func getSecret(path string) (v1.Secret, error) {
	sc := v1.Secret{}
	err := unmarshal(path, &sc)
	// discard corruptInputError
	if err != nil {
		var b64cie base64.CorruptInputError
		if !errors.As(err, &b64cie) {
			return sc, err
		}
	}
	return sc, nil
}

func deleteResource(scPath string) error {
	data, err := replaceNamespaceInTemplate(scPath)
	if err != nil {
		e2elog.Logf("failed to read content from %s %v", scPath, err)
	}
	_, err = framework.RunKubectlInput(cephCSINamespace, data, ns, "delete", "-f", "-")
	if err != nil {
		e2elog.Logf("failed to delete %s %v", scPath, err)
	}
	return err
}

func unmarshal(fileName string, obj interface{}) error {
	f, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	data, err := utilyaml.ToJSON(f)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, obj)
	return err
}

// createPVCAndApp creates pvc and pod
// if name is not empty same will be set as pvc and app name.
func createPVCAndApp(name string, f *framework.Framework, pvc *v1.PersistentVolumeClaim, app *v1.Pod, pvcTimeout int) error {
	if name != "" {
		pvc.Name = name
		app.Name = name
		app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = name
	}
	err := createPVCAndvalidatePV(f.ClientSet, pvc, pvcTimeout)
	if err != nil {
		return err
	}
	err = createApp(f.ClientSet, app, deployTimeout)
	return err
}

// deletePVCAndApp delete pvc and pod
// if name is not empty same will be set as pvc and app name.
func deletePVCAndApp(name string, f *framework.Framework, pvc *v1.PersistentVolumeClaim, app *v1.Pod) error {
	if name != "" {
		pvc.Name = name
		app.Name = name
		app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = name
	}

	err := deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return err
	}
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	return err
}

func createPVCAndAppBinding(pvcPath, appPath string, f *framework.Framework, pvcTimeout int) (*v1.PersistentVolumeClaim, *v1.Pod, error) {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return nil, nil, err
	}
	pvc.Namespace = f.UniqueName

	app, err := loadApp(appPath)
	if err != nil {
		return nil, nil, err
	}
	app.Namespace = f.UniqueName

	err = createPVCAndApp("", f, pvc, app, pvcTimeout)
	if err != nil {
		return nil, nil, err
	}

	return pvc, app, nil
}

func validatePVCAndAppBinding(pvcPath, appPath string, f *framework.Framework) error {
	pvc, app, err := createPVCAndAppBinding(pvcPath, appPath, f, deployTimeout)
	if err != nil {
		return err
	}
	err = deletePVCAndApp("", f, pvc, app)
	return err
}

func getMountType(appName, appNamespace, mountPath string, f *framework.Framework) (string, error) {
	opt := metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", appName).String(),
	}
	cmd := fmt.Sprintf("lsblk -o TYPE,MOUNTPOINT | grep '%s' | awk '{print $1}'", mountPath)
	stdOut, stdErr, err := execCommandInPod(f, cmd, appNamespace, &opt)
	if err != nil {
		return "", err
	}
	if stdErr != "" {
		return strings.TrimSpace(stdOut), fmt.Errorf(stdErr)
	}
	return strings.TrimSpace(stdOut), nil
}

// readVaultSecret method will execute few commands to try read the secret for
// specified key from inside the vault container:
//  * authenticate with vault and ignore any stdout (we do not need output)
//  * issue get request for particular key
// resulting in stdOut (first entry in tuple) - output that contains the key
// or stdErr (second entry in tuple) - error getting the key.
func readVaultSecret(key string, usePassphrasePath bool, f *framework.Framework) (string, string) {
	extraPath := vaultPassphrasePath
	if !usePassphrasePath {
		extraPath = ""
	}

	loginCmd := fmt.Sprintf("vault login -address=%s sample_root_token_id > /dev/null", vaultAddr)
	readSecret := fmt.Sprintf("vault kv get -address=%s -field=data %s%s%s",
		vaultAddr, vaultBackendPath, extraPath, key)
	cmd := fmt.Sprintf("%s && %s", loginCmd, readSecret)
	opt := metav1.ListOptions{
		LabelSelector: "app=vault",
	}
	stdOut, stdErr := execCommandInPodAndAllowFail(f, cmd, cephCSINamespace, &opt)
	return strings.TrimSpace(stdOut), strings.TrimSpace(stdErr)
}

func validateNormalUserPVCAccess(pvcPath string, f *framework.Framework) error {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}
	pvc.Namespace = f.UniqueName
	pvc.Name = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return err
	}
	var user int64 = 2000
	app := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-run-as-non-root",
			Namespace: f.UniqueName,
			Labels: map[string]string{
				"app": "pod-run-as-non-root",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:    "write-pod",
					Image:   "registry.centos.org/centos:latest",
					Command: []string{"/bin/sleep", "999999"},
					SecurityContext: &v1.SecurityContext{
						RunAsUser: &user,
					},
					VolumeMounts: []v1.VolumeMount{
						{
							MountPath: "/target",
							Name:      "target",
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "target",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvc.Name,
							ReadOnly:  false},
					},
				},
			},
		},
	}

	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}

	opt := metav1.ListOptions{
		LabelSelector: "app=pod-run-as-non-root",
	}
	_, stdErr, err := execCommandInPod(f, "echo testing > /target/testing", app.Namespace, &opt)
	if err != nil {
		return nil
	}
	if stdErr != "" {
		return fmt.Errorf("failed to touch a file as non-root user %v", stdErr)
	}
	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return err
	}

	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	return err
}

// writeDataInPod fill zero content to a file in the provided POD volume.
func writeDataInPod(app *v1.Pod, opt *metav1.ListOptions, f *framework.Framework) error {
	app.Namespace = f.UniqueName

	err := createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}

	// write data to PVC. The idea here is to fill some content in the file
	// instead of filling and reverifying the md5sum/data integrity
	filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
	// While writing more data we are encountering issues in E2E timeout, so keeping it low for now
	_, writeErr, err := execCommandInPod(f, fmt.Sprintf("dd if=/dev/zero of=%s bs=1M count=10 status=none", filePath), app.Namespace, opt)
	if err != nil {
		return err
	}
	if writeErr != "" {
		err = fmt.Errorf("failed to write data %v", writeErr)
	}

	return err
}

func checkDataPersist(pvcPath, appPath string, f *framework.Framework) error {
	data := "checking data persist"
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}

	pvc.Namespace = f.UniqueName

	app, err := loadApp(appPath)
	if err != nil {
		return err
	}
	app.Labels = map[string]string{"app": "validate-data"}
	app.Namespace = f.UniqueName

	err = createPVCAndApp("", f, pvc, app, deployTimeout)
	if err != nil {
		return err
	}

	opt := metav1.ListOptions{
		LabelSelector: "app=validate-data",
	}
	// write data to PVC
	filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"

	_, stdErr, err := execCommandInPod(f, fmt.Sprintf("echo %s > %s", data, filePath), app.Namespace, &opt)
	if err != nil {
		return nil
	}
	if stdErr != "" {
		return fmt.Errorf("failed to write data to a file %v", stdErr)
	}
	// delete app
	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		return err
	}
	// recreate app and check data persist
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}
	persistData, stdErr, err := execCommandInPod(f, fmt.Sprintf("cat %s", filePath), app.Namespace, &opt)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to get file content %v", stdErr)
	}
	if !strings.Contains(persistData, data) {
		return fmt.Errorf("data not persistent expected data %s received data %s  ", data, persistData)
	}

	err = deletePVCAndApp("", f, pvc, app)
	return err
}

func pvcDeleteWhenPoolNotFound(pvcPath string, cephfs bool, f *framework.Framework) error {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}
	pvc.Namespace = f.UniqueName

	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return err
	}
	if cephfs {
		err = deleteBackingCephFSVolume(f, pvc)
		if err != nil {
			return err
		}
		// delete cephfs filesystem
		err = deletePool("myfs", cephfs, f)
		if err != nil {
			return err
		}
	} else {
		err = deleteBackingRBDImage(f, pvc)
		if err != nil {
			return err
		}
		// delete rbd pool
		err = deletePool(defaultRBDPool, cephfs, f)
		if err != nil {
			return err
		}
	}
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	return err
}

func checkMountOptions(pvcPath, appPath string, f *framework.Framework, mountFlags []string) error {
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return err
	}

	pvc.Namespace = f.UniqueName

	app, err := loadApp(appPath)
	if err != nil {
		return err
	}
	app.Labels = map[string]string{"app": "validate-mount-opt"}
	app.Namespace = f.UniqueName

	err = createPVCAndApp("", f, pvc, app, deployTimeout)
	if err != nil {
		return err
	}

	opt := metav1.ListOptions{
		LabelSelector: "app=validate-mount-opt",
	}

	cmd := fmt.Sprintf("mount |grep %s", app.Spec.Containers[0].VolumeMounts[0].MountPath)
	data, stdErr, err := execCommandInPod(f, cmd, app.Namespace, &opt)
	if err != nil {
		return err
	}
	if stdErr != "" {
		return fmt.Errorf("failed to get mount point %v", stdErr)
	}
	for _, f := range mountFlags {
		if !strings.Contains(data, f) {
			return fmt.Errorf("mount option %s not found in %s", f, data)
		}
	}

	err = deletePVCAndApp("", f, pvc, app)
	return err
}

func addTopologyDomainsToDSYaml(template, labels string) string {
	return strings.ReplaceAll(template, "# - \"--domainlabels=failure-domain/region,failure-domain/zone\"",
		"- \"--domainlabels="+labels+"\"")
}

func oneReplicaDeployYaml(template string) string {
	var re = regexp.MustCompile(`(\s+replicas:) \d+`)
	return re.ReplaceAllString(template, `$1 1`)
}

func enableTopologyInTemplate(data string) string {
	return strings.ReplaceAll(data, "--feature-gates=Topology=false", "--feature-gates=Topology=true")
}

func writeDataAndCalChecksum(app *v1.Pod, opt *metav1.ListOptions, f *framework.Framework) (string, error) {
	filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
	// write data in PVC
	err := writeDataInPod(app, opt, f)
	if err != nil {
		e2elog.Logf("failed to write data in the pod with error %v", err)
		return "", err
	}

	checkSum, err := calculateSHA512sum(f, app, filePath, opt)
	if err != nil {
		e2elog.Logf("failed to calculate checksum with error %v", err)
		return checkSum, err
	}

	err = deletePod(app.Name, app.Namespace, f.ClientSet, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to delete pod with error %v", err)
	}
	return checkSum, nil
}

// nolint:gocyclo,gocognit // reduce complexity
func validatePVCClone(totalCount int, sourcePvcPath, sourceAppPath, clonePvcPath, clonePvcAppPath string, validatePVC validateFunc, f *framework.Framework) {
	var wg sync.WaitGroup
	wgErrs := make([]error, totalCount)
	chErrs := make([]error, totalCount)
	pvc, err := loadPVC(sourcePvcPath)
	if err != nil {
		e2elog.Failf("failed to load PVC with error %v", err)
	}

	label := make(map[string]string)
	pvc.Namespace = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to create PVC with error %v", err)
	}
	app, err := loadApp(sourceAppPath)
	if err != nil {
		e2elog.Failf("failed to load app with error %v", err)
	}
	label[appKey] = appLabel
	app.Namespace = f.UniqueName
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
	app.Labels = label
	opt := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
	}

	checkSum := ""
	pvc, err = f.ClientSet.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(context.TODO(), pvc.Name, metav1.GetOptions{})
	if err != nil {
		e2elog.Failf("failed to get pvc %v", err)
	}
	if *pvc.Spec.VolumeMode == v1.PersistentVolumeFilesystem {
		checkSum, err = writeDataAndCalChecksum(app, &opt, f)
		if err != nil {
			e2elog.Failf("failed to calculate checksum with error %v", err)
		}
	}
	// validate created backend rbd images
	validateRBDImageCount(f, 1)
	pvcClone, err := loadPVC(clonePvcPath)
	if err != nil {
		e2elog.Failf("failed to load PVC with error %v", err)
	}
	pvcClone.Spec.DataSource.Name = pvc.Name
	pvcClone.Namespace = f.UniqueName
	appClone, err := loadApp(clonePvcAppPath)
	if err != nil {
		e2elog.Failf("failed to load application with error %v", err)
	}
	appClone.Namespace = f.UniqueName
	wg.Add(totalCount)
	// create clone and bind it to an app
	for i := 0; i < totalCount; i++ {
		go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			label := make(map[string]string)
			label[appKey] = name
			a.Labels = label
			opt := metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
			}
			wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
			if *pvc.Spec.VolumeMode == v1.PersistentVolumeFilesystem && wgErrs[n] == nil {
				filePath := a.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				checkSumClone := ""
				e2elog.Logf("Calculating checksum clone for filepath %s", filePath)
				checkSumClone, chErrs[n] = calculateSHA512sum(f, &a, filePath, &opt)
				e2elog.Logf("checksum for clone is %s", checkSumClone)
				if chErrs[n] != nil {
					e2elog.Logf("Failed calculating checksum clone %s", chErrs[n])
				}
				if checkSumClone != checkSum {
					e2elog.Logf("checksum didn't match. checksum=%s and checksumclone=%s", checkSum, checkSumClone)
				}
			}
			if wgErrs[n] == nil && validatePVC != nil {
				wgErrs[n] = validatePVC(f, &p, &a)
			}
			w.Done()
		}(&wg, i, *pvcClone, *appClone)
	}
	wg.Wait()

	failed := 0
	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to create PVC (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		e2elog.Failf("creating PVCs failed, %d errors were logged", failed)
	}

	for i, err := range chErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to calculate checksum (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		e2elog.Failf("calculating checksum failed, %d errors were logged", failed)
	}

	// total images in cluster is 1 parent rbd image+ total
	// temporary clone+ total clones
	totalCloneCount := totalCount + totalCount + 1
	validateRBDImageCount(f, totalCloneCount)
	// delete parent pvc
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to delete PVC with error %v", err)
	}

	totalCloneCount = totalCount + totalCount
	validateRBDImageCount(f, totalCloneCount)
	wg.Add(totalCount)
	// delete clone and app
	for i := 0; i < totalCount; i++ {
		go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			p.Spec.DataSource.Name = name
			wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
			w.Done()
		}(&wg, i, *pvcClone, *appClone)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to delete PVC and application (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		e2elog.Failf("deleting PVCs and applications failed, %d errors were logged", failed)
	}

	validateRBDImageCount(f, 0)
}

// nolint:gocyclo,gocognit,nestif // reduce complexity
func validatePVCSnapshot(totalCount int, pvcPath, appPath, snapshotPath, pvcClonePath, appClonePath, kms string, validateEncryption bool, f *framework.Framework) {
	var wg sync.WaitGroup
	wgErrs := make([]error, totalCount)
	chErrs := make([]error, totalCount)
	wg.Add(totalCount)

	err := createRBDSnapshotClass(f)
	if err != nil {
		e2elog.Failf("failed to create storageclass with error %v", err)
	}
	defer func() {
		err = deleteRBDSnapshotClass()
		if err != nil {
			e2elog.Failf("failed to delete VolumeSnapshotClass: %v", err)
		}
	}()

	pvc, err := loadPVC(pvcPath)
	if err != nil {
		e2elog.Failf("failed to load PVC with error %v", err)
	}
	label := make(map[string]string)
	pvc.Namespace = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to create PVC with error %v", err)
	}
	app, err := loadApp(appPath)
	if err != nil {
		e2elog.Failf("failed to load app with error %v", err)
	}
	// write data in PVC
	label[appKey] = appLabel
	app.Namespace = f.UniqueName
	app.Labels = label
	opt := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
	}
	app.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = pvc.Name
	checkSum, err := writeDataAndCalChecksum(app, &opt, f)
	if err != nil {
		e2elog.Failf("failed to calculate checksum with error %v", err)
	}
	validateRBDImageCount(f, 1)
	snap := getSnapshot(snapshotPath)
	snap.Namespace = f.UniqueName
	snap.Spec.Source.PersistentVolumeClaimName = &pvc.Name
	// create snapshot
	for i := 0; i < totalCount; i++ {
		go func(w *sync.WaitGroup, n int, s v1beta1.VolumeSnapshot) {
			s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
			wgErrs[n] = createSnapshot(&s, deployTimeout)
			if wgErrs[n] == nil && validateEncryption {
				if kmsIsVault(kms) || kms == vaultTokens {
					content, sErr := getVolumeSnapshotContent(s.Namespace, s.Name)
					if sErr != nil {
						wgErrs[n] = fmt.Errorf("failed to get snapshotcontent for %s in namespace %s with error: %w", s.Name, s.Namespace, sErr)
					} else {
						// check new passphrase created
						_, stdErr := readVaultSecret(*content.Status.SnapshotHandle, kmsIsVault(kms), f)
						if stdErr != "" {
							wgErrs[n] = fmt.Errorf("failed to read passphrase from vault: %s", stdErr)
						}
					}
				}
			}
			w.Done()
		}(&wg, i, snap)
	}
	wg.Wait()

	failed := 0
	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to create snapshot (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		e2elog.Failf("creating snapshots failed, %d errors were logged", failed)
	}

	// total images in cluster is 1 parent rbd image+ total snaps
	validateRBDImageCount(f, totalCount+1)
	pvcClone, err := loadPVC(pvcClonePath)
	if err != nil {
		e2elog.Failf("failed to load PVC with error %v", err)
	}
	appClone, err := loadApp(appClonePath)
	if err != nil {
		e2elog.Failf("failed to load application with error %v", err)
	}
	pvcClone.Namespace = f.UniqueName
	appClone.Namespace = f.UniqueName
	pvcClone.Spec.DataSource.Name = fmt.Sprintf("%s%d", f.UniqueName, 0)

	// create multiple PVC from same snapshot
	wg.Add(totalCount)
	for i := 0; i < totalCount; i++ {
		go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			label := make(map[string]string)
			label[appKey] = name
			a.Labels = label
			opt := metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", appKey, label[appKey]),
			}
			wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
			if wgErrs[n] == nil {
				filePath := a.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
				checkSumClone := ""
				e2elog.Logf("calculating checksum clone for filepath %s", filePath)
				checkSumClone, chErrs[n] = calculateSHA512sum(f, &a, filePath, &opt)
				e2elog.Logf("checksum value for the clone is %s with pod name %s", checkSumClone, name)
				if chErrs[n] != nil {
					e2elog.Logf("failed to calculte checksum for clone with error %s", chErrs[n])
				}
				if checkSumClone != checkSum {
					e2elog.Logf("checksum value didn't match. checksum=%s and checksumclone=%s", checkSum, checkSumClone)
				}
			}
			if wgErrs[n] == nil && validateEncryption {
				wgErrs[n] = isEncryptedPVC(f, &p, &a)
			}
			w.Done()
		}(&wg, i, *pvcClone, *appClone)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to create PVC and application (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		e2elog.Failf("creating PVCs and applications failed, %d errors were logged", failed)
	}

	for i, err := range chErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to calculate checksum (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		e2elog.Failf("calculating checksum failed, %d errors were logged", failed)
	}
	// total images in cluster is 1 parent rbd image+ total
	// snaps+ total clones
	totalCloneCount := totalCount + totalCount + 1
	validateRBDImageCount(f, totalCloneCount)
	wg.Add(totalCount)
	// delete clone and app
	for i := 0; i < totalCount; i++ {
		go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			p.Spec.DataSource.Name = name
			wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
			w.Done()
		}(&wg, i, *pvcClone, *appClone)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to delete PVC and application (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		e2elog.Failf("deleting PVCs and applications failed, %d errors were logged", failed)
	}

	// total images in cluster is 1 parent rbd image+ total
	// snaps
	validateRBDImageCount(f, totalCount+1)
	// create clones from different snapshots and bind it to an
	// app
	wg.Add(totalCount)
	for i := 0; i < totalCount; i++ {
		go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			p.Spec.DataSource.Name = name
			wgErrs[n] = createPVCAndApp(name, f, &p, &a, deployTimeout)
			w.Done()
		}(&wg, i, *pvcClone, *appClone)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to create PVC and application (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		e2elog.Failf("creating PVCs and applications failed, %d errors were logged", failed)
	}

	// total images in cluster is 1 parent rbd image+ total
	// snaps+ total clones
	totalCloneCount = totalCount + totalCount + 1
	validateRBDImageCount(f, totalCloneCount)
	// delete parent pvc
	err = deletePVCAndValidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		e2elog.Failf("failed to delete PVC with error %v", err)
	}

	// total images in cluster is total snaps+ total clones
	totalSnapCount := totalCount + totalCount
	validateRBDImageCount(f, totalSnapCount)
	wg.Add(totalCount)
	// delete snapshot
	for i := 0; i < totalCount; i++ {
		go func(w *sync.WaitGroup, n int, s v1beta1.VolumeSnapshot) {
			s.Name = fmt.Sprintf("%s%d", f.UniqueName, n)
			content := &v1beta1.VolumeSnapshotContent{}
			var err error
			if validateEncryption {
				if kmsIsVault(kms) || kms == vaultTokens {
					content, err = getVolumeSnapshotContent(s.Namespace, s.Name)
					if err != nil {
						wgErrs[n] = fmt.Errorf("failed to get snapshotcontent for %s in namespace %s with error: %w", s.Name, s.Namespace, err)
					}
				}
			}
			if wgErrs[n] == nil {
				wgErrs[n] = deleteSnapshot(&s, deployTimeout)
				if wgErrs[n] == nil && validateEncryption {
					if kmsIsVault(kms) || kms == vaultTokens {
						// check passphrase deleted
						stdOut, _ := readVaultSecret(*content.Status.SnapshotHandle, kmsIsVault(kms), f)
						if stdOut != "" {
							wgErrs[n] = fmt.Errorf("passphrase found in vault while should be deleted: %s", stdOut)
						}
					}
				}
			}
			w.Done()
		}(&wg, i, snap)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to delete snapshot (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		e2elog.Failf("deleting snapshots failed, %d errors were logged", failed)
	}

	validateRBDImageCount(f, totalCount)
	wg.Add(totalCount)
	// delete clone and app
	for i := 0; i < totalCount; i++ {
		go func(w *sync.WaitGroup, n int, p v1.PersistentVolumeClaim, a v1.Pod) {
			name := fmt.Sprintf("%s%d", f.UniqueName, n)
			p.Spec.DataSource.Name = name
			wgErrs[n] = deletePVCAndApp(name, f, &p, &a)
			w.Done()
		}(&wg, i, *pvcClone, *appClone)
	}
	wg.Wait()

	for i, err := range wgErrs {
		if err != nil {
			// not using Failf() as it aborts the test and does not log other errors
			e2elog.Logf("failed to delete PVC and application (%s%d): %v", f.UniqueName, i, err)
			failed++
		}
	}
	if failed != 0 {
		e2elog.Failf("deleting PVCs and applications failed, %d errors were logged", failed)
	}

	// validate created backend rbd images
	validateRBDImageCount(f, 0)
}

// validateController simulates the required operations to validate the
// controller.
// Controller will generates the omap data when the PV is created.
// for that we need to do below operations
// Create PVC with Retain policy
// Store the PVC and PV kubernetes objects so that we can create static
// binding between PVC-PV
// Delete the omap data created for PVC
// Create the static PVC and PV and let controller regenerate the omap
// Mount the PVC to application (NodeStage/NodePublish should work)
// Resize the PVC
// Delete the Application and PVC.
func validateController(f *framework.Framework, pvcPath, appPath, scPath string) error {
	size := "1Gi"
	poolName := defaultRBDPool
	expandSize := "10Gi"
	var err error
	// create storageclass with retain
	err = createRBDStorageClass(f.ClientSet, f, nil, nil, retainPolicy)
	if err != nil {
		return fmt.Errorf("failed to create storageclass with error %v", err)
	}

	// create pvc
	pvc, err := loadPVC(pvcPath)
	if err != nil {
		return fmt.Errorf("failed to load PVC with error %v", err)
	}
	resizePvc, err := loadPVC(pvcPath)
	if err != nil {
		return fmt.Errorf("failed to load PVC with error %v", err)
	}
	resizePvc.Namespace = f.UniqueName

	pvc.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(size)
	pvc.Namespace = f.UniqueName
	err = createPVCAndvalidatePV(f.ClientSet, pvc, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to create PVC with error %v", err)
	}
	// get pvc and pv object
	pvc, pv, err := getPVCAndPV(f.ClientSet, pvc.Name, pvc.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get PVC with error %v", err)
	}
	// Recreate storageclass with delete policy
	err = deleteResource(scPath)
	if err != nil {
		return fmt.Errorf("failed to delete storageclass with error %v", err)
	}
	err = createRBDStorageClass(f.ClientSet, f, nil, nil, deletePolicy)
	if err != nil {
		return fmt.Errorf("failed to create storageclass with error %v", err)
	}
	// delete omap data
	err = deletePVCImageJournalInPool(f, pvc, poolName)
	if err != nil {
		return err
	}
	err = deletePVCCSIJournalInPool(f, pvc, poolName)
	if err != nil {
		return err
	}
	// delete pvc and pv
	err = deletePVCAndPV(f.ClientSet, pvc, pv, deployTimeout)
	if err != nil {
		return fmt.Errorf("failed to delete PVC or PV with error %v", err)
	}
	// create pvc and pv with application
	pv.Spec.ClaimRef = nil
	pv.Spec.PersistentVolumeReclaimPolicy = deletePolicy
	// unset the resource version as should not be set on objects to be created
	pvc.ResourceVersion = ""
	pv.ResourceVersion = ""
	err = createPVCAndPV(f.ClientSet, pvc, pv)
	if err != nil {
		e2elog.Failf("failed to create PVC or PV with error %v", err)
	}
	// bind PVC to application
	app, err := loadApp(appPath)
	if err != nil {
		return err
	}
	app.Labels = map[string]string{"app": "resize-pvc"}
	app.Namespace = f.UniqueName
	opt := metav1.ListOptions{
		LabelSelector: "app=resize-pvc",
	}
	err = createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}
	// resize PVC
	err = expandPVCSize(f.ClientSet, resizePvc, expandSize, deployTimeout)
	if err != nil {
		return err
	}
	if *pvc.Spec.VolumeMode == v1.PersistentVolumeFilesystem {
		err = checkDirSize(app, f, &opt, expandSize)
		if err != nil {
			return err
		}
	}

	if *pvc.Spec.VolumeMode == v1.PersistentVolumeBlock {
		err = checkDeviceSize(app, f, &opt, expandSize)
		if err != nil {
			return err
		}
	}
	// delete pvc and storageclass
	err = deletePVCAndApp("", f, resizePvc, app)
	if err != nil {
		return err
	}
	return deleteResource(rbdExamplePath + "storageclass.yaml")
}

// k8sVersionGreaterEquals checks the ServerVersion of the Kubernetes cluster
// and compares it to the major.minor version passed. In case the version of
// the cluster is equal or higher to major.minor, `true` is returned, `false`
// otherwise.
//
// If fetching the ServerVersion of the Kubernetes cluster fails, the calling
// test case is marked as `FAILED` and gets aborted.
//
// nolint:unparam // currently major is always 1, this can change in the future
func k8sVersionGreaterEquals(c kubernetes.Interface, major, minor int) bool {
	v, err := c.Discovery().ServerVersion()
	if err != nil {
		e2elog.Failf("failed to get server version with error %v", err)
		// Failf() marks the case as failure, and returns from the
		// Go-routine that runs the case. This function will not have a
		// return value.
	}

	maj := fmt.Sprintf("%d", major)
	min := fmt.Sprintf("%d", minor)

	return (v.Major > maj) || (v.Major == maj && v.Minor >= min)
}
