//go:build !(nautilus || octopus)
// +build !nautilus,!octopus

package nfs

import (
	ccom "github.com/ceph/go-ceph/common/commands"
)

// Admin is used to administer ceph nfs features.
type Admin struct {
	conn ccom.RadosCommander
}

// NewFromConn creates an new management object from a preexisting
// rados connection. The existing connection can be rados.Conn or any
// type implementing the RadosCommander interface.
func NewFromConn(conn ccom.RadosCommander) *Admin {
	return &Admin{conn}
}
