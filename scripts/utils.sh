#!/bin/bash -E

KUBECTL_RETRY=5
KUBECTL_RETRY_DELAY=10

kubectl_retry() {
    local retries=0 action="${1}" ret=0 stdout stderr
    shift

    # temporary files for kubectl output
    stdout=$(mktemp rook-kubectl-stdout.XXXXXXXX)
    stderr=$(mktemp rook-kubectl-stderr.XXXXXXXX)

    while ! kubectl "${action}" "${@}" 2>"${stderr}" 1>"${stdout}"
    do
        # in case of a failure when running "create", ignore errors with "AlreadyExists"
        if [ "${action}" == 'create' ]
        then
            # count lines in stderr that do not have "AlreadyExists"
            ret=$(grep -cvw 'AlreadyExists' "${stderr}")
            if [ "${ret}" -eq 0 ]
            then
                # Success! stderr is empty after removing all "AlreadyExists" lines.
                break
            fi
        fi

        # in case of a failure when running "delete", ignore errors with "NotFound"
        if [ "${action}" == 'delete' ]
        then
            # count lines in stderr that do not have "NotFound"
            ret=$(grep -cvw 'NotFound' "${stderr}")
            if [ "${ret}" -eq 0 ]
            then
                # Success! stderr is empty after removing all "NotFound" lines.
                break
            fi
        fi

        retries=$((retries+1))
        if [ ${retries} -eq ${KUBECTL_RETRY} ]
        then
            ret=1
            break
        fi

	# log stderr and empty the tmpfile
	cat "${stderr}" > /dev/stderr
	true > "${stderr}"
	echo "kubectl_retry ${*} failed, will retry in ${KUBECTL_RETRY_DELAY} seconds"

        sleep ${KUBECTL_RETRY_DELAY}

	# reset ret so that a next working kubectl does not cause a non-zero
	# return of the function
        ret=0
    done

    # write output so that calling functions can consume it
    cat "${stdout}" > /dev/stdout
    cat "${stderr}" > /dev/stderr

    rm -f "${stdout}" "${stderr}"

    return ${ret}
}
