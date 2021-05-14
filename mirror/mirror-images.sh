#!/bin/bash
#
# Mirror images listed in images.txt to the internal CI registry.
#
# Set DOCKER_CONFIG_JSON to the contents of ~/.docker/config.json to use the
# authentication details from a cached configuration file.
#
# Set CI_REGISTRY_USER and CI_REGISTRY_PASSWD with the login details for the CI
# registry.
#

CI_REGISTRY='registry-ceph-csi.apps.ocp.ci.centos.org'

# get_image_entries returns the contents of images.txt without comments or empty lines
function get_image_entries() {
	grep -v -e '^#' -e '^$' images.txt
}

# create a temporary containers/auth.json for skopeo usage
REGISTRY_AUTH_FILE="$(mktemp -t)"
export REGISTRY_AUTH_FILE

# make the temporary containers/auth.json a valid json file
echo '{}' > "${REGISTRY_AUTH_FILE}"

# if DOCKER_CONFIG_JSON is non-empty, use the credentials from there
if [ -n "${DOCKER_CONFIG_JSON}" ]
then
	echo 'using ~/docker/config.json from env:DOCKER_CONFIG_JSON'
	echo "${DOCKER_CONFIG_JSON}" > "${REGISTRY_AUTH_FILE}"
fi

# login on the CI registry if the user/passwd are available
if [ -n "${CI_REGISTRY_USER}" ] && [ -n "${CI_REGISTRY_PASSWD}" ]
then
	echo 'using CI registry login from env:CI_REGISTRY_USER and env:CI_REGISTRY_PASSWD'
	skopeo login \
		--username="${CI_REGISTRY_USER}" \
		--password="${CI_REGISTRY_PASSWD}" \
		"${CI_REGISTRY}"
fi

# read each line from get_image_entries() into an array
while read -r -a IMAGE_ENTRY
do
	# 1st entry in the array is the source
	# 2nd entry in the array is the destination in the CI registry
	SRC="docker://${IMAGE_ENTRY[0]}"
	DST="docker://${CI_REGISTRY}/${IMAGE_ENTRY[1]}"
	echo "going to copy ${SRC} to ${DST}" 
	skopeo copy "${SRC}" "${DST}"
done <<< "$(get_image_entries)"
