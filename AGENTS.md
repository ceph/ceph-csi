# AGENTS.md

This file provides guidance to AI agents when working with code in this
branch of the repository.

## Branch Overview

This is the `ci/centos` branch. It does **not** contain the Ceph-CSI driver
source code. Its sole purpose is to host the CI infrastructure for the [CentOS
CI Jenkins instance](https://jenkins-ceph-csi.apps.ocp.cloud.ci.centos.org)
that tests the main Ceph-CSI codebase.

Contents of this branch:

- `jobs/` — [Jenkins Job Builder (JJB)][jjb] YAML definitions for every CI job
- `*.groovy` — [Jenkins Pipeline][pipeline] scripts, one per job
- `prepare.sh` — bare-metal machine provisioning script (installs deps, checks
  out the PR under test)
- `scripts/` — helper scripts used by pipelines and linting
- `deploy/` — OpenShift manifests and tooling to build and deploy the JJB
  container and push job definitions to Jenkins
- `mirror/` — container image mirroring from public registries into the
  internal CI registry, runs as a daily OpenShift CronJob
- `Makefile` — local linting and containerized-test entry points

The actual tests that CI runs live on the project's normal branches (e.g.
`devel`). This branch only controls *when* and *how* those tests are
triggered and executed.

## How CI Works

Each job follows the same pattern:

1. A Jenkins Pipeline (`.groovy` file) is triggered by a GitHub Pull Request
   event or a poll of the repository.
