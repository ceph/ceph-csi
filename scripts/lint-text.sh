#! /bin/bash
# vim: set ts=4 sw=4 et :

# Usage: pre-commit.sh [--require-all]
#   --require-all  Fail instead of warn if a checker is not found

set -e

# Run checks from root of the repo
scriptdir="$(dirname "$(realpath "$0")")"
cd "$scriptdir/.."

# run_check <file_regex> <checker_exe> [optional args to checker...]
function run_check() {
    regex="$1"
    shift
    exe="$1"
    shift

    if [ -x "$(command -v "$exe")" ]; then
        find . -path ./vendor -prune -o -regextype egrep -iregex "$regex" -print0 |
            xargs -0rt -n1 "$exe" "$@"
    elif [ "$all_required" -eq 0 ]; then
        echo "Warning: $exe not found... skipping some tests."
    else
        echo "FAILED: All checks required, but $exe not found!"
        exit 1
    fi
}

all_required=0
if [ "x$1" == "x--require-all" ]; then
    all_required=1
fi

# markdownlint: https://github.com/markdownlint/markdownlint
# https://github.com/markdownlint/markdownlint/blob/master/docs/RULES.md
# Install via: gem install mdl
run_check '.*\.md' mdl --style scripts/mdl-style.rb

# Install via: dnf install shellcheck
run_check '.*\.(ba)?sh' shellcheck
run_check '.*\.(ba)?sh' bash -n

# Install via: pip install yamllint
# disable yamlint chekck for helm chats
run_check '.*\.ya?ml' yamllint -s -d "{extends: default, rules: {line-length: {allow-non-breakable-inline-mappings: true}},ignore: deploy/rbd/helm/templates/*.yaml}"

echo "ALL OK."
