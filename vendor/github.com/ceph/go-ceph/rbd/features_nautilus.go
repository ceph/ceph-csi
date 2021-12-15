package rbd

// #include <rbd/librbd.h>
import "C"

const (
	// FeatureMigrating is the representation of RBD_FEATURE_MIGRATING from
	// librbd
	FeatureMigrating = uint64(C.RBD_FEATURE_MIGRATING)

	// FeatureNameMigrating is the representation of
	// RBD_FEATURE_NAME_MIGRATING from librbd
	FeatureNameMigrating = C.RBD_FEATURE_NAME_MIGRATING
)

func init() {
	featureNameToBit[FeatureNameMigrating] = FeatureMigrating
}
