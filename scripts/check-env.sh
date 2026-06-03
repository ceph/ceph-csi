#!/bin/sh
#
# Check the environment for dependencies and configuration
#

# check appropriate package installer to recommend corresponding packages
RPM_CMD=$(command -v rpm)
DPKG_CMD=$(command -v dpkg)

# count errors, run script to the end before exiting
ERRORS=0
fail() {
	echo "${*}" > /dev/stderr
	ERRORS=$((ERRORS+1))
}

# create a temp go file
LIBCHECK=$(mktemp)
mv "${LIBCHECK}" "${LIBCHECK}".go
LIBCHECK=${LIBCHECK}.go

# check for packages using a compile test
cat << EOF > "${LIBCHECK}"
package main

/*
#include <rados/librados.h>
#include <rbd/librbd.h>
*/
import "C"

func main() {
	_ = C.LIBRADOS_VERSION_CODE
	_ = C.LIBRBD_VER_MAJOR
	_ = C.RBD_MAX_IMAGE_NAME_SIZE
}
EOF

# check if 'go' is available
if [ -n "$(command -v go)" ]; then
	# in case of a failed execution, the user will be informed about
	# the missing packages based on whether they are on rpm or debian
	# based systems.
	if ! go run -mod=vendor "${LIBCHECK}" > /dev/null; then
		if [ -n "${RPM_CMD}" ]; then
			echo "Packages librbd-devel librados-devel need to be installed"
		elif [ -n "${DPKG_CMD}" ]; then
			echo "Packages librbd-dev librados-dev need to be installed"
		else
			fail "error can't verify Ceph development headers"
		fi
		echo "To build ceph-csi in a container: $ make containerized-build"
	fi

	# remove the temp file
	rm -f "${LIBCHECK}"
else
	fail "could not find 'go' executable"
	echo "To build ceph-csi in a container: $ make containerized-build"
fi

# parse the Golang version, return the digit passed as argument
# 1.16.4 -> go_version 1 -> 1
# 1.16.4 -> go_version 2 -> 16
# 1.16.4 -> go_version 3 -> 4
go_version() {
	go version | cut -d' ' -f3 | sed 's/^go//' | cut -d'.' -f"${1}"
}

# Golang needs to be > 1.13
GO_MAJOR=$(go_version 1)
GO_MINOR=$(go_version 2)
if ! [ "${GO_MAJOR}" -gt 1 ]
then
	if ! { [ "${GO_MAJOR}" -eq 1 ] && [ "${GO_MINOR}" -ge 13 ]; }
	then
		fail "go version needs to be >= 1.13"
	fi
fi

# we're building with modules, so GO111MODULE needs to be 'on' or 'auto'
# some versions of Go return nothing when GO111MODULE is not explicitly
# configured
[ "$(go env GO111MODULE 2>&1)" = '' ] || [ "$(go env GO111MODULE)" = 'on' ] || [ "$(go env GO111MODULE)" = 'auto' ] \
	|| fail "GO111MODULE should be set to 'on' or 'auto'"

# CGO needs to be enabled, we build with go-ceph language bindings
[ "$(go env CGO_ENABLED)" = '1' ] \
	|| fail "CGO_ENABLED should be set to '1'"

exit ${ERRORS}
