//go:build !nautilus
// +build !nautilus

package admin

import (
	"fmt"
)

// ImageSpec values are used to identify an RBD image wherever Ceph APIs
// require an image_spec/image_id_spec using image name/id and optional
// pool and namespace.
type ImageSpec struct {
	spec string
}

// NewImageSpec is used to construct an ImageSpec given an image name/id
// and optional namespace and pool names.
//
// NewImageSpec constructs an ImageSpec to identify an RBD image and thus
// requires image name/id, whereas NewLevelSpec constructs LevelSpec to
// identify entire pool, pool namespace or single RBD image, all of which
// requires pool name.
func NewImageSpec(pool, namespace, image string) ImageSpec {
	var s string
	if pool != "" && namespace != "" {
		s = fmt.Sprintf("%s/%s/%s", pool, namespace, image)
	} else if pool != "" {
		s = fmt.Sprintf("%s/%s", pool, image)
	} else {
		s = image
	}
	return ImageSpec{s}
}

// NewRawImageSpec returns a ImageSpec directly based on the spec string
// argument without constructing it from component values.
//
// This should only be used if NewImageSpec can not create the imagespec value
// you want to pass to ceph.
func NewRawImageSpec(spec string) ImageSpec {
	return ImageSpec{spec}
}
