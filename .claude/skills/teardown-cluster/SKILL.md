---
name: teardown-cluster
description: >
  Tear down and deprovision test infrastructure after e2e tests. Use when users
  ask "tear down cluster", "deprovision", "clean up cluster", "delete cluster",
  "destroy cluster", or need to clean up after a pipeline run.
---

# Tear Down Test Infrastructure

## kind Cluster on AWS

The kind cluster on AWS is automatically deprovisioned by the pipeline's `finally`
block (see `.tekton/pipelines/conforma-e2e/pipeline.yaml`). It always runs, even
on failure. **No manual action is needed if the pipeline ran to completion.**

### Verify Deprovision Ran

```bash
kubectl get taskrun -l tekton.dev/pipelineRun=<name>,tekton.dev/pipelineTask=deprovision-kind-cluster
```

If it failed, check the logs:

```bash
tkn taskrun logs <deprovision-taskrun-name>
```

### Manual Cleanup (Pipeline Cancelled or Deprovision Failed)

If the pipeline was cancelled before the `finally` block ran, the kind cluster on
AWS may still be running. Re-run the deprovision task manually:

```bash
kubectl create -f - <<EOF
apiVersion: tekton.dev/v1
kind: TaskRun
metadata:
  generateName: deprovision-kind-
spec:
  taskRef:
    resolver: git
    params:
      - name: url
        value: https://github.com/konflux-ci/tekton-integration-catalog.git
      - name: revision
        value: main
      - name: pathInRepo
        value: tasks/mapt-oci/kind-aws-spot/deprovision/0.1/kind-aws-deprovision.yaml
  params:
    - name: secret-aws-credentials
      value: mapt-kind-secret
    - name: id
      value: <original-pipelinerun-name>
    - name: cluster-access-secret
      value: kfg-<original-pipelinerun-name>
    - name: oci-container
      value: quay.io/conforma/e2e-tests:<original-pipelinerun-name>
    - name: oci-credentials
      value: konflux-test-infra
EOF
```

Replace `<original-pipelinerun-name>` with the name of the PipelineRun that
provisioned the cluster.

## OpenShift Cluster

The OpenShift cluster used to run the pipeline is separate from the kind cluster.
Tear it down using cluster-bot or your provisioning tool.

## Clean Up Pipeline Resources

```bash
# Delete a specific PipelineRun
kubectl delete pipelinerun <name>

# Delete all completed PipelineRuns
kubectl delete pipelinerun --field-selector=status.conditions[0].reason=Succeeded

# Delete the pipeline definition
kubectl delete pipeline conforma-e2e-pipeline
```
