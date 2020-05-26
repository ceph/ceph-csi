#!/bin/bash

# In case no value is specified, default values will be used.
gitrepo="https://github.com/ceph/ceph-csi"
workdir="tip/"
ref="master"

ARGUMENT_LIST=(
    "ref"
    "workdir"
    "gitrepo"
)

opts=$(getopt \
    --longoptions "$(printf "%s:," "${ARGUMENT_LIST[@]}")help" \
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

eval set -- ${opts}

while true; do
    case "${1}" in
    --help)  
        shift
        echo "Options:"
        echo "--help|-h                 specify the flags"
        echo "--ref                     specify the reference of pr"
        echo "--workdir                 specify the working directory"
        echo "--gitrepo                 specify the git repository" 
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
        echo ${ref}      
        ;;
    --)
        shift
        break
        ;;
    esac
    shift
done

set -x

yum -y install git podman

git clone --depth=1 ${gitrepo} ${workdir}
cd ${workdir}
git fetch --depth=1 origin "${ref}:tip/${ref}"
git checkout "tip/${ref}"
