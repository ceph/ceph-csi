#!/bin/bash
# vim: set ts=4 sw=4 et :

# This script will be used to lint non-go files
# Usage: ./scripts/lint-extras.sh <command>
# Available commands are [lint-shell lint-yaml lint-markdown lint-helmlint-all ]
set -e

# Run checks from root of the repo
scriptdir="$(dirname "$(realpath "$0")")"
cd "$scriptdir/.."

# run_check <file_regex> <checker_exe> [optional args to checker...]
# Pass empty regex when no regex is needed
function run_check() {
    local regex="$1"
    shift
    local exe="$1"
    shift

    if [ -x "$(command -v "${exe}")" ]; then
        if [ -z "${regex}" ]; then
            "$exe" "$@"
        else
            find . -path "*/vendor" -prune -o -regextype egrep -iregex "$regex" -print0 |
                xargs -0rt -n1 "$exe" "$@"
        fi
    elif [ "$all_required" -eq 0 ]; then
        echo "Warning: $exe not found... skipping some tests."
    else
        echo "FAILED: All checks required, but $exe not found!"
        exit 1
    fi
}

all_required=0

function lint_markdown() {
    # markdownlint: https://github.com/markdownlint/markdownlint
    # https://github.com/markdownlint/markdownlint/blob/master/docs/RULES.md
    # Install via: gem install mdl
    run_check '.*\.md' mdl --style scripts/mdl-style.rb
}

function lint_shell() {
    # Install via: dnf install shellcheck
    run_check '.*\.(ba)?sh' shellcheck --external-sources
    run_check '.*\.(ba)?sh' bash -n
}

function lint_yaml() {
    # Install via: pip install yamllint
    # disable yamlint check for helm charts
    run_check '.*\.ya?ml' yamllint -s -d "{extends: default, rules: {line-length: {allow-non-breakable-inline-mappings: true}},ignore: charts/*/templates/*.yaml}"
}

function lint_helm() {
    # Install via: https://github.com/helm/helm/blob/master/docs/install.md
    run_check '' helm lint --namespace=test charts/*
}

function lint_py() {
    # Install via: sudo apt-get install python3-pylint
    run_check '.*\.py' pylint --score n --output-format=colorized
}

function lint_all() {
    # runs all checks
    all_required=1
    lint_shell
    lint_yaml
    lint_markdown
    lint_helm
    lint_py
}
case "${1:-}" in
lint-shell)
    lint_shell
    ;;
lint-yaml)
    lint_yaml
    ;;
lint-markdown)
    lint_markdown
    ;;
lint-helm)
    lint_helm
    ;;
lint-py)
    lint_py
    ;;
lint-all)
    lint_all
    ;;
*)
    echo " $0 [command]
Available Commands:
  lint-shell             Lint shell files
  lint-yaml              Lint yaml files
  lint-markdown          Lint markdown files
  lint-helm              Lint helm charts
  lint-py                Lint python files
  lint-all               Run lint on all non-go files
" >&2
    ;;
esac
