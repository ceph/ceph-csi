#!/bin/bash

set -e -o pipefail

# In case no value is specified, default values will be used.
gitrepo="https://github.com/ceph/ceph-csi"
workdir="tip/"
ref="devel"
base="devel"
history="no"

ARGUMENT_LIST=(
    "ref"
    "workdir"
    "gitrepo"
    "base"
)

opts=$(getopt \
    --longoptions "$(printf "%s:," "${ARGUMENT_LIST[@]}")history,help" \
    --name "$(basename "${0}")" \
    --options "" \
    -- "$@"
)
ret=$?

if [ ${ret} -ne 0 ]
then
    echo "Try '--help' for more information."
    exit 1
fi

eval set -- "${opts}"

while true; do
    case "${1}" in
    --help)
        shift
        echo "Options:"
        echo "--help|-h                 specify the flags"
        echo "--ref                     specify the reference of pr"
        echo "--workdir                 specify the working directory"
        echo "--gitrepo                 specify the git repository"
        echo "--base                    specify the base branch to checkout"
        echo "--history                 fetch the history of the base branch"
        echo " "
        echo "Sample Usage:"
        echo "./prepare.sh --gitrepo=https://github.com/example --workdir=/opt/build --ref=pull/123/head"
        exit 0
        ;;
    --gitrepo)
        shift
        gitrepo=${1}
        ;;
    --workdir)
        shift
        workdir=${1}
        ;;
    --ref)
        shift
        ref=${1}
        echo "${ref}"
        ;;
    --base)
        shift
        base=${1}
        ;;
    --history)
        history="yes"
        ;;
    --)
        shift
        break
        ;;
    esac
    shift
done

set -x

dnf -y install git podman make

# if --history is passed, don't pass --depth=1
depth='--depth=1'
if [[ "${history}" == 'yes' ]]
then
    depth=''
fi

git clone "${depth}" --branch="${base}" "${gitrepo}" "${workdir}"
cd "${workdir}"
git fetch origin "${ref}:tip/${ref}"
git checkout "tip/${ref}"
