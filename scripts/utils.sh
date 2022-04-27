#!/bin/bash

KUBECTL_RETRY=5
KUBECTL_RETRY_DELAY=10

# kubectl_retry calls `kubectl` with the passed arguments. In case of a
# failure, the `kubectl` command will be retried for `KUBECTL_RETRY` times,
# with a delay of `KUBECTL_RETRY_DELAY` between them.
#
# Upon creation failures, `AlreadyExists` and `Warning` are ignored, making
# sure the create succeeds in case some objects were created successfully in a
# previous try.
#
# Upon deletion failures, the same applies as for creation, except that
# NotFound is ignored.
#
# Logs from `kubectl` are passed on to stdout, so that a calling function can
# capture it. During the function, logs are written to stderr as to not
# interfere with the log parsing of the calling function.
kubectl_retry() {
    local retries=0 action="${1}" ret=0 stdout stderr
    shift

    # temporary files for kubectl output
    stdout=$(mktemp rook-kubectl-stdout.XXXXXXXX)
    stderr=$(mktemp rook-kubectl-stderr.XXXXXXXX)

    while ! ( kubectl "${action}" "${@}" 2>"${stderr}" 1>>"${stdout}" )
    do
        # write logs to stderr and empty stderr (only)
        cat "${stdout}" > /dev/stderr
        cat "${stderr}" > /dev/stderr
        echo "$(date): 'kubectl_retry ${action} ${*}' try #${retries} failed, checking errors" > /dev/stderr

        # in case of a failure when running "create", ignore errors with "AlreadyExists"
        if [ "${action}" == 'create' ]
        then
            # count lines in stderr that do not have "AlreadyExists" or "Warning"
            ret=$(grep -cvw -e 'AlreadyExists' -e '^Warning:' "${stderr}" || true)
            if [ "${ret}" -eq 0 ]
            then
                # Success! stderr is empty after removing all "AlreadyExists" lines.
                echo "$(date): 'kubectl_retry ${action} ${*}' succeeded without unknown errors" > /dev/stderr
                break
            fi
        fi

        # in case of a failure when running "delete", ignore errors with "NotFound"
        if [ "${action}" == 'delete' ]
        then
            # count lines in stderr that do not have "NotFound" or "Warning"
            ret=$(grep -cvw -e 'NotFound' -e '^Warning:' "${stderr}" || true)
            if [ "${ret}" -eq 0 ]
            then
                # Success! stderr is empty after removing all "NotFound" lines.
                echo "$(date): 'kubectl_retry ${action} ${*}' succeeded without unknown errors" > /dev/stderr
                break
            fi
        fi

        retries=$((retries+1))
        if [ ${retries} -eq ${KUBECTL_RETRY} ]
        then
            echo "$(date): 'kubectl_retry ${action} ${*}' failed, no more retries left (${retries}/${KUBECTL_RETRY})" > /dev/stderr
            ret=1
            break
        fi

        # empty stderr for the next loop
        true > "${stderr}"
        echo "$(date): 'kubectl_retry ${action} ${*}' failed (${retries}/${KUBECTL_RETRY}), will retry in ${KUBECTL_RETRY_DELAY} seconds" > /dev/stderr

        sleep ${KUBECTL_RETRY_DELAY}

        # reset ret so that a next working kubectl does not cause a non-zero
        # return of the function
        ret=0
    done

    echo "$(date): 'kubectl_retry ${action} ${*}' done (ret=${ret})" > /dev/stderr

    # write output so that calling functions can consume it
    cat "${stdout}" > /dev/stdout
    cat "${stderr}" > /dev/stderr

    rm -f "${stdout}" "${stderr}"

    return ${ret}
}
