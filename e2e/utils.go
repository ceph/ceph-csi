package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	scv1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2elog "k8s.io/kubernetes/test/e2e/framework/log"
)

/* #nosec:G101, values not credententials, just a reference to the location.*/
const (
	defaultNs     = "default"
	vaultSecretNs = "/secret/ceph-csi/"

	// rook created cephfs user
	cephfsNodePluginSecretName  = "rook-csi-cephfs-node"
	cephfsProvisionerSecretName = "rook-csi-cephfs-provisioner"

	// rook created rbd user
	rbdNodePluginSecretName  = "rook-csi-rbd-node"
	rbdProvisionerSecretName = "rook-csi-rbd-provisioner"

	rookTolBoxPodLabel = "app=rook-ceph-tools"
	rbdmountOptions    = "mountOptions"
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

// updateSecretForEncryption is an hack to update the secrets created by rook to
// include the encyption key
// TODO in cephcsi we need to create own users in ceph cluster and use it for E2E.
func updateSecretForEncryption(c kubernetes.Interface) error {
	secrets, err := c.CoreV1().Secrets(rookNamespace).Get(context.TODO(), rbdProvisionerSecretName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	secrets.Data["encryptionPassphrase"] = []byte("test_passphrase")

	_, err = c.CoreV1().Secrets(rookNamespace).Update(context.TODO(), secrets, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	secrets, err = c.CoreV1().Secrets(rookNamespace).Get(context.TODO(), rbdNodePluginSecretName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	secrets.Data["encryptionPassphrase"] = []byte("test_passphrase")

	_, err = c.CoreV1().Secrets(rookNamespace).Update(context.TODO(), secrets, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
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
	if pvc == nil {
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
func readVaultSecret(key string, f *framework.Framework) (string, string) {
	loginCmd := fmt.Sprintf("vault login -address=%s sample_root_token_id > /dev/null", vaultAddr)
	readSecret := fmt.Sprintf("vault kv get -address=%s %s%s", vaultAddr, vaultSecretNs, key)
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
					Image:   "alpine",
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
func writeDataInPod(app *v1.Pod, f *framework.Framework) error {
	app.Labels = map[string]string{"app": "write-data-in-pod"}
	app.Namespace = f.UniqueName

	err := createApp(f.ClientSet, app, deployTimeout)
	if err != nil {
		return err
	}
	opt := metav1.ListOptions{
		LabelSelector: "app=write-data-in-pod",
	}
	// write data to PVC. The idea here is to fill some content in the file
	// instead of filling and reverifying the md5sum/data integrity
	filePath := app.Spec.Containers[0].VolumeMounts[0].MountPath + "/test"
	// While writing more data we are encountering issues in E2E timeout, so keeping it low for now
	_, writeErr, err := execCommandInPod(f, fmt.Sprintf("dd if=/dev/zero of=%s bs=1M count=10 status=none", filePath), app.Namespace, &opt)
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
	if pvc == nil {
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
	if pvc == nil {
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
