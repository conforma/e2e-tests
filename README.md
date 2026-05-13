# Conforma E2E Tests

### Description

These tests validate Conforma (Enterprise Contract) end-to-end functionality -- covering Tekton Chains signing, attestation verification, and enterprise contract policy evaluation. They are ported from [konflux-ci/e2e-tests](https://github.com/konflux-ci/e2e-tests) and run against an upstream Konflux instance deployed with Tekton Chains.

The test suite uses the [Ginkgo](https://onsi.github.io/ginkgo/) framework and is labeled with `ec` for selective execution.

### Prerequisites

- An OpenShift or KinD cluster with Konflux deployed (including Tekton Chains and Enterprise Contract)
- `QUAY_TOKEN` environment variable set (base64-encoded Docker config for Quay.io registry access)
- The following CRD APIs available on the cluster:
  - `Snapshot` (Application API)
  - `PipelineRun` (Tekton)
  - `EnterpriseContractPolicy` (Conforma CRDs)

### What the tests cover

1. **Infrastructure checks**
   - Tekton Chains controller is running
   - Cosign signing secret (`signing-secrets`) is present with `cosign.key`, `cosign.pub`, and `cosign.password`

2. **Image build, signing, and attestation**
   - A `buildah-demo` pipeline builds and pushes a container image
   - Tekton Chains creates a cosign signature (`.sig`) and attestation (`.att`) for the image

3. **Enterprise Contract verification (`verify-enterprise-contract` task)**
   - Succeeds when the SLSA provenance policy is met
   - Reports `FAILURE` (non-strict mode) when test policies are not satisfied
   - Fails (strict mode) when test policies are not satisfied
   - Fails when an unexpected/wrong signing key is used

4. **EC CLI validation**
   - Error handling: verifies proper failure message when attestation doesn't match the public key
   - Multi-image validation: accepts a list of image references for batch verification

5. **Release policy**
   - Red Hat products pass the full Red Hat policy rule collection
   - Untrusted task references are detected and rejected
   - Unpinned task bundle references produce a `WARNING`

### How to run

#### Option A: Full pipeline (provisions a new cluster)

1. **Provision an OpenShift cluster**

   Use cluster-bot or a similar tool:

   ```
   workflow-launch hypershift-hostedcluster-workflow 4.15
   ```

2. **Install the OpenShift Pipelines operator**

   ```bash
   kubectl apply -f - <<EOF
   apiVersion: operators.coreos.com/v1alpha1
   kind: Subscription
   metadata:
     name: openshift-pipelines-operator
     namespace: openshift-operators
   spec:
     channel: latest
     name: openshift-pipelines-operator-rh
     source: redhat-operators
     sourceNamespace: openshift-marketplace
   EOF
   ```

3. **Create required secrets**

   The following secrets must exist in the pipeline namespace:

   | Secret | Purpose |
   |--------|---------|
   | `mapt-kind-secret` | AWS credentials for KinD cluster provisioning/deprovisioning |
   | `konflux-e2e-secrets` | E2E test secrets (e.g., `quay-token`) |
   | `konflux-test-infra` | OCI registry credentials for artifact storage |
   | `konflux-operator-e2e-credentials` | Operator-level credentials for E2E |

4. **Apply the pipeline definition**

   ```bash
   kubectl apply -f ./.tekton/pipelines/conforma-e2e/pipeline.yaml
   ```

5. **Start the pipeline**

   ```bash
   tkn pipeline start conforma-e2e-pipeline \
     --param git-url=https://github.com/conforma/e2e-tests.git \
     --param revision=main \
     --param oci-container-repo=quay.io/conforma/e2e-tests \
     --param oci-container-repo-credentials-secret=konflux-test-infra \
     --use-param-defaults \
     --showlog
   ```

   The pipeline will:
   - Provision a KinD cluster on AWS
   - Deploy Konflux with Tekton Chains via the Konflux operator
   - Run the Ginkgo test suite
   - Collect artifacts and push to OCI
   - Deprovision the cluster

#### Option B: Run directly against an existing Konflux cluster

If you already have a Konflux cluster running:

```bash
cd e2e-tests
export KUBECONFIG=/path/to/your/kubeconfig
export QUAY_TOKEN="$(base64 -w0 < ~/.docker/config.json)"
export TEST_ENVIRONMENT=upstream
go run github.com/onsi/ginkgo/v2/ginkgo -v --label-filter="ec" ./cmd
```

Or using the Makefile from the repository root:

```bash
export KUBECONFIG=/path/to/your/kubeconfig
export QUAY_TOKEN="$(base64 -w0 < ~/.docker/config.json)"
export TEST_ENVIRONMENT=upstream
make test-e2e
```

### Configuration

| Environment variable | Required | Description |
|---------------------|----------|-------------|
| `KUBECONFIG` | Yes | Path to kubeconfig for the target cluster |
| `QUAY_TOKEN` | Yes | Base64-encoded Docker config for Quay.io registry |
| `TEST_ENVIRONMENT` | No | Set to `upstream` for upstream Konflux deployments |
| `QUAY_E2E_ORGANIZATION_ENV` | No | Quay.io organization for test images (defaults to `redhat-appstudio-qe`) |
| `E2E_APPLICATIONS_NAMESPACE` | No | Override the generated test namespace |
| `KLOG_VERBOSITY` | No | Kubernetes client logging verbosity (default: `1`) |

### Project structure

```
e2e-tests/
  cmd/e2e_test.go                      # Test entrypoint and BeforeSuite setup
  tests/contract/contract.go           # Enterprise Contract test scenarios
  pkg/
    clients/
      common/controller.go             # Kubernetes helper operations
      kubernetes/client.go             # K8s client initialization
      tekton/                          # Tekton-specific clients (bundles, chains, ECP, pipelines, signing)
    constants/constants.go             # Shared constants and timeouts
    framework/                         # Test framework (namespace creation, RBAC, reporting)
    utils/
      contract/policy.go               # ECP policy helpers
      tekton/                          # Pipeline generators, matchers, cosign utilities
.tekton/
  pipelines/conforma-e2e/pipeline.yaml # Tekton Pipeline for full CI execution
  conforma-e2e-pull-request.yaml       # PipelineRun trigger for pull requests
```
