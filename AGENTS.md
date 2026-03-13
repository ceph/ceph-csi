# AGENTS.md

This file provides guidance to AI agents (including Claude Code from
claude.ai/code) when working with code in this repository.

## Project Overview

Ceph-CSI implements Container Storage Interface (CSI) drivers for Ceph
storage, enabling dynamic provisioning and management of Ceph volumes in
Kubernetes. The project supports multiple storage types: RBD (block),
CephFS (filesystem), NFS, and NVMeoF, all packaged in a single binary.

## Build Commands

**Recommended:** Use `make containerized-build` for all compilation tasks.
Local builds only work when Ceph libraries and headers (`librados-devel`,
`librbd-devel`, `libcephfs-devel`) are installed on the system.

### Containerized Build (Recommended)

```bash
make containerized-build                           # Build cephcsi binary in container (output: _output/cephcsi)
make containerized-build TARGET=<target>          # Build specific target in container
make containerized-build TARGET=e2e.test          # Build e2e test binary in container
```

### Local Build (Only if Ceph libraries are available)

```bash
make                    # Build cephcsi binary locally
make cephcsi           # Same as make
make e2e.test          # Build e2e test binary locally
```

### Container Images

```bash
make image-cephcsi                                # Build container image
```

The built binary will be in `_output/` directory.

### Environment Variables

- `GO_TAGS`: Build tags (default: ceph version + ceph_preview)
- `CONTAINERIZED`: Set to "yes" for containerized builds
- `CSI_IMAGE_NAME`: Container image name (default: quay.io/cephcsi/cephcsi)
- `CSI_IMAGE_VERSION`: From build.env, default "canary"

## Testing

### Unit Tests (Containerized - Recommended)

```bash
make containerized-test                            # Run all tests in container (go-test + static-check)
make containerized-test TARGET=static-check       # Run specific test target in container
make containerized-test TARGET=go-test            # Run only Go unit tests in container
```

### Local Unit Tests

```bash
make test                                          # Run all tests locally (go-test + static-check + mod-check)
make go-test                                       # Run Go unit tests only
```

**Note:** Use `make containerized-test` for running unit tests to ensure
consistent environment.

### Static Checks

```bash
make static-check      # Run all static checks (go-lint + lint-extras + codespell)
make go-lint           # Run golangci-lint
make lint-extras       # Run shell/markdown/yaml/helm/python linters
make codespell         # Spell checking
```

### End-to-End Tests

```bash
make e2e.test                                      # Build e2e test binary
make run-e2e                                       # Run e2e tests
make run-e2e E2E_ARGS="--test-cephfs=false"       # Run with specific args
```

**Important:** E2E tests require BOTH a functional Kubernetes cluster AND a
functional Ceph cluster. Only run these tests when both are available and
properly configured. See `e2e/` directory for test implementations.

### Module Checks

```bash
make mod-check         # Verify go.mod, vendor, and go.sum are up to date
```

This runs `go mod tidy && go mod vendor && go mod verify` across all modules.

## Code Architecture

### Multi-Driver Architecture

The project uses a single binary (`cmd/cephcsi.go`) that can run as different
driver types based on the `-type` flag:

- `rbd` - RBD block storage driver (internal/rbd/)
- `cephfs` - CephFS filesystem driver (internal/cephfs/)
- `nfs` - NFS driver (internal/nfs/)
- `nvmeof` - NVMe-oF driver (internal/nvmeof/)
- `liveness` - Liveness probe server (internal/liveness/)
- `controller` - Kubernetes controller (internal/controller/)

Each driver implements the CSI spec with three main components:

- **IdentityServer**: Driver identification and capabilities
- **ControllerServer**: Volume lifecycle (create, delete, attach, detach,
  snapshot, clone)
- **NodeServer**: Node-local operations (stage, unstage, publish, unpublish,
  mount)

### Directory Structure

#### Core Implementation

- `cmd/` - Main entry point, flag parsing, driver initialization
- `internal/cephfs/` - CephFS CSI driver implementation
- `internal/rbd/` - RBD CSI driver implementation
- `internal/nfs/` - NFS CSI driver implementation
- `internal/nvmeof/` - NVMe-oF CSI driver implementation
- `internal/util/` - Shared utilities (connection, journal, encryption, KMS)
- `internal/csi-common/` - Common CSI server implementations
- `internal/journal/` - Volume journaling for idempotency
- `internal/kms/` - Key Management System integrations (Vault, AWS, Azure, etc.)

