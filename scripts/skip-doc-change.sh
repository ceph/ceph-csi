#!/bin/bash -e
#

GIT_SINCE="${1}"
if [ -z "${GIT_SINCE}" ]; then
	GIT_SINCE='origin/master'
fi

CHANGED_FILES=$(git diff --name-only "${GIT_SINCE}")

[[ -z $CHANGED_FILES ]] && exit 1

skip=0
#files to be skipped
declare -a FILES=(^docs/ .md$ LICENSE .mergify.yml .github .gitignore .commitlintrc.yml)

function check_file_present() {
    local file=$1
    for FILE in "${FILES[@]}"; do
        if [[ $file =~ $FILE ]]; then
            return 0
        fi
    done
    return 1
}

for CHANGED_FILE in $CHANGED_FILES; do
    if ! check_file_present "$CHANGED_FILE"; then
        skip=1
    fi
done
if [ $skip -eq 0 ]; then
    echo "doc change only"
    exit 1
fi
