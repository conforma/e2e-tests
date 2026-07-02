---
name: write-new-test
description: >
  Write a new e2e test for the conforma test suite. Use when users ask "add a test",
  "write a new test", "new test case", "test scenario", "how to use the framework",
  "how to use TektonController", "how to use CommonController", or need guidance on
  the test framework, client libraries, PipelineRun generators, or Gomega matchers.
---

# Write a New E2E Test

## Where Tests Live

All tests are in `tests/contract/contract.go` inside a single `ConformaSuiteDescribe` block. New tests are added as `It` blocks within the existing `Context` hierarchy.

The test entrypoint `cmd/e2e_test.go` imports `tests/contract` via a blank import for test discovery.

## Test Framework Overview

### Framework (`pkg/framework/`)

```go
fwk, err := framework.NewFramework(framework.GetGeneratedNamespace("my-test"))
// fwk.UserNamespace  - the created test namespace
// fwk.AsKubeAdmin    - ControllerHub with admin access
```

`ControllerHub` provides:
- `CommonController` - namespace ops, pods, secrets, configmaps
- `TektonController` - PipelineRuns, TaskRuns, ECP, Chains

### Common Controller (`pkg/clients/common/`)

```go
// Create test namespace with required secrets
fwk.AsKubeAdmin.CommonController.CreateQuayRegistrySecret(namespace)

// Wait for a pod to be running
fwk.AsKubeAdmin.CommonController.WaitForPodSelector(
    fwk.AsKubeAdmin.CommonController.IsPodRunning,
    namespace, "app", "my-app", 60, 100)

// Get a secret or configmap
secret, err := fwk.AsKubeAdmin.CommonController.GetSecret(ns, name)
cm, err := fwk.AsKubeAdmin.CommonController.GetConfigMap(name, ns)
```

### Tekton Controller (`pkg/clients/tekton/`)

```go
// Run a pipeline using a generator
pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(generator, namespace, timeout)

// Watch until completion
err = fwk.AsKubeAdmin.TektonController.WatchPipelineRun(pr.Name, namespace, timeout)

// Get TaskRun results
digest, err := fwk.AsKubeAdmin.TektonController.GetTaskRunResult(
    fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "task-name", "RESULT_NAME")

// Get TaskRun status (for assertions)
tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(
    fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "task-name")

// Manage EC policies
fwk.AsKubeAdmin.TektonController.CreateOrUpdatePolicyConfiguration(namespace, policySpec)
fwk.AsKubeAdmin.TektonController.CreateEnterpriseContractPolicy(name, namespace, spec)
```

## PipelineRun Generators (`pkg/utils/tekton/`)

Generators implement the `PipelineRunGenerator` interface:

### BuildahDemo - Build a container image
```go
generator := tekton.BuildahDemo{
    Image:        "quay.io/org/repo:tag",
    Bundle:       dockerBuildBundle,       // from build-pipeline-config ConfigMap
    PipelineName: "docker-build-oci-ta-min",
    Namespace:    namespace,
    Name:         "my-build-run",
}
```

### VerifyEnterpriseContract - Validate an image
```go
generator := tekton.VerifyEnterpriseContract{
    TaskBundle:          verifyECTaskBundle,  // from ec-defaults ConfigMap
    Name:                "verify-ec",
    Namespace:           namespace,
    PolicyConfiguration: "ec-policy",         // ConfigMap name
    PublicKey:           "k8s://ns/secret",
    Strict:              true,
    EffectiveTime:       "now",
    IgnoreRekor:         true,
}
generator.WithComponentImage("quay.io/org/image@sha256:abc123")
// Or multiple images:
generator.AppendComponentImage("quay.io/org/image2@sha256:def456")
```

## Gomega Matchers (`pkg/utils/tekton/matchers.go`)

```go
// Check if a TaskRun succeeded
gomega.Expect(tekton.DidTaskRunSucceed(tr)).To(gomega.BeTrue())

// Match a TaskRun result by name and value
gomega.Expect(tr.Status.Results).Should(
    gomega.ContainElements(
        tekton.MatchTaskRunResult("RESULT_NAME", "expected-value"),
    ),
)

// Match a TaskRun result via JSONPath
gomega.Expect(tr.Status.Results).Should(
    gomega.ContainElements(
        tekton.MatchTaskRunResultWithJSONPathValue(
            "TEST_OUTPUT", "{$.result}", `["SUCCESS"]`),
    ),
)
```

## Policy Manipulation (`pkg/utils/contract/`)

```go
// Set include filter on all sources
policy := contract.PolicySpecWithSourceConfig(
    defaultECP.Spec,
    ecp.SourceConfig{Include: []string{"slsa_provenance_available"}},
)

// Set config + rule data on all sources
policy := contract.PolicySpecWithSource(
    defaultECP.Spec,
    ecp.Source{
        Config:   &ecp.SourceConfig{Include: []string{"@slsa3"}},
        RuleData: &apiextensionsv1.JSON{Raw: []byte(`{"key": "value"}`)},
    },
)
```

## Template: Adding a New Verify-Enterprise-Contract Test

```go
ginkgo.It("describes what the test validates", func() {
    // 1. Configure policy (if different from BeforeEach default)
    policy := contract.PolicySpecWithSourceConfig(
        defaultECP.Spec, ecp.SourceConfig{Include: []string{"my_rule"}})
    gomega.Expect(fwk.AsKubeAdmin.TektonController.CreateOrUpdatePolicyConfiguration(
        namespace, policy)).To(gomega.Succeed())

    // 2. Configure generator (image, strict mode, etc.)
    generator.Strict = true
    generator.WithComponentImage(imageWithDigest)

    // 3. Run the pipeline
    pr, err := fwk.AsKubeAdmin.TektonController.RunPipeline(
        generator, namespace, pipelineRunTimeout)
    gomega.Expect(err).NotTo(gomega.HaveOccurred())
    gomega.Expect(fwk.AsKubeAdmin.TektonController.WatchPipelineRun(
        pr.Name, namespace, pipelineRunTimeout)).To(gomega.Succeed())

    // 4. Fetch results
    pr, err = fwk.AsKubeAdmin.TektonController.GetPipelineRun(pr.Name, pr.Namespace)
    gomega.Expect(err).NotTo(gomega.HaveOccurred())
    tr, err := fwk.AsKubeAdmin.TektonController.GetTaskRunStatus(
        fwk.AsKubeAdmin.CommonController.KubeRest(), pr, "verify-enterprise-contract")
    gomega.Expect(err).NotTo(gomega.HaveOccurred())

    // 5. Assert
    printTaskRunStatus(tr, namespace, *fwk.AsKubeAdmin.CommonController)
    gomega.Expect(tekton.DidTaskRunSucceed(tr)).To(gomega.BeTrue())
    gomega.Expect(tr.Status.Results).Should(
        gomega.ContainElements(
            tekton.MatchTaskRunResultWithJSONPathValue(
                constants.TektonTaskTestOutputName, "{$.result}", `["SUCCESS"]`),
        ),
    )
})
```

## Conventions

- Use `ginkgo.Label("ec")` on all test nodes (inherited from `ConformaSuiteDescribe`)
- Use `ginkgo.GinkgoWriter.Printf` for test output (not `fmt.Println`)
- Register failure diagnostics with `ginkgo.AfterEach(framework.ReportFailure(&fwk))`
- Generate unique names with `util.GenerateRandomString(10)` from `github.com/devfile/library/v2/pkg/util`
- Tests are `Ordered` (via `ConformaSuiteDescribe`), so earlier contexts set up state for later ones
