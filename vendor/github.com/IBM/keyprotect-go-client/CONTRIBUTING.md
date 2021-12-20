# Contributing to keyprotect-go-client

`keyprotect-go-client` is open for code perusal and contributions. We welcome contributions in the form of feedback, bugs, or patches.

## Bugs and Feature Requests

If you find something that does not work as expected or would like to see a new feature added, 
please open a [Github Issue](https://github.com/IBM/keyprotect-go-client/issues)

## Pull Requests

For your pull request to be merged, it must meet the criteria of a "correct patch", and also
be fully reviewed and approved by two Maintainer level contributors.

A correct patch is defined as the following:

 - If the patch fixes a bug, it must be the simplest way to fix the issue
 - Your patch must come with unit tests
 - Unit tests (CI job) must pass
 - New feature function should have integration tests as well


# Development

## Compiling the package

```sh
go build ./...
```

The client relies on go modules to pull in required dependencies at build time.

https://github.com/golang/go/wiki/Modules#how-to-use-modules

## Running the test cases

Using `go test`

```sh
go test -v -race ./...
```

The test cases are also runnable through `make`

```sh
make test
# or
make test-integration
```
