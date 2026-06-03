#!/bin/bash -e
CHANGED_FILES=$(git diff --name-only "$TRAVIS_COMMIT_RANGE")

[[ -z $CHANGED_FILES ]] && exit 1

skip=0
#files to be skipped
declare -a FILES=(^docs/ .md$ ^scripts/ LICENSE .mergify.yml .github .gitignore .commitlintrc.yml .pre-commit-config.yaml)

function check_file_present() {
    local file=$1
    for FILE in "${FILES[@]}"; do
        if [[ $file =~ $FILE ]]; then
            if [[ $file =~ (minikube.sh) ]]; then
                continue
            fi
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
    echo "Skipping functional tests"
    exit 1
fi
