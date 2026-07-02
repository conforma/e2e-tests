---
name: setup-cluster
description: >
  Set up a cluster and run conforma e2e tests via the full CI pipeline. Use when
  users ask "set up a cluster", "provision cluster", "create test cluster",
  "run the pipeline", "deploy konflux", or need a cluster to run e2e tests against.
  This skill runs actual commands to provision infrastructure and launch the pipeline.
---

# Set Up a Cluster and Run Conforma E2E Tests

This skill provisions an OpenShift cluster, installs OpenShift Pipelines, creates
the required secrets, and runs the conforma e2e pipeline which provisions its own
kind cluster on AWS, deploys Konflux, runs tests, and deprovisions.

## Prerequisites

- Access to cluster-bot (or similar tool) for OpenShift cluster provisioning
- AWS credentials for kind provisioning (mapt-kind-secret)
- Quay.io credentials for artifact storage
- `kubectl` configured to talk to the OpenShift cluster

## Step 1: Provision an OpenShift Cluster

Use cluster-bot or a similar tool:

```
workflow-launch hypershift-hostedcluster-workflow 4.15
```

Wait for the cluster to be ready and configure `KUBECONFIG`.

## Step 2: Install the OpenShift Pipelines Operator

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

Wait for the operator to be ready:

```bash
kubectl wait --for=condition=Ready tektonconfig/config --timeout=600s
```

## Step 3: Create Required Secrets

The following secrets must exist in the pipeline namespace before running the pipeline:

| Secret | Purpose |
|--------|---------|
| `mapt-kind-secret` | AWS credentials for kind cluster provisioning/deprovisioning |
| `konflux-e2e-secrets` | E2E test secrets (must contain `quay-token` key) |
| `konflux-test-infra` | OCI registry credentials for artifact storage (contains `oci-storage-dockerconfigjson` key) |
| `konflux-operator-e2e-credentials` | Operator-level credentials for E2E |

Create them with `kubectl create secret generic` using your credentials. For example:

```bash
kubectl create secret generic mapt-kind-secret \
  --from-file=aws-credentials=/path/to/aws-credentials

kubectl create secret generic konflux-e2e-secrets \
  --from-file=quay-token=/path/to/quay-token

kubectl create secret generic konflux-test-infra \
  --from-file=oci-storage-dockerconfigjson=/path/to/docker-config.json

kubectl create secret generic konflux-operator-e2e-credentials \
  --from-literal=GITHUB_APP_ID='<your-github-app-id>' \
  --from-file=GITHUB_PRIVATE_KEY='/tmp/gh-key.pem' \
  --from-literal=WEBHOOK_SECRET='<your-webhook-secret>' \
  --from-literal=QUAY_TOKEN='<your-quay-token>' \
  --from-literal=QUAY_ORGANIZATION='<your-quay-org>' \
  --from-literal=SMEE_CHANNEL='<your-smee-channel-url>'
```

## Step 4: Apply the Setup-Only Pipeline and Run

Apply only the provisioning and deploy tasks (strip the e2e test and deprovision tasks):

```bash
head -$(($(grep -n '# E2E tests' .tekton/pipelines/conforma-e2e/pipeline.yaml | head -1 | cut -d: -f1) - 2)) .tekton/pipelines/conforma-e2e/pipeline.yaml | kubectl apply -f -
```

Then create a PipelineRun to trigger it:

```bash
kubectl create -f .tekton/conforma-e2e-pull-request.yaml
```

Or create a PipelineRun manually with custom parameters:

```bash
kubectl create -f - <<EOF
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: conforma-e2e-
spec:
  pipelineRef:
    name: conforma-e2e-pipeline
  params:
    - name: git-url
      value: https://github.com/conforma/e2e-tests.git
    - name: revision
      value: main
    - name: oci-container-repo
      value: quay.io/conforma/e2e-tests
    - name: oci-container-repo-credentials-secret
      value: konflux-test-infra
    - name: aws-credentials-secret
      value: mapt-kind-secret
    - name: deprovision-aws-credentials-secret
      value: mapt-kind-secret
  taskRunTemplate:
    serviceAccountName: konflux-integration-runner
EOF
```

### Pipeline Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `git-url` | conforma/e2e-tests | Git URL for the test repo |
| `revision` | `main` | Git commit/branch/tag to test |
| `konflux-ready-timeout` | `30m` | Max wait for Konflux deployment |
| `custom-ec-cli-url` | (empty) | Custom CLI repo to build from source |
| `custom-ec-cli-revision` | (empty) | Custom CLI commit/branch |

## What the Pipeline Does

1. **provision-kind-cluster** — Spins up a kind cluster on AWS (16 CPU, 32GB RAM) via `kind-aws-provision` task
2. **deploy-konflux** — Installs Konflux + Tekton Chains via the konflux-ci operator

## Monitoring the Run

```bash
# Watch PipelineRun status
kubectl get pipelinerun -w

# Get detailed status
kubectl get pipelinerun <name> -o yaml

# Follow logs of a specific TaskRun
tkn taskrun logs <taskrun-name> -f

# Or follow the whole pipeline
tkn pipelinerun logs <pipelinerun-name> -f
```

## Retrieving Artifacts

Test artifacts (JUnit reports, logs) are pushed to OCI storage after the run:

```bash
oras pull quay.io/conforma/e2e-tests:<pipelinerun-name>
```

