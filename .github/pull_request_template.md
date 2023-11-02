<!-- Please take a look at our [Contributing](https://github.com/ceph/ceph-csi/blob/devel/docs/development-guide.md#Code-contribution-workflow)
documentation before submitting a Pull Request!
Thank you for contributing to ceph-csi! -->

# Describe what this PR does #

Provide some context for the reviewer

## Is there anything that requires special attention ##

Do you have any questions?

Is the change backward compatible?

Are there concerns around backward compatibility?

Provide any external context for the change, if any.

For example:

* Kubernetes links that explain why the change is required
* CSI spec related changes/catch-up that necessitates this patch
* golang related practices that necessitates this change

## Related issues ##

Mention any github issues relevant to this PR. Adding below line
will help to auto close the issue once the PR is merged.

Fixes: #issue_number

## Future concerns ##

List items that are not part of the PR and do not impact it's
functionality, but are work items that can be taken up subsequently.

**Checklist:**

* [ ] **Commit Message Formatting**: Commit titles and messages follow
  guidelines in the [developer
  guide](https://github.com/ceph/ceph-csi/blob/devel/docs/development-guide.md#commit-messages).
* [ ] Reviewed the developer guide on [Submitting a Pull
  Request](https://github.com/ceph/ceph-csi/blob/devel/docs/development-guide.md#development-workflow)
* [ ] [Pending release
  notes](https://github.com/ceph/ceph-csi/blob/devel/PendingReleaseNotes.md)
  updated with breaking and/or notable changes for the next major release.
* [ ] Documentation has been updated, if necessary.
* [ ] Unit tests have been added, if necessary.
* [ ] Integration tests have been added, if necessary.

---

<details>
<summary>Show available bot commands</summary>

These commands are normally not required, but in case of issues, leave any of
the following bot commands in an otherwise empty comment in this PR:

* `/retest ci/centos/<job-name>`: retest the `<job-name>` after unrelated
  failure (please report the failure too!)

</details>
