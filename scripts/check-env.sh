#!/bin/sh
#
# Check the environment for dependencies and configuration
#

# count errors, run script to the end before exiting
ERRORS=0
fail() {
	echo "${*}" > /dev/stderr
	ERRORS=$((ERRORS+1))
}

# check if 'go' is available
[ -n "$(command -v go)" ] \
	|| fail "could not find 'go' executable"

# parse the Golang version, return the digit passed as argument
# 1.13.9 -> go_version 1 -> 1
# 1.13.9 -> go_version 2 -> 13
# 1.13.9 -> go_version 3 -> 9
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
