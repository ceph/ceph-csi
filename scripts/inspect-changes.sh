#!/bin/bash
# vim: set ts=4 sw=4 et :
#
# inspect-changes.sh — inspect changed files between two git trees and
# identify whether they touch a specific backend (cephfs, rbd, nfs, nvmeof)
# or shared code.
#
# Uses a three-dot diff (<since>...<until>) so that only the files actually
# changed by the PR are considered, regardless of shallow-clone depth.
# Documentation files are always ignored when checking backend/shared changes.
#
# Usage:
#   scripts/inspect-changes.sh [--backend=<backend>] [--repo=<path>] [--doc-change-only] <since> [<until>]
#
# Arguments:
#   <since>             git ref for the base of the range, e.g. "origin/devel"
#                       or a commit SHA.  Used as the left side of a three-dot
#                       diff: files changed relative to the merge base.
#   <until>             git ref for the tip of the range (default: HEAD).
#
# Options:
#   --backend=<name>    filter to a single backend: cephfs, rbd, nfs, nvmeof
#                       When omitted, all backends and shared code are reported.
#   --repo=<path>       path to the ceph-csi git repository to inspect.
#                       Defaults to the current working directory.
#   --files             show the list of changed files for each matched category
#   --doc-change-only   exit 0 if every changed file is a documentation file,
#                       exit 1 if any non-documentation file was changed.
#                       Cannot be combined with --backend.
#   -h, --help          print this help text and exit
#
# Exit codes:
#   0   one or more changed files touch the requested backend / shared code
#         (or, with --doc-change-only: all changes are documentation-only)
#   1   no changed files match the requested backend / shared code
#         (or, with --doc-change-only: at least one non-documentation file changed)
#   2   usage error
#

set -e -o pipefail

# ---------------------------------------------------------------------------
# backend → file-path prefixes (relative to the repo root).
# Any file not matched by any backend path is considered shared code, and
# counts as a change to every backend.
# ---------------------------------------------------------------------------

declare -A BACKEND_PATHS=(
    [cephfs]="internal/cephfs/ internal/csi-addons/cephfs/"
    [rbd]="internal/rbd/ internal/csi-addons/rbd/"
    [nfs]="internal/nfs/ internal/csi-addons/nfs/ internal/cephfs/"
    [nvmeof]="internal/nvmeof/ internal/csi-addons/nvmeof/ internal/rbd/"
)

# ---------------------------------------------------------------------------
# documentation file patterns — matched against each changed file path.
# A file is considered documentation-only if it matches any of these.
# ---------------------------------------------------------------------------
declare -a DOC_PATTERNS=(
    "^docs/"
    "\.md$"
    "^LICENSE$"
    "^\.mergify\.yml$"
    "^\.github/"
    "^\.gitignore$"
    "^\.commitlintrc\.yml$"
)

# ---------------------------------------------------------------------------
# defaults
# ---------------------------------------------------------------------------
REPO="."
BACKEND=""
SHOW_FILES=0
DOC_CHANGE_ONLY=0
GIT_SINCE=""
GIT_UNTIL="HEAD"

# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------
usage() {
    sed -n '3,/^$/p' "$0" | sed 's/^# \{0,1\}//'
    exit "${1:-0}"
}

log_info()  { echo "[INFO]  $*" >&2; }
log_warn()  { echo "[WARN]  $*" >&2; }
log_error() { echo "[ERROR] $*" >&2; }

# path_matches_prefixes <file> <prefix1> [prefix2 ...]
# Returns 0 if $file starts with any of the given prefixes.
path_matches_prefixes() {
    local file="$1"
    shift
    local prefix
    for prefix in "$@"; do
        if [[ "${file}" == "${prefix}"* ]]; then
            return 0
        fi
    done
    return 1
}

# is_doc_file <file>
# Returns 0 if the file matches any documentation pattern.
is_doc_file() {
    local file="$1"
    local pattern
    for pattern in "${DOC_PATTERNS[@]}"; do
        if [[ "${file}" =~ ${pattern} ]]; then
            return 0
        fi
    done
    return 1
}

# ---------------------------------------------------------------------------
# argument parsing
# ---------------------------------------------------------------------------
PARSED=$(getopt \
    --longoptions "backend:,repo:,files,doc-change-only,help" \
    --options "h" \
    --name "$(basename "$0")" \
    -- "$@") || { usage 2; }

eval set -- "${PARSED}"

while true; do
    case "$1" in
    --backend)
        shift
        BACKEND="$1"
        ;;
    --repo)
        shift
        REPO="$1"
        ;;
    --files)
        SHOW_FILES=1
        ;;
    --doc-change-only)
        DOC_CHANGE_ONLY=1
        ;;
    -h | --help)
        usage 0
        ;;
    --)
        shift
        break
        ;;
    esac
    shift
done

GIT_SINCE="${1:-}"
GIT_UNTIL="${2:-HEAD}"

if [[ -z "${GIT_SINCE}" ]]; then
    log_error "<since> argument is required."
    usage 2
fi

# Validate backend choice
if [[ -n "${BACKEND}" ]]; then
    if [[ -z "${BACKEND_PATHS[${BACKEND}]+set}" ]]; then
        log_error "Unknown backend '${BACKEND}'. Valid choices: ${!BACKEND_PATHS[*]}"
        exit 2
    fi
fi

# --doc-change-only is incompatible with --backend
if [[ "${DOC_CHANGE_ONLY}" -eq 1 && -n "${BACKEND}" ]]; then
    log_error "--doc-change-only cannot be combined with --backend."
    exit 2
