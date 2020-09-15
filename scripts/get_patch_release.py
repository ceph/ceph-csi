#!/usr/bin/python
'''
Fetches the Kubernetes releases from GitHub and returns the most recent patch
release for a major version.

Parameters:
 --version=<version>: the major version to find the latest patch release for, i.e. v1.19
'''

import argparse
import sys
import requests

RELEASE_URL = 'https://api.github.com/repos/kubernetes/kubernetes/releases'
'''
URL for fetching the releases. Add '?per_num=50' to increase the number of
returned releases from default 30 to 50 (max 100).
'''


def get_json_releases():
    '''
    Fetch the releases from GitHub, return the full JSON structures that were
    obtained.
    '''
    headers = {'Accept': 'application/vnd.github.v3+json'}
    res = requests.get(RELEASE_URL, headers=headers)
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
    json = get_json_releases()
    releases = get_releases(json)

    # in case --version is passed, exit with 0 or 1
    if args.version:
        version = args.version
        if not version.startswith('v'):
            version = 'v' + version

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
