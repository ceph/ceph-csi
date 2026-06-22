//go:build !(nautilus || octopus)
// +build !nautilus,!octopus

package nfs

import (
	ccom "github.com/ceph/go-ceph/common/commands"
)

// Commander interface supports sending commands to Ceph.
type Commander interface {
	ccom.RadosBufferCommander
}

// Admin is used to administer ceph nfs features.
type Admin struct {
	conn Commander
}

// NewFromConn creates an new management object from a preexisting
// rados connection. The existing connection can be rados.Conn or any
// type implementing the RadosCommander interface.
func NewFromConn(conn Commander) *Admin {
	return &Admin{conn}
}
