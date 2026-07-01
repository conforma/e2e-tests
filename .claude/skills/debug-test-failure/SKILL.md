---
name: debug-test-failure
description: >
  Debug a failing conforma e2e test. Use when users ask "why is this test failing",
  "debug e2e failure", "test failed", "pipeline run failed", "task run failed",
  "how to read test logs", "attestation not found", "chains controller", or need
  help interpreting Ginkgo output, TaskRun results, or PipelineRun status.
---

# Debug a Failing E2E Test

## Step 1: Identify the Failure

Read the Ginkgo output. The test suite prints structured diagnostics on failure via `ReportFailure` and `printTaskRunStatus`. Look for:

- **Which `It` block failed** - the test name tells you what scenario broke
- **Gomega assertion message** - the expected vs actual comparison
- **TaskRun status YAML** - serialized TaskRun status dumped on failure
- **Container logs** - logs from each step container in the TaskRun pod

## Step 2: Understand the Test Structure

All tests are in `tests/contract/contract.go`. The suite is ordered:

1. **Infrastructure checks** (`pipeline` label) - verifies Chains controller is running and signing secret exists
2. **Image build** - runs a `buildah-demo` PipelineRun, extracts `IMAGE_DIGEST` and `IMAGE_URL`
3. **Attestation** - waits up to 20min for Tekton Chains to produce `.sig` and `.att` artifacts
4. **Verify-enterprise-contract tests** - runs the EC validation task with various policy configurations

Tests are sequential and share state: if the image build fails, all downstream tests fail.

## Step 3: Common Failure Patterns

### "chains controller is not running"
The Tekton Chains controller pod is not ready.
```bash
kubectl get pods -n <chains-namespace> -l app=tekton-chains-controller
kubectl logs -n <chains-namespace> -l app=tekton-chains-controller
```
The test auto-discovers the chains namespace from: `tekton-chains`, `openshift-pipelines`, `tekton-pipelines`.

### "signing secret not present"
The cosign signing secret hasn't been provisioned.
```bash
kubectl get secret signing-secrets -n <chains-namespace> -o jsonpath='{.data}' | jq 'keys'
```
Expected keys: `cosign.key`, `cosign.pub`, `cosign.password`.

### "Could not find .att or .sig ImageStreamTags within the 20m0s timeout"
Tekton Chains didn't sign the image in time. Check:
```bash
# Chains controller logs for errors
kubectl logs -n <chains-namespace> -l app=tekton-chains-controller --tail=100

# Verify the build PipelineRun completed
kubectl get pipelinerun -n <test-namespace> -l tekton.dev/pipeline=buildah-demo
```

### Task result mismatch (SUCCESS vs FAILURE vs WARNING)
The verify-enterprise-contract task returned an unexpected result. Examine:
- The `step-report-json` container logs (EC validation report)
- The policy configuration (`ec-policy` ConfigMap in the test namespace)
- Which policy rules were included/excluded

### "both CUSTOM_EC_CLI_IMAGE and CUSTOM_EC_TASK_YAML must be set together"
Set both or neither. These are used to test a custom EC CLI build.

## Step 4: Inspect Cluster State

```bash
# List test namespaces (pattern: chains-e2e-<random>)
kubectl get ns | grep chains-e2e

# Check PipelineRuns in the test namespace
kubectl get pipelinerun -n <namespace>

# Get TaskRun details from a PipelineRun
kubectl get taskrun -n <namespace> -l tekton.dev/pipelineRun=<pr-name>

# Read the EC policy ConfigMap
kubectl get configmap ec-policy -n <namespace> -o yaml

# Check the EnterpriseContractPolicy CRD
kubectl get enterprisecontractpolicy -n enterprise-contract-service -o yaml
```

## Step 5: Read Container Logs

The test suite dumps logs automatically on failure, but you can also fetch them manually:

```bash
# Find the TaskRun pod
kubectl get pods -n <namespace> -l tekton.dev/taskRun=<taskrun-name>

# Read specific step logs
kubectl logs -n <namespace> <pod-name> -c step-validate
kubectl logs -n <namespace> <pod-name> -c step-report-json
kubectl logs -n <namespace> <pod-name> -c step-summary
```

## Step 6: CI-Specific Debugging

In CI, artifacts are pushed to OCI storage after the test run:
- **JUnit report**: `e2e-report.xml` in the artifact directory
- **OCI artifact**: pushed to `quay.io/conforma/e2e-tests:<pipelinerun-name>`

To retrieve CI artifacts:
```bash
oras pull quay.io/conforma/e2e-tests:<pipelinerun-name>
```

The CI cluster is ephemeral (KinD on AWS) and is deprovisioned after the run, so cluster-level debugging must happen from the collected artifacts.