#### Testing & Deployment

- `e2e/` - End-to-end tests with Ginkgo framework
- `deploy/` - Kubernetes deployment manifests (YAML templates, Helm charts)
- `scripts/` - Build, test, and deployment scripts

#### Other

- `api/` - Separate Go module for API definitions
- `pkg/` - Exported packages (minimal, most code in internal/)
- `docs/` - Documentation

### Go Modules

The repository has 4 separate Go modules:

- `./` - Main ceph-csi module
- `e2e/` - E2E test module
- `api/` - API definitions module
- `actions/retest` - GitHub Actions module

When making dependency changes, you may need to run `make mod-check` to
update all modules.

## Development Requirements

### Prerequisites

- Go 1.25.0+ (see build.env for exact version)
- CGO must be enabled (`CGO_ENABLED=1`) - required for go-ceph bindings
- Ceph development libraries: `librados-devel`, `librbd-devel`,
  `libcephfs-devel`
- For containerized builds: podman or docker

### Code Conventions

#### Import Organization

```go
import (
    // Standard library
    "os"
    "path"

    // Third-party packages
    "github.com/pborman/uuid"

    // ceph-csi packages
    "github.com/ceph/ceph-csi/internal/util"
)
```

#### Error Handling

- Use variable name `err` for errors
- Reuse `err` in scope, don't create `errWrite`, `errRead`, etc.
- Don't ignore errors with `_` unless intentional
- Error strings should not start with capital letter
- Error types should end with `Error`

#### Logging

- Utility functions should NOT log - let callers handle errors
- Use DEBUG for diagnostic info for developers/sysadmins
- Use INFO for user/sysadmin information in normal operations
- Use WARN for failures with workarounds/retries
- Use ERROR for operation failures (not service failures)

#### Line Length

- Maximum 120 characters per line
- Break long function calls across multiple lines for readability

#### Variable Naming

- Keep variable names short for local scope
- Don't export functions/variables until needed externally

### Commit Message Format

```
<component>: <subject of the change>

<paragraph(s) with reason/description>

Assisted-by: Claude Code <noreply@anthropic.com>
Signed-off-by: Your Name <your.email@example.org>
```

**Component prefixes**: `cephfs`, `rbd`, `nfs`, `nvmeof`, `doc`, `util`,
`journal`, `helm`, `deploy`, `build`, `ci`, `e2e`, `cleanup`, `revert`

- Subject line: max 70 characters
- Body: wrap at 80 characters
- All commits require DCO sign-off (`git commit -s`)
- **Add `Assisted-by:` line when AI agents (like Claude Code) helped with
  the changes**
- Focus on "why" not "what" in the description

### Testing Guidelines

- Provide unit tests for new code
- Functional tests go in `e2e/` directory
- Set `CEPH_CSI_RUN_ALL_TESTS=1` to run tests requiring extended permissions
- CI runs containerized tests automatically on PRs

## Common Development Workflows

### Recommended: Containerized Development

For most development tasks, use containerized builds and tests to ensure a
consistent environment without needing to install Ceph development libraries
locally:

```bash
# Build and test in one go
make containerized-build && make containerized-test
```

### Running a Single Test

```bash
# Unit test for a specific package
go test -v github.com/ceph/ceph-csi/internal/rbd

# With build tags (important for ceph integration)
go test -tags=tentacle,ceph_preview -v ./internal/rbd/
```

### Working with Multiple Drivers

The codebase shares common infrastructure (journal, KMS, utilities) across
drivers. When modifying shared code in `internal/util/`, consider impact on
all driver types (RBD, CephFS, NFS, NVMeoF).

### Deployment Manifests

Deployment YAMLs in `deploy/` are generated from templates. To regenerate:

```bash
make generate-deploy
```

## Key Files

- `build.env` - Version specifications for builds and dependencies
- `Makefile` - Primary build and test automation
- `cmd/cephcsi.go` - Main entry point for all driver types
- `scripts/test-go.sh` - Go test runner with coverage support
- `scripts/golangci.yml` - Linter configuration (generated from golangci.yml.in)
- `.pre-commit-config.yaml` - Pre-commit hooks configuration

## Documentation

- Development guide: `docs/development-guide.md`
- Coding conventions: `docs/coding.md`
- RBD deployment: `docs/rbd/deploy.md`
- CephFS deployment: `docs/cephfs/deploy.md`
- Additional docs in `docs/` directory
