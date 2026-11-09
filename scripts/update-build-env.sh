#!/bin/bash
#
# Check the linter/tool versions in build.env against the latest upstream
# release, update build.env and create one commit per updated component.
#
# Usage: scripts/update-build-env.sh [-n|--dry-run]
#
#   -n, --dry-run   only report available updates, do not modify anything
#
# Requires: git (with user.name/user.email configured), curl, sort -V.

set -o errexit -o pipefail -o nounset

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
BUILD_ENV="${SCRIPT_DIR}/../build.env"

DRY_RUN=0
case "${1:-}" in
-n | --dry-run) DRY_RUN=1 ;;
"") ;;
*)
    echo "usage: $0 [-n|--dry-run]" >&2
    exit 1
    ;;
esac

# variable|git repository|tag prefix|description (used in the commit message)
#   [|value prefix|branch variable]
#
# The optional fields are for variables that do not contain just the version:
#   value prefix    text in front of the version in build.env
#   branch variable ceph/ceph only: the version is the highest major release
#                   (e.g. v21) with a release branch that is marked "stable"
#                   in src/ceph_release, and this variable is set to the name
#                   of that release branch (codename)
COMPONENTS=(
    "GOLANGCI_VERSION|https://github.com/golangci/golangci-lint|v|golangci-lint"
    "GOPLS_VERSION|https://github.com/golang/tools|gopls/v|gopls"
    "LYCHEE_VERSION|https://github.com/lycheeverse/lychee|lychee-v|lychee"
    "ROOK_VERSION|https://github.com/rook/rook|v|Rook"
    "ROOK_CEPH_CLUSTER_IMAGE|https://github.com/ceph/ceph|v|Ceph|quay.io/ceph/ceph:|CEPH_VERSION"
)

# releases <repository> <tag prefix>
# prints the versions (no pre-release suffix), including the leading "v", for
# tags matching <tag prefix>X.Y.Z, highest first
releases() {
    git ls-remote --tags --refs "${1}" "refs/tags/${2}*" |
        sed -n "s|.*refs/tags/${2}\\([0-9][0-9.]*\\)\$|v\\1|p" |
        sort -rV
}

# latest_release <repository> <tag prefix>
latest_release() {
    releases "$@" | sed -n 1p
}

# latest_ceph_release <repository> <tag prefix>
# prints the highest major version (e.g. v21) of which the release branch is
# marked "stable" in src/ceph_release, followed by the name of that release
# branch (codename) on the next line
latest_ceph_release() {
    local raw="https://raw.githubusercontent.com/${1#https://github.com/}"
    local seen=" " v major codename heads status all
    # a failing request is an error, not a reason to fall back to an older release
    all="$(releases "$@")" || return 1
    for v in ${all}; do
        major="${v%%.*}"
        # the highest tag of a major is checked first, skip the others
        case "${seen}" in *" ${major} "*) continue ;; esac
        seen+="${major} "

        codename="$(curl -fsS "${raw}/${v}/src/ceph_release" | sed -n 2p)" || return 1
        # a release under development does not have a release branch yet
        heads="$(git ls-remote --heads "${1}" "refs/heads/${codename}")" || return 1
        [ -n "${heads}" ] || continue

        status="$(curl -fsS "${raw}/${codename}/src/ceph_release" | sed -n 3p)" || return 1
        if [ "${status}" = "stable" ]; then
            echo "${major}"
            echo "${codename}"
            return 0
        fi
    done
    return 1
}

# newer <current> <candidate>: succeeds when candidate is a higher version
newer() {
    [ "${1}" != "${2}" ] && [ "$(printf '%s\n%s\n' "${1}" "${2}" | sort -V | tail -n1)" = "${2}" ]
}

if [ "${DRY_RUN}" -eq 0 ] && [ -n "$(git status --porcelain --untracked-files=no)" ]; then
    echo "working tree is not clean, commit or stash your changes first" >&2
    exit 1
fi

rc=0
for entry in "${COMPONENTS[@]}"; do
    IFS='|' read -r var repo prefix name vprefix branchvar <<<"${entry}"

    current="$(sed -n "s/^${var}=//p" "${BUILD_ENV}")"
    current="${current#"${vprefix}"}"
    if [ -z "${current}" ]; then
        echo "${var}: not found in build.env" >&2
        rc=1
        continue
    fi

    # a failing lookup is reported below, it should not abort the other components
    latest=""
    branch=""
    if [ -n "${branchvar}" ]; then
        { read -r latest && read -r branch; } < <(latest_ceph_release "${repo}" "${prefix}") ||
            { latest=""; branch=""; }
    else
        latest="$(latest_release "${repo}" "${prefix}")" || latest=""
    fi
    if [ -z "${latest}" ] || { [ -n "${branchvar}" ] && [ -z "${branch}" ]; }; then
        echo "${var}: unable to determine latest release of ${repo}" >&2
        rc=1
        continue
    fi

    if ! newer "${current}" "${latest}"; then
        echo "${var}: ${current} is up to date"
        continue
    fi

    echo "${var}: ${current} -> ${latest}${branch:+ (${branchvar}=${branch})}"
    [ "${DRY_RUN}" -eq 0 ] || continue

    sed -i "s|^${var}=.*|${var}=${vprefix}${latest}|" "${BUILD_ENV}"
    [ -z "${branch}" ] || sed -i "s|^${branchvar}=.*|${branchvar}=${branch}|" "${BUILD_ENV}"
    git add "${BUILD_ENV}"
    git commit -s -q -F - <<EOF
build: update ${name} to version ${latest#v}

Update ${name} from ${current} to the latest release ${latest}.${branch:+
Set ${branchvar} to ${branch}, the release branch of ${latest}.}
EOF
done

exit "${rc}"
