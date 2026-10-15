#!/bin/bash
#
# Check the linter/tool versions in build.env against the latest upstream
# release, update build.env and create one commit per updated component.
#
# Usage: scripts/update-build-env.sh [-n|--dry-run]
#
#   -n, --dry-run   only report available updates, do not modify anything
#
# Requires: git (with user.name/user.email configured), sort -V.

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
COMPONENTS=(
    "GOLANGCI_VERSION|https://github.com/golangci/golangci-lint|v|golangci-lint"
    "GOPLS_VERSION|https://github.com/golang/tools|gopls/v|gopls"
    "LYCHEE_VERSION|https://github.com/lycheeverse/lychee|lychee-v|lychee"
)

# latest_release <repository> <tag prefix>
# prints the highest stable (no pre-release suffix) version, including the
# leading "v", for tags matching <tag prefix>X.Y.Z
latest_release() {
    git ls-remote --tags --refs "${1}" "refs/tags/${2}*" |
        sed -n "s|.*refs/tags/${2}\\([0-9][0-9.]*\\)\$|v\\1|p" |
        sort -V |
        tail -n1
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
    IFS='|' read -r var repo prefix name <<<"${entry}"

    current="$(sed -n "s/^${var}=//p" "${BUILD_ENV}")"
    if [ -z "${current}" ]; then
        echo "${var}: not found in build.env" >&2
        rc=1
        continue
    fi

    # a failing lookup is reported below, it should not abort the other components
    latest="$(latest_release "${repo}" "${prefix}")" || latest=""
    if [ -z "${latest}" ]; then
        echo "${var}: unable to determine latest release of ${repo}" >&2
        rc=1
        continue
    fi

    if ! newer "${current}" "${latest}"; then
        echo "${var}: ${current} is up to date"
        continue
    fi

    echo "${var}: ${current} -> ${latest}"
    [ "${DRY_RUN}" -eq 0 ] || continue

    sed -i "s/^${var}=.*/${var}=${latest}/" "${BUILD_ENV}"
    git add "${BUILD_ENV}"
    git commit -s -q -F - <<EOF
build: update ${name} to version ${latest#v}

Update ${name} from ${current} to the latest release ${latest}.
EOF
done

exit "${rc}"
