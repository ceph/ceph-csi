// +build !luminous

package rbd

// #include <rbd/librbd.h>
import "C"

const (
	// FeatureOperations is the representation of RBD_FEATURE_OPERATIONS
	// from librbd
	FeatureOperations = uint64(C.RBD_FEATURE_OPERATIONS)

	// FeatureNameOperations is the representation of
	// RBD_FEATURE_NAME_OPERATIONS from librbd
	FeatureNameOperations = C.RBD_FEATURE_NAME_OPERATIONS
)

func init() {
	featureNameToBit[FeatureNameOperations] = FeatureOperations
}
