package osd

import (
	ccom "github.com/ceph/go-ceph/common/commands"
	"github.com/ceph/go-ceph/internal/commands"
)

// Commander interface supports sending commands to Ceph.
type Commander interface {
	ccom.RadosCommander
}

// Admin is used to administer Ceph OSDs.
type Admin struct {
	conn Commander
}

// NewFromConn creates an new management object from a preexisting
// rados connection. The existing connection can be rados.Conn or any
// type implementing the RadosCommander interface.
func NewFromConn(conn Commander) *Admin {
	return &Admin{conn}
}

type response = commands.Response
