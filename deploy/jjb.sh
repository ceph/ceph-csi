#!/bin/sh
#
# Create a new Job in OCP that runs the jbb-validate container once. This
# script will wait for completion of the validation, and uses the result of the
# container to report the status.
#

CMD="${1}"

get_pod_status() {
	oc get pod/${1} --no-headers -o=jsonpath='{.status.phase}'
}

case "${CMD}" in
	"validate")
		;;
	"deploy")
		;;
	*)
		echo "no such command: ${CMD}"
		exit 1
		;;
esac

# make sure there is a valid OCP session
oc version || exit 1

# the deploy directory where this script is located, contains files we need
cd $(dirname ${0})

oc create -f jjb-${CMD}.yaml

# loop until pod is available
while true
do
	jjb_pod=$(oc get pods --no-headers -l job-name=jjb-${CMD} -o=jsonpath='{.items[0].metadata.name}')
	[ ${?} = 0 ] && [ -n "${jjb_pod}" ] && break
	sleep 1
done

# loop until the pod has finished
while true
do
	status=$(get_pod_status "${jjb_pod}")
	# TODO: is Running as a status sufficient, did it terminate yet?
	[ ${?} = 0 ] && ( [ "${status}" = "Succeeded" ] || [ "${status}" = "Failed" ] ) && break
	sleep 0.5
done

# show the log of the finished pod
oc logs "${jjb_pod}"

# delete the job, so a next run can create it again
oc delete --wait -f jjb-${CMD}.yaml

# return the exit status of the pod
[ "${status}" = 'Succeeded' ]
