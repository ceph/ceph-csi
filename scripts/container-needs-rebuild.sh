#!/bin/sh
#
# Check for changes needed by the container image. Two flavours of images are
# supported, namely 'test' and 'devel'.
#
# Usage: scripts/container-needs-rebuild.sh {test|devel} [git-since]
#
#   - flavour is either 'test' or 'devel'
#   - the optional 'git-since' points to a git reference, 'origin/devel' if
#     not set
#
# Returns 0 in case changes do not affect the container image.
# Returns 10 in case the changes need a rebuild of the container image.
#

FLAVOUR="${1}"

if [ "${FLAVOUR}" != 'test' ] && [ "${FLAVOUR}" != 'devel' ]
then
	echo 'ERROR: flavour must be "test" or "devel"'
	exit 1
fi

GIT_SINCE="${2}"
if [ -z "${GIT_SINCE}" ]; then
	GIT_SINCE='origin/devel'
fi

MODIFIED_FILES=$(git diff --name-only "${GIT_SINCE}")

# files used for container image input
# :test container
CONTAINER_FILES="build.env scripts/Dockerfile.${FLAVOUR}"

for MF in ${MODIFIED_FILES}
do
	for CF in ${CONTAINER_FILES}
	do
		if [ "${MF}" = "${CF}" ]
		then
			echo "container needs rebuild, ${CF} was modified"
			exit 10
		fi
	done
done

exit 0
