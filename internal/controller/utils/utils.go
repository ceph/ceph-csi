package utils

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	"github.com/ceph/ceph-csi/internal/util"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RBDVolCmd               = "rbd"
	RBDExportArg            = "export"
	RBDDUArg                = "du"
	RBDExportDiffArg        = "export-diff"
	RBDImportDiffArg        = "import-diff"
	RBDImportArg            = "import"
	RBDRenameArg            = "rename"
	RBDRemoveArg            = "rm"
	RBDCreateArg            = "create"
	RBDTrashMoveArg         = "trash move"
	RBDTrashPurgeArg        = "trash purge"
	RBDPurgeArg             = "snap purge"
	RBDFinalizer     string = "rbd.ceph.io/finalizer"
)

func GetCredentials(
	ctx context.Context,
	client client.Client,
	name,
	namespace string) (*util.Credentials, error) {
	var cr *util.Credentials

	if name == "" || namespace == "" {
		errStr := "secret name or secret namespace is empty"
		util.ErrorLogMsg(errStr)

		return nil, errors.New(errStr)
	}
	secret := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret)
	if err != nil {
		return nil, fmt.Errorf("error getting secret %s in namespace %s: %w", name, namespace, err)
	}

	credentials := map[string]string{}
	for key, value := range secret.Data {
		credentials[key] = string(value)
	}

	cr, err = util.NewUserCredentials(credentials)
	if err != nil {
		util.ErrorLogMsg("failed to get user credentials %s", err)

		return nil, err
	}

	return cr, nil
}
