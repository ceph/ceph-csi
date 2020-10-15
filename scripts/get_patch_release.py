#!/usr/bin/python
'''
Fetches the Kubernetes releases from GitHub and returns the most recent patch
release for a major version (like 1.19). In case the version that is passed
contains the patch release already, the latest patch release will not be
detected, but the passed version will be returned (enables forcing a particular
version in CI jobs).

Parameters:
 --version=<version>: the major version to find the latest patch release for, i.e. v1.19

Environment:
 GITHUB_API_TOKEN: the GitHub "personal access token" to use
'''

import argparse
import os
import requests
from requests.auth import HTTPBasicAuth
import sys

RELEASE_URL = 'https://api.github.com/repos/kubernetes/kubernetes/releases'
'''
URL for fetching the releases. Add '?per_num=50' to increase the number of
returned releases from default 30 to 50 (max 100).
'''


def log_rate_limit(res):
    limit = -1
    if 'X-Ratelimit-Limit' in res.headers:
        limit = int(res.headers['X-Ratelimit-Limit'])

    remaining = -1
    if 'X-Ratelimit-Remaining' in res.headers:
        remaining = int(res.headers['X-Ratelimit-Remaining'])

    used = -1
    if 'X-Ratelimit-Used' in res.headers:
        used = int(res.headers['X-Ratelimit-Used'])

    print('Rate limit (limit/used/remaining): %d/%d/%d' % (limit, used, remaining))


def get_github_auth():
    auth = None

    if 'GITHUB_API_TOKEN' in os.environ:
        github_api_token = os.environ['GITHUB_API_TOKEN']
        if github_api_token != '':
            # the username "unused" is not relevant, needs to be non-empty
            auth = HTTPBasicAuth('unused', github_api_token)

    return auth


def get_json_releases():
    '''
    Fetch the releases from GitHub, return the full JSON structures that were
    obtained.
    '''
    headers = {'Accept': 'application/vnd.github.v3+json'}
    res = requests.get(RELEASE_URL, headers=headers, auth=get_github_auth())

    if res.status_code == 403:
        log_rate_limit(res)

    # if "res.status_code != requests.codes.ok", raise an exception
    res.raise_for_status()

    return res.json()


def get_releases(gh_releases):
    '''
    Take the JSON formatted releases, and return a list of the name for each label.
    '''
    releases = list()
    for release in gh_releases:
        releases.append(release['name'])
    return releases


def main():
    '''
    main() function to parse arguments and run the actions.
    '''
    parser = argparse.ArgumentParser()
    parser.add_argument('--version', help='major version to find patch release for')
    args = parser.parse_args()

    # get all the releases
    try:
        json = get_json_releases()
    except Exception as err:
        print('Error: %s' % err)
        sys.exit(1)

    releases = get_releases(json)

    # in case --version is passed, exit with 0 or 1
    if args.version:
        version = args.version
        if not version.startswith('v'):
            version = 'v' + version

        # when version already contains the patch release, do not run any
        # detection
        if len(version.split('.')) >= 3:
            print(version)
            sys.exit(0)

        # releases are ordered from newest to oldest, so the 1st match is the
        # most current patch update
        for release in releases:
            if release.startswith(version + '.'):
                print(release)
                sys.exit(0)

        # no match, exit with an error
        sys.exit(1)
    # --version was not passed, list all releases
    else:
        for release in releases:
            print(release)

    sys.exit(0)

if __name__ == '__main__':
    main()
