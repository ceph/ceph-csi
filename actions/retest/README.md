# retest-action

This is a github action built using the golang and the [github
api](https://github.com/google/go-github). The main idea behind this one is to retest
the failed tests on the approved PR's to avoid burden on the
maintainer's/author's to retest all the failed tests.

* List the pull requests from the  github organization.
* Check PR is open and have required approvals.
* Check PR has the required label to continue to retest.
* Pulls the failed test details.
* Check failed test has reached the maximum limit.
* If the limit has not reached, the action will post the `retest` command on the
  PR with log location for further debugging.
* If the limit has reached, the Pull Request will be skipped.
