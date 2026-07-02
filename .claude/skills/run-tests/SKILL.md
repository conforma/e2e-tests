---
name: run-tests
description: >
  Run conforma e2e tests locally or understand how CI runs them. Use when users ask
  "how do I run the tests", "run e2e tests", "dry run", "ginkgo", "label filter",
  "test timeout", "make test", or questions about test execution, environment setup,
  or required secrets/env vars.
---

# Run Conforma E2E Tests

## Prerequisites

1. **A kind cluster with Konflux deployed** — use the `setup-cluster` skill to provision one
2. **KUBECONFIG** pointing at the kind cluster (e.g. `/tmp/kind-kubeconfig`)
3. **Go 1.26+** installed
4. **QUAY_TOKEN** env var set (base64-encoded Docker config JSON for quay.io push access)

## Quick Start

```bash
# Install Ginkgo CLI
go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo

# Dry run (list tests without executing)
make test-e2e-dry-run

# Full run (60m timeout, label-filter="ec")
make test-e2e
```

## Running with Ginkgo Directly

The Makefile targets call Ginkgo under the hood. For more control:

```bash
# Default full suite
ginkgo -v --timeout=60m --label-filter="ec" ./cmd

# Dry run to preview test nodes
ginkgo -v --dry-run --label-filter="ec" ./cmd

# Focus on a specific test by name regex
ginkgo -v --timeout=60m --label-filter="ec" --focus="succeeds when policy is met" ./cmd

# Run only pipeline infrastructure checks
ginkgo -v --timeout=60m --label-filter="ec && pipeline" ./cmd

# JUnit report output
ginkgo -v --timeout=60m --label-filter="ec" --junit-report=e2e-report.xml --output-dir=./artifacts ./cmd
```

### Available Ginkgo Labels

| Label | Scope |
|-------|-------|
| `ec` | All tests (required by Makefile) |
| `pipeline` | Infrastructure checks + image build/sign tests |
| `conforma-suite` | Applied by `ConformaSuiteDescribe` wrapper |

## Required Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `QUAY_TOKEN` | Yes | Base64-encoded Docker config JSON for quay.io. Checked in `BeforeSuite` |
| `KUBECONFIG` | Yes (defaults to `~/.kube/config`) | Path to cluster kubeconfig |

## Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TEST_ENVIRONMENT` | (empty) | Set to `upstream` for upstream Konflux clusters (changes RBAC setup) |
| `E2E_APPLICATIONS_NAMESPACE` | `chains-e2e-<random>` | Override the auto-generated test namespace |
| `QUAY_E2E_ORGANIZATION` | `redhat-appstudio-qe` | Quay.io org for test image storage |
| `CUSTOM_EC_CLI_IMAGE` | (none) | Custom EC CLI container image to test against |
| `CUSTOM_EC_TASK_YAML` | (none) | Path to verify-enterprise-contract task YAML (must be set with `CUSTOM_EC_CLI_IMAGE`) |
| `KLOG_VERBOSITY` | `1` | Kubernetes client log verbosity (0-4) |

## How CI Runs Tests

The Tekton pipeline in `.tekton/pipelines/conforma-e2e/pipeline.yaml` runs `ginkgo` with `--label-filter="ec"` against the kind cluster. CI also supports custom EC CLI builds: pass `custom-ec-cli-url` and `custom-ec-cli-revision` pipeline params to build the CLI from source and test against it.

## Troubleshooting

- **"failed to create sandbox user"**: The cluster doesn't have the expected CRDs. Verify `SnapshotList`, `PipelineRunList`, and `EnterpriseContractPolicyList` APIs are available.
- **QUAY_TOKEN missing**: Export it before running. The test suite fails fast in `BeforeSuite` if it's not set.
- **Timeout**: Default is 60m. Image builds + Chains attestation can take 20+ minutes. Increase with `--timeout=90m` if needed.