1. The pipeline runs on a `cico-workspace` Jenkins agent that has
   [duffy](https://duffy.ci.centos.org) configured to reserve bare-metal
   machines from the CentOS CI pool (`virt-ec2-t2-centos-9s-x86_64`).
1. `prepare.sh` is copied to the reserved machine and executed — it installs
   `git`, `podman`, `make`, and `wget`, then checks out the PR under test.
1. The pipeline SSH-es into the bare-metal machine and runs the appropriate
   `make` target from the checked-out ceph-csi repository.
1. The bare-metal machine is returned to the pool (`duffy client
   retire-session`) in the `finally` block, even on failure.

Container images are cached in the internal registry
`registry-ceph-csi.apps.ocp.cloud.ci.centos.org` to speed up jobs.
`scripts/container-needs-rebuild.sh` detects whether `build.env` or
`scripts/Dockerfile.{test,devel}` changed and forces a rebuild only when
necessary.

## Jenkins Jobs

Job definitions live in `jobs/*.yaml` (JJB format) paired with a
`<name>.groovy` pipeline script at the repository root.

| Job | YAML | Pipeline | Trigger |
|-----|------|----------|---------|
| `containerized-tests` | `jobs/containerized-tests.yaml` | `containerized-tests.groovy` | PR comment `/test ci/centos/containerized-tests` |
| `ci-job-validation` | `jobs/ci-job-validation.yaml` | `ci-job-validation.groovy` | PRs to `ci/centos`; `/test ci/centos/job-validation` |
| `commitlint` | `jobs/commitlint.yaml` | `commitlint.groovy` | PR comment `/test commitlint` |
| `build-images` | `jobs/build-images.yaml` | `build-images.groovy` | Poll SCM every 5 min on `devel` |
| `mini-e2e_k8s-{ver}` | `jobs/mini-e2e.yaml` | `mini-e2e.groovy` | PR comment `/test ci/centos/mini-e2e[/k8s-{ver}]` |
| `mini-e2e-helm_k8s-{ver}` | `jobs/mini-e2e.yaml` | `mini-e2e-helm.groovy` | PR comment `/test ci/centos/mini-e2e-helm[/k8s-{ver}]` |
| `mini-e2e-operator_k8s-{ver}` | `jobs/mini-e2e.yaml` | `mini-e2e-operator.groovy` | PR comment `/test ci/centos/mini-e2e-operator[/k8s-{ver}]` |
| `k8s-e2e-external-storage-{ver}` | `jobs/k8s-e2e-external-storage.yaml` | `k8s-e2e-external-storage.groovy` | PR comment `/test ci/centos/k8s-e2e-external-storage[/{ver}]` |
| `upgrade-tests-{type}` | `jobs/upgrade-tests.yaml` | `upgrade-tests.groovy` | PR comment `/test ci/centos/upgrade-tests[-{type}]` |
| `jjb-validate` | `jobs/jjb-validate.yaml` | `jjb-validate.groovy` | PRs to `ci/centos`; `/test ci/centos/jjb-validate` |
| `jjb-deploy` | `jobs/jjb-deploy.yaml` | `jjb-deploy.groovy` | Poll SCM every 5 min on `ci/centos` |

Kubernetes versions covered by matrix jobs: 1.32 – 1.37.
Test types for `mini-e2e`: `cephfs`, `nfs`, `nvmeof`, `rbd`.
Test types for `upgrade-tests`: `cephfs`, `rbd`.

## Linting and Local Testing

The `Makefile` in this branch lints the CI scripts themselves (not the
ceph-csi driver). Always use `make containerized-test` to validate changes —
it runs the linters inside a container built from `scripts/Dockerfile.test`,
ensuring the same environment as CI.

Use `TARGET` to select which lint target to run:

```bash
make containerized-test                        # default: lint-all (shell + markdown + yaml)
make containerized-test TARGET=lint-all        # shell + markdown + yaml lints
make containerized-test TARGET=lint-shell      # *.sh files: shellcheck + bash -n
make containerized-test TARGET=lint-markdown   # *.md files: mdl
make containerized-test TARGET=lint-yaml       # *.yaml files: yamllint
make containerized-test TARGET=commitlint      # commit messages since origin/ci/centos
make containerized-test TARGET=commitlint REBASE=1  # rebase first, then commitlint
```

Running all checks at once (what CI runs):

```bash
make test   # containerized lint-all + containerized commitlint
```

The individual targets (`lint-shell`, `lint-markdown`, `lint-yaml`,
`commitlint`) can also be run directly without a container if the required
tools (`shellcheck`, `mdl`, `yamllint`, `commitlint`) are installed locally.

The test container is rebuilt automatically when `scripts/Dockerfile.test`
changes. To skip rebuilding and use a pre-pulled image:

```bash
make containerized-test USE_PULLED_IMAGE=yes
```

## Image Mirroring

The `mirror/` directory keeps a copy of all external container images that CI
jobs depend on inside the internal registry
(`registry-ceph-csi.apps.ocp.cloud.ci.centos.org`). This avoids rate-limiting
and network failures when pulling from public registries during test runs.

### How it works

- `mirror/images.txt` lists every image to mirror in the format:
  `<source-image> <destination-short-name>`
- `mirror/mirror-images.sh` iterates the list and uses `skopeo copy` to
  push each image into the CI registry. Authentication is provided via the
  `DOCKER_CONFIG_JSON`, `CI_REGISTRY_USER`, and `CI_REGISTRY_PASSWD`
  environment variables.
- `mirror/Containerfile` builds the container image that runs the script
  (based on `quay.io/centos/centos:stream9` with `skopeo` installed).
- `mirror/mirror-buildconfig.yaml` defines the OpenShift `BuildConfig` and
  `ImageStream` that build the mirror container image from the `mirror/`
  context directory of the `ci/centos` branch.
- `mirror/mirror-cronjob.yaml` defines an OpenShift `CronJob` (`@daily`)
  that runs the mirror container, injecting registry credentials from the
  `cephcsibot-docker-io` and `container-registry-auth` OpenShift Secrets.

### Adding or updating a mirrored image

Edit `mirror/images.txt` — add a line with the full source image reference
and the desired short name under the CI registry:

```
quay.io/example/image:tag    example/image:tag
```

The next daily CronJob run will pick it up. To trigger an immediate sync,
start a new build in OpenShift:

```bash
oc start-build mirror-images
```

### Deploying the mirror infrastructure (once)

```bash
oc create -f mirror/mirror-buildconfig.yaml   # ImageStream + BuildConfig
oc create -f mirror/mirror-cronjob.yaml       # CronJob
```

## Deploying / Updating Jenkins Jobs

Jenkins Jobs are deployed via a JJB container running on OpenShift. See
[`deploy/README.md`](deploy/README.md) for full instructions. The short
version:

```bash
# Push all OpenShift objects (once):
oc create -f deploy/<file>.yaml    # for each yaml in deploy/

# Validate job definitions (dry-run):
./deploy/jjb.sh --cmd validate --GIT_REF ci/centos

# Deploy (push) job definitions to Jenkins:
./deploy/jjb.sh --cmd deploy --GIT_REF ci/centos
```

The `jjb-deploy` Jenkins job automates this — it fires on every change to
`ci/centos` (5-minute SCM poll) and pushes updated job definitions to Jenkins.

Local validation without OpenShift (requires `jenkins-job-builder` installed):

```bash
make -C deploy test                # jenkins-jobs test -o _output/ jobs/
```

## Adding or Modifying a CI Job

1. **Add/edit the JJB definition** in `jobs/<name>.yaml`. Follow the
   structure of an existing file — each job needs `project-type: pipeline`,
   a `pipeline-scm` block pointing to this branch, `script-path: <name>.groovy`,
   and a `triggers` block.

1. **Add/edit the Groovy pipeline** `<name>.groovy` at the repository root.
   All pipelines must return the bare-metal machine in a `finally` block.

1. **Run linting** before committing:

   ```bash
   make containerized-test TARGET=lint-yaml    # validates *.yaml files
   make containerized-test TARGET=lint-shell   # validates *.sh scripts
   ```

1. **Validate JJB syntax** with:

   ```bash
   make -C deploy test
   ```

1. Open a PR targeting the `ci/centos` branch. The `ci-job-validation` and
   `jjb-validate` jobs will run automatically to verify the changes.

## Commit Message Format

```
ci: <subject of the change>

<paragraph(s) with reason/description>

Assisted-by: AI Agent Name (Optional Model) <contact@example.com>
Signed-off-by: Your Name <your.email@example.org>
```

Rules (enforced by `commitlint`):

- **Type prefix** must be one of: `build`, `cephfs`, `ci`, `cleanup`,
  `deploy`, `doc`, `e2e`, `helm`, `journal`, `rbd`, `rebase`, `revert`,
  `util`
- **Subject line**: max 72 characters, no trailing period
- **Scopes** are not used (scope-empty warning)
- All commits require DCO sign-off (`git commit -s`)
- Add `Assisted-By: AskBob <askbob@ibm.com>` when an AI agent helped

## Key Files

| File | Purpose |
|------|---------|
| `Makefile` | Linting and containerized-test entry points |
| `prepare.sh` | Installs deps and checks out a PR on a bare-metal machine |
| `scripts/Dockerfile.test` | Container image used for linting |
| `scripts/lint-extras.sh` | Shell/markdown/yaml lint runner |
| `scripts/skip-doc-change.sh` | Exits 1 when only docs changed (skips CI) |
| `scripts/container-needs-rebuild.sh` | Detects if container image must be rebuilt |
| `jobs/` | JJB YAML job definitions |
| `deploy/` | OpenShift manifests + JJB container + `jjb.sh` helper |
| `deploy/Makefile` | `test` (validate) and `deploy` targets for `jenkins-jobs` |
| `deploy/jjb.sh` | Creates an OpenShift Job, waits for it, reports result |
| `mirror/images.txt` | List of images to mirror into the CI registry |
| `mirror/mirror-images.sh` | Mirrors images using `skopeo copy` |
| `mirror/Containerfile` | Container image that runs `mirror-images.sh` |
| `mirror/mirror-buildconfig.yaml` | OpenShift `BuildConfig`/`ImageStream` for the mirror image |
| `mirror/mirror-cronjob.yaml` | OpenShift `CronJob` that runs the mirror daily |
| `.commitlintrc.yml` | Commitlint rules for this branch |

## External Links

- [Jenkins instance][ceph_csi_ci]
- [Jenkins Job Builder docs][jjb]

[ceph_csi_ci]: https://jenkins-ceph-csi.apps.ocp.cloud.ci.centos.org
[jjb]: https://jenkins-job-builder.readthedocs.io/en/latest/index.html
[pipeline]: https://jenkins-job-builder.readthedocs.io/en/latest/project_pipeline.html
