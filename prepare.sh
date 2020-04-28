#!/bin/sh

ARGUMENT_LIST=(
    "ref"
    "workdir"
    "gitrepo"
)

opts=$(getopt \
    --longoptions "$(printf "%s:," "${ARGUMENT_LIST[@]}") $(printf "help::")" \
    --name "$(basename "${0}")" \
    --options "" \
    -- "$@"
)

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
        #extracting Pull Id from passed reference
        ghprbPullId="$(echo ${ref} | cut -d '/' -f2)"         
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

#validates to true if ghprbPullId is not empty 
if [ -n "${ghprbPullId}" ]
then
    git clone --depth=1 ${gitrepo} ${workdir}
    cd ${workdir}
    git fetch origin "${ref}:pr_${ghprbPullId}"
    git checkout "pr_${ghprbPullId}"
fi
