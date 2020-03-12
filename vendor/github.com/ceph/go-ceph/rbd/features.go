package rbd

// #cgo LDFLAGS: -lrbd
// #include <rbd/librbd.h>
import "C"

const (
	// RBD features, bit values

	// FeatureLayering is the representation of RBD_FEATURE_LAYERING from
	// librbd
	FeatureLayering = uint64(C.RBD_FEATURE_LAYERING)

	// FeatureStripingV2 is the representation of RBD_FEATURE_STRIPINGV2
	// from librbd
	FeatureStripingV2 = uint64(C.RBD_FEATURE_STRIPINGV2)

	// FeatureExclusiveLock is the representation of
	// RBD_FEATURE_EXCLUSIVE_LOCK from librbd
	FeatureExclusiveLock = uint64(C.RBD_FEATURE_EXCLUSIVE_LOCK)

	// FeatureObjectMap is the representation of RBD_FEATURE_OBJECT_MAP
	// from librbd
	FeatureObjectMap = uint64(C.RBD_FEATURE_OBJECT_MAP)

	// FeatureFastDiff is the representation of RBD_FEATURE_FAST_DIFF from
	// librbd
	FeatureFastDiff = uint64(C.RBD_FEATURE_FAST_DIFF)

	// FeatureDeepFlatten is the representation of RBD_FEATURE_DEEP_FLATTEN
	// from librbd
	FeatureDeepFlatten = uint64(C.RBD_FEATURE_DEEP_FLATTEN)

	// FeatureJournaling is the representation of RBD_FEATURE_JOURNALING
	// from librbd
	FeatureJournaling = uint64(C.RBD_FEATURE_JOURNALING)

	// FeatureDataPool is the representation of RBD_FEATURE_DATA_POOL from
	// librbd
	FeatureDataPool = uint64(C.RBD_FEATURE_DATA_POOL)

	// RBD features, strings

	// FeatureNameLayering is the representation of
	// RBD_FEATURE_NAME_LAYERING from librbd
	FeatureNameLayering = C.RBD_FEATURE_NAME_LAYERING

	// FeatureNameStripingV2 is the representation of
	// RBD_FEATURE_NAME_STRIPINGV2 from librbd
	FeatureNameStripingV2 = C.RBD_FEATURE_NAME_STRIPINGV2

	// FeatureNameExclusiveLock is the representation of
	// RBD_FEATURE_NAME_EXCLUSIVE_LOCK from librbd
	FeatureNameExclusiveLock = C.RBD_FEATURE_NAME_EXCLUSIVE_LOCK

	// FeatureNameObjectMap is the representation of
	// RBD_FEATURE_NAME_OBJECT_MAP from librbd
	FeatureNameObjectMap = C.RBD_FEATURE_NAME_OBJECT_MAP

	// FeatureNameFastDiff is the representation of
	// RBD_FEATURE_NAME_FAST_DIFF from librbd
	FeatureNameFastDiff = C.RBD_FEATURE_NAME_FAST_DIFF

	// FeatureNameDeepFlatten is the representation of
	// RBD_FEATURE_NAME_DEEP_FLATTEN from librbd
	FeatureNameDeepFlatten = C.RBD_FEATURE_NAME_DEEP_FLATTEN

	// FeatureNameJournaling is the representation of
	// RBD_FEATURE_NAME_JOURNALING from librbd
	FeatureNameJournaling = C.RBD_FEATURE_NAME_JOURNALING

	// FeatureNameDataPool is the representation of
	// RBD_FEATURE_NAME_DATA_POOL from librbd
	FeatureNameDataPool = C.RBD_FEATURE_NAME_DATA_POOL

	// old names for backwards compatibility (unused?)
	RbdFeatureLayering      = FeatureLayering
	RbdFeatureStripingV2    = FeatureStripingV2
	RbdFeatureExclusiveLock = FeatureExclusiveLock
	RbdFeatureObjectMap     = FeatureObjectMap
	RbdFeatureFastDiff      = FeatureFastDiff
	RbdFeatureDeepFlatten   = FeatureDeepFlatten
	RbdFeatureJournaling    = FeatureJournaling
	RbdFeatureDataPool      = FeatureDataPool

	// the following are probably really unused?
	RbdFeaturesDefault        = uint64(C.RBD_FEATURES_DEFAULT)
	RbdFeaturesIncompatible   = uint64(C.RBD_FEATURES_INCOMPATIBLE)
	RbdFeaturesRwIncompatible = uint64(C.RBD_FEATURES_RW_INCOMPATIBLE)
	RbdFeaturesMutable        = uint64(C.RBD_FEATURES_MUTABLE)
	RbdFeaturesSingleClient   = uint64(C.RBD_FEATURES_SINGLE_CLIENT)
)

// FeatureSet is a combination of the bit value for multiple featurs.
type FeatureSet uint64

var (
	featureNameToBit = map[string]uint64{
		FeatureNameLayering:      FeatureLayering,
		FeatureNameStripingV2:    FeatureStripingV2,
		FeatureNameExclusiveLock: FeatureExclusiveLock,
		FeatureNameObjectMap:     FeatureObjectMap,
		FeatureNameFastDiff:      FeatureFastDiff,
		FeatureNameDeepFlatten:   FeatureDeepFlatten,
		FeatureNameJournaling:    FeatureJournaling,
		FeatureNameDataPool:      FeatureDataPool,
	}
)

func FeatureSetFromNames(names []string) FeatureSet {
	var fs uint64
	for _, name := range names {
		fs |= featureNameToBit[name]
	}
	return FeatureSet(fs)
}

func (fs *FeatureSet) Names() []string {
	names := []string{}

	for name, bit := range featureNameToBit {
		if (uint64(*fs) & bit) == bit {
			names = append(names, name)
		}
	}

	return names
}

// GetFeatures returns the features bitmask for the rbd image.
//
// Implements:
//  int rbd_get_features(rbd_image_t image, uint64_t *features);
func (image *Image) GetFeatures() (features uint64, err error) {
	if err := image.validate(imageIsOpen); err != nil {
		return 0, err
	}

	if ret := C.rbd_get_features(image.image, (*C.uint64_t)(&features)); ret < 0 {
		return 0, RBDError(ret)
	}

	return features, nil
}

// UpdateFeatures updates the features on the Image.
//
// Implements:
//   int rbd_update_features(rbd_image_t image, uint64_t features,
//                           uint8_t enabled);
func (image *Image) UpdateFeatures(features uint64, enabled bool) error {
	if image.image == nil {
		return RbdErrorImageNotOpen
	}

	cEnabled := C.uint8_t(0)
	if enabled {
		cEnabled = 1
	}
	return getError(C.rbd_update_features(image.image, C.uint64_t(features), cEnabled))
}
