//go:build !nautilus
// +build !nautilus

package admin

import (
	"fmt"

	ccom "github.com/ceph/go-ceph/common/commands"
)

// RBDAdmin is used to administrate rbd volumes and pools.
type RBDAdmin struct {
	conn ccom.RadosCommander
}

// NewFromConn creates an new management object from a preexisting
// rados connection. The existing connection can be rados.Conn or any
// type implementing the RadosCommander interface.
func NewFromConn(conn ccom.RadosCommander) *RBDAdmin {
	return &RBDAdmin{conn}
}

// LevelSpec values are used to identify RBD objects wherever Ceph APIs
// require a levelspec to select an image, pool, or namespace.
type LevelSpec struct {
	spec string
}

// NewLevelSpec is used to construct a LevelSpec given a pool and
// optional namespace and image names.
func NewLevelSpec(pool, namespace, image string) LevelSpec {
	var s string
	if image != "" && namespace != "" {
		s = fmt.Sprintf("%s/%s/%s", pool, namespace, image)
	} else if image != "" {
		s = fmt.Sprintf("%s/%s", pool, image)
	} else if namespace != "" {
		s = fmt.Sprintf("%s/%s/", pool, namespace)
	} else {
		s = fmt.Sprintf("%s/", pool)
	}
	return LevelSpec{s}
}

// NewRawLevelSpec returns a LevelSpec directly based on the spec string
// argument without constructing it from component values. This should only be
// used if NewLevelSpec can not create the levelspec value you want to pass to
// ceph.
func NewRawLevelSpec(spec string) LevelSpec {
	return LevelSpec{spec}
}
