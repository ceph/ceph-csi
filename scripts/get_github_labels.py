#!/usr/bin/python
'''
Fetches the labels of an Issue or Pull-Request from GitHub.

Parameters:
 --id=<id>: the number of the the GitHub Issue or Pull-Request
 --has-label=<label>: label to check (exit 0 if set, 2 unset), without
                      --has-label, all labels of the Issue or Pull-Request
                      get printed.

Exit codes:
 0: success
 1: any unexpected failure
 2: --has-label=<label> was passed, <label> is not set on the PR

Environment:
 GITHUB_API_TOKEN: the GitHub "personal access token" to use
'''

import argparse
import os
import requests
from requests.auth import HTTPBasicAuth
import sys

LABEL_URL_FMT = 'https://api.github.com/repos/ceph/ceph-csi/issues/%s/labels'
'''
Formatted URL for fetching the labels, replace '%s' with the number of  the
Issue or Pull-Request.
'''


def get_json_labels(gh_id):
    '''
    Fetch the labels from GitHub, return the full JSON structures that were
    obtained. These include several ID's, color, decription, name and more.
    '''
    url = LABEL_URL_FMT % gh_id
    headers = {'Accept': 'application/vnd.github.v3+json'}

    auth = None
    if 'GITHUB_API_TOKEN' in os.environ:
        github_api_token = os.environ['GITHUB_API_TOKEN']
        if github_api_token != '':
            # the username "unused" is not relevant, needs to be non-empty
            auth = HTTPBasicAuth('unused', github_api_token)

    res = requests.get(url, headers=headers, auth=auth)

    # if "res.status_code != requests.codes.ok", raise an exception
    res.raise_for_status()

    return res.json()


def get_names(gh_labels):
    '''
    Take the JSON formatted labels, and return a list of the name for each label.
    '''
    names = list()
    for label in gh_labels:
        names.append(label['name'])
    return names


def main():
    '''
    main() function to parse arguments and run the actions.
    '''
    parser = argparse.ArgumentParser()
    parser.add_argument('--id', type=int, required=True, help='ID of the Issue or Pull-Request')
    parser.add_argument('--has-label', help='check if the Issue or Pull-Request has the label set')
    args = parser.parse_args()

    # get the labels for the issue
    try:
        json = get_json_labels(args.id)
    except Exception as err:
        print('Error: %s' % err)
        sys.exit(1)

    names = get_names(json)

    # in case --has-label is passed, exit with 0 or 2
    if args.has_label:
        if args.has_label in names:
            sys.exit(0)
        else:
            sys.exit(2)
    # --has-label was not passed, list all labels
    else:
        for name in names:
            print(name)

    sys.exit(0)

if __name__ == '__main__':
    main()
