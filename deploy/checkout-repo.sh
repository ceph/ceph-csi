#!/bin/sh

fail() {
	echo "${@}" > /dev/stderr
	exit 1
}

[ -n "${GIT_REPO}" ] || fail 'GIT_REPO environment variable not set'
[ -n "${GIT_REF}" ] || fail 'GIT_REF environment variable not set'

# exit in case a command fails
set -e

git init .
git remote add origin "${GIT_REPO}"
git fetch origin "${GIT_REF}"
git checkout FETCH_HEAD
