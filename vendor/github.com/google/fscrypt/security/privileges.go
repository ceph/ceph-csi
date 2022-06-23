/*
 * privileges.go - Functions for managing users and privileges.
 *
 * Copyright 2017 Google Inc.
 * Author: Joe Richey (joerichey@google.com)
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */

// Package security manages:
//  - Cache clearing (cache.go)
//  - Privilege manipulation (privileges.go)
package security

// Use the libc versions of setreuid, setregid, and setgroups instead of the
// "sys/unix" versions.  The "sys/unix" versions use the raw syscalls which
// operate on the calling thread only, whereas the libc versions operate on the
// whole process.  And we need to operate on the whole process, firstly for
// pam_fscrypt to prevent the privileges of Go worker threads from diverging
// from the PAM stack's "main" thread, violating libc's assumption and causing
// an abort() later in the PAM stack; and secondly because Go code may migrate
// between OS-level threads while it's running.
//
// See also: https://github.com/golang/go/issues/1435

/*
#define _GNU_SOURCE    // for getresuid and setresuid
#include <sys/types.h>
#include <unistd.h>    // getting and setting uids and gids
#include <grp.h>       // setgroups
*/
import "C"

import (
	"log"
	"os/user"
	"syscall"

	"github.com/pkg/errors"

	"github.com/google/fscrypt/util"
)

// Privileges encapsulate the effective uid/gid and groups of a process.
type Privileges struct {
	euid   C.uid_t
	egid   C.gid_t
	groups []C.gid_t
}

// ProcessPrivileges returns the process's current effective privileges.
func ProcessPrivileges() (*Privileges, error) {
	ruid := C.getuid()
	euid := C.geteuid()
	rgid := C.getgid()
	egid := C.getegid()

	var groups []C.gid_t
	n, err := C.getgroups(0, nil)
	if n < 0 {
		return nil, err
	}
	// If n == 0, the user isn't in any groups, so groups == nil is fine.
	if n > 0 {
		groups = make([]C.gid_t, n)
		n, err = C.getgroups(n, &groups[0])
		if n < 0 {
			return nil, err
		}
		groups = groups[:n]
	}
	log.Printf("Current privs (real, effective): uid=(%d,%d) gid=(%d,%d) groups=%v",
		ruid, euid, rgid, egid, groups)
	return &Privileges{euid, egid, groups}, nil
}

// UserPrivileges returns the default privileges for the specified user.
func UserPrivileges(user *user.User) (*Privileges, error) {
	privs := &Privileges{
		euid: C.uid_t(util.AtoiOrPanic(user.Uid)),
		egid: C.gid_t(util.AtoiOrPanic(user.Gid)),
	}
	userGroups, err := user.GroupIds()
	if err != nil {
		return nil, util.SystemError(err.Error())
	}
	privs.groups = make([]C.gid_t, len(userGroups))
	for i, group := range userGroups {
		privs.groups[i] = C.gid_t(util.AtoiOrPanic(group))
	}
	return privs, nil
}

// SetProcessPrivileges sets the privileges of the current process to have those
// specified by privs. The original privileges can be obtained by first saving
// the output of ProcessPrivileges, calling SetProcessPrivileges with the
// desired privs, then calling SetProcessPrivileges with the saved privs.
func SetProcessPrivileges(privs *Privileges) error {
	log.Printf("Setting euid=%d egid=%d groups=%v", privs.euid, privs.egid, privs.groups)

	// If setting privs as root, we need to set the euid to 0 first, so that
	// we will have the necessary permissions to make the other changes to
	// the groups/egid/euid, regardless of our original euid.
	C.seteuid(0)

	// Separately handle the case where the user is in no groups.
	numGroups := C.size_t(len(privs.groups))
	groupsPtr := (*C.gid_t)(nil)
	if numGroups > 0 {
		groupsPtr = &privs.groups[0]
	}

	if res, err := C.setgroups(numGroups, groupsPtr); res < 0 {
		return errors.Wrapf(err.(syscall.Errno), "setting groups")
	}
	if res, err := C.setegid(privs.egid); res < 0 {
		return errors.Wrapf(err.(syscall.Errno), "setting egid")
	}
	if res, err := C.seteuid(privs.euid); res < 0 {
		return errors.Wrapf(err.(syscall.Errno), "setting euid")
	}
	ProcessPrivileges()
	return nil
}

// SetUids sets the process's real, effective, and saved UIDs.
func SetUids(ruid, euid, suid int) error {
	log.Printf("Setting ruid=%d euid=%d suid=%d", ruid, euid, suid)
	// We elevate all the privs before setting them. This prevents issues
	// with (ruid=1000,euid=1000,suid=0), where just a single call to
	// setresuid might fail with permission denied.
	if res, err := C.setresuid(0, 0, 0); res < 0 {
		return errors.Wrapf(err.(syscall.Errno), "setting uids")
	}
	if res, err := C.setresuid(C.uid_t(ruid), C.uid_t(euid), C.uid_t(suid)); res < 0 {
		return errors.Wrapf(err.(syscall.Errno), "setting uids")
	}
	return nil
}

// GetUids gets the process's real, effective, and saved UIDs.
func GetUids() (int, int, int) {
	var ruid, euid, suid C.uid_t
	C.getresuid(&ruid, &euid, &suid)
	return int(ruid), int(euid), int(suid)
}
