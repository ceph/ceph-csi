# Coding Conventions

Please follow coding conventions and guidelines described in the following documents:

* [Go proverbs](https://go-proverbs.github.io/) - highly recommended read
* [CodeReviewComments](https://github.com/golang/go/wiki/CodeReviewComments)
* [Effective Go](https://golang.org/doc/effective_go.html)
* [How to Write a Git Commit Message](https://chris.beams.io/posts/git-commit/)

Here's a list of some more specific conventions that are often followed in
the code and will be pointed out in the review process:

## General

* Keep variable names short for variables that are local to the function.
* Do not export a function or variable name outside the package until you
  have an external consumer for it.

### Imports

We use the following convention for specifying imports:

```
<import standard library packages>

<import ceph-csi packages>

<import third-party packages>
```

Example:

```go
import (
 "os"
 "path"
 "strings"
 "time"

 "github.com/ceph/ceph-csi/internal/util"

 "github.com/pborman/uuid"
)
```

### Error Handling

* Use variable name `err` to denote error variable during a function call.
* Reuse the previously declared `err` variable as long as it is in scope.
  For example, do not use `errWrite` or `errRead`.
* Do not panic() for errors that can be bubbled up back to user. Use panic()
  only for fatal errors which shouldn't occur.
* Do not ignore errors using `_` variable unless you know what you're doing.
* Error strings should not start with a capital letter.
* If error requires passing of extra information, you can define a new type
* Error types should end with `Error`.

### Logging

* The inner-most utility functions should never log. Logging must almost always
  be done by the caller on receiving an `error`.
* Always use log level `DEBUG` to provide useful **diagnostic information** to
  developers or sysadmins.
* Use log level `INFO` to provide information to users or sysadmins. This is the
  kind of information you'd like to log in an out-of-the-box configuration in
  happy scenario.
* Use log level `WARN` when something fails but there's a workaround or fallback
  or retry for it and/or is fully recoverable.
* Use log level `ERROR` when something occurs which is fatal to the operation,
  but not to the service or application.

### Wrap long lines

At present, we restrict the number of chars in a line to `120` which is the
default value for the `lll` linter check we have in CI. If your source code line
or comment goes beyond this limit please try to organize it in such a way it
is within the line length limit and help on code reading.

Example:

```
_, err := framework.RunKubectl(cephCSINamespace, "delete", "cm", "ceph-csi-encryption-kms-config", "--namespace", cephCSINamespace, "--ignore-not-found=true")
```

Instead of above long function signature, we can organize it to something like below
which is clear and help on easy code reading.

```
_, err := framework.RunKubectl(
            cephCSINamespace,
            "delete",
            "cm",
            "ceph-csi-encryption-kms-config",
            "--namespace",
            cephCSINamespace,
            "--ignore-not-found=true")
```

### Mark Down Rules

* MD014 - Dollar signs used before commands without showing output

  The dollar signs are unnecessary, it is easier to copy and paste and
  less noisy if the dollar signs are omitted. Especially when the
  command doesn't list the output, but if the command follows output
  we can use '$ ' (dollar+space) mainly to differentiate between
  command and its output.

  scenario 1: when command doesn't follow output

  ```console
  cd ~/work
  ```

  scenario 2: when command follow output (use dollar+space)

  ```console
  $ ls ~/work
  file1 file2 dir1 dir2 ...
  ```
