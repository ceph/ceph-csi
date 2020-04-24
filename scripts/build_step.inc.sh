#!/bin/sh

# variables used for logging build steps, use build_step() and
# build_step_done() for setting or clearing them.
BUILD_STEP_FILE="$(mktemp)"
BUILD_STEP_PID=''
BUILD_STEP_DELAY='1m'

build_step_log() {
	echo "$(date) -- ${*}"
}

# start a build step, and log every
build_step() {
	echo "${*}" > "${BUILD_STEP_FILE}"
	build_step_log "starting: $(cat "${BUILD_STEP_FILE}")"
	[ -n "${BUILD_STEP_PID}" ] && return
	while sleep "${BUILD_STEP_DELAY}"
	do
		build_step_log "running: $(cat "${BUILD_STEP_FILE}")"
	done & BUILD_STEP_PID=${!}
}

# clean up the logging of build steps
build_steps_cleanup() {
	[ -n "${BUILD_STEP_PID}" ] && kill "${BUILD_STEP_PID}"
	rm -f "${BUILD_STEP_FILE}"
	BUILD_STEP_PID=''
}

# automatically stop build step logging and cleanup temporary file
trap build_steps_cleanup EXIT