fi

# Validate the repo
if [[ ! -d "${REPO}/.git" ]]; then
    log_error "'${REPO}' does not appear to be a git repository."
    exit 2
fi

# ---------------------------------------------------------------------------
# collect changed files via three-dot diff
#
# The three-dot syntax (<since>...<until>) diffs from the merge base of the
# two refs, so only files actually introduced by the PR branch are listed —
# unaffected by shallow-clone depth or pre-PR history on the base branch.
# ---------------------------------------------------------------------------
log_info "Inspecting changes: ${GIT_SINCE}...${GIT_UNTIL}  (repo: ${REPO})"

if ! CHANGED_FILES_RAW=$(git -C "${REPO}" diff --no-renames --name-only "${GIT_SINCE}...${GIT_UNTIL}" 2>&1); then
    log_error "git diff failed: ${CHANGED_FILES_RAW}"
    exit 2
fi

mapfile -t CHANGED_FILES < <(printf '%s\n' "${CHANGED_FILES_RAW}" | grep -v '^$' || true)

if [[ ${#CHANGED_FILES[@]} -eq 0 ]]; then
    log_warn "No changed files found in ${GIT_SINCE}...${GIT_UNTIL}."
    exit 1
fi

log_info "Total changed files: ${#CHANGED_FILES[@]}"

# ---------------------------------------------------------------------------
# --doc-change-only: exit 0 if every changed file is a documentation file
# ---------------------------------------------------------------------------
if [[ "${DOC_CHANGE_ONLY}" -eq 1 ]]; then
    for f in "${CHANGED_FILES[@]}"; do
        if ! is_doc_file "${f}"; then
            log_info "Non-documentation file changed: ${f}"
            exit 1
        fi
    done
    log_info "All changed files are documentation — skipping."
    exit 0
fi

# ---------------------------------------------------------------------------
# build the set of backends to check
# ---------------------------------------------------------------------------
if [[ -n "${BACKEND}" ]]; then
    CHECK_BACKENDS=("${BACKEND}")
else
    CHECK_BACKENDS=("${!BACKEND_PATHS[@]}")
fi

# Pre-compute the flat list of all backend prefixes (for shared-code detection)
all_backend_prefixes=()
for _b in "${!BACKEND_PATHS[@]}"; do
    read -r -a _p <<< "${BACKEND_PATHS[${_b}]}"
    all_backend_prefixes+=("${_p[@]}")
done

# ---------------------------------------------------------------------------
# classify changed files
# ---------------------------------------------------------------------------

declare -A BACKEND_FILES   # backend → newline-separated matched file list
declare -a SHARED_FILES

for backend in "${CHECK_BACKENDS[@]}"; do
    BACKEND_FILES[${backend}]=""
done

for f in "${CHANGED_FILES[@]}"; do
    # documentation files are ignored for backend/shared classification
    if is_doc_file "${f}"; then
        log_info "Ignoring documentation file: ${f}"
        continue
    fi

    if ! path_matches_prefixes "${f}" "${all_backend_prefixes[@]}"; then
        # shared file: counts as a change to every backend
        SHARED_FILES+=("${f}")
        for backend in "${CHECK_BACKENDS[@]}"; do
            BACKEND_FILES[${backend}]+="${f}"$'\n'
        done
    else
        # backend-specific file: attribute to the matching backend(s) only
        for backend in "${CHECK_BACKENDS[@]}"; do
            read -r -a path_prefixes <<< "${BACKEND_PATHS[${backend}]}"
            if path_matches_prefixes "${f}" "${path_prefixes[@]}"; then
                BACKEND_FILES[${backend}]+="${f}"$'\n'
            fi
        done
    fi
done

# ---------------------------------------------------------------------------
# report results
# ---------------------------------------------------------------------------
overall_match=0

print_files() {
    local label="$1"
    local files="$2"   # newline-separated string, or empty for array fallback
    shift 2

    echo ""
    echo "  ${label}:"
    if [[ -n "${files}" ]]; then
        while IFS= read -r line; do
            [[ -z "${line}" ]] && continue
            echo "    ${line}"
        done <<< "${files}"
    else
        local f
        for f in "$@"; do
            echo "    ${f}"
        done
    fi
}

echo ""
echo "=== Change inspection: ${GIT_SINCE}...${GIT_UNTIL} ==="

# backends
for backend in "${CHECK_BACKENDS[@]}"; do
    files="${BACKEND_FILES[${backend}]}"
    # count non-empty lines
    count=$(echo -n "${files}" | grep -c . || true)
    if [[ "${count}" -gt 0 ]]; then
        overall_match=1
        if [[ "${SHOW_FILES}" -eq 1 ]]; then
            print_files "backend: ${backend} (${count} file(s))" "${files}"
        else
            echo "  backend ${backend}: ${count} file(s) matched"
        fi
    else
        echo "  backend ${backend}: no matching files"
    fi
done

# shared (only shown when no specific backend requested, to avoid redundancy)
if [[ -z "${BACKEND}" ]]; then
    count=${#SHARED_FILES[@]}
    if [[ "${count}" -gt 0 ]]; then
        overall_match=1
        if [[ "${SHOW_FILES}" -eq 1 ]]; then
            print_files "shared code (${count} file(s))" "" "${SHARED_FILES[@]}"
        else
            echo "  shared code: ${count} file(s) matched"
        fi
    else
        echo "  shared code: no matching files"
    fi
fi

echo ""

exit $((1 - overall_match))
