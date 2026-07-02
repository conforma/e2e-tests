---
name: pr-checklist
description: >
  Definition of done checklist for conforma/e2e-tests pull requests. Use when users
  ask "is this PR ready", "definition of done", "PR checklist", "what do I need
  before merging", "review checklist", or when preparing a PR for review.
---

# PR Definition of Done for conforma/e2e-tests

Before submitting a PR, verify each item:

## Code Quality

- [ ] `go build ./...` succeeds
- [ ] `go mod tidy` produces no changes
- [ ] `go vet ./...` reports no issues
- [ ] New test cases follow the existing patterns in `tests/contract/contract.go`
- [ ] No hardcoded image references that should be configurable via env vars

## Test Verification

- [ ] `make test-e2e-dry-run` lists the expected test nodes (no compilation errors, correct labels)
- [ ] If adding/modifying tests: the full suite passes against a Konflux cluster (`make test-e2e`)
- [ ] New tests use `ginkgo.Label("ec")` (inherited from `ConformaSuiteDescribe`, but verify if adding a new `Describe` block)

## Framework Usage

- [ ] Uses `framework.NewFramework()` for namespace creation and controller setup
- [ ] Uses `framework.ReportFailure()` in `AfterEach` for failure diagnostics
- [ ] Uses `ginkgo.GinkgoWriter.Printf` instead of `fmt.Println` for test output
- [ ] Uses `util.GenerateRandomString()` for unique resource names
- [ ] Secret/ConfigMap creation uses existing controller methods (not raw client calls)

## CI Compatibility

- [ ] No new required environment variables without updating `cmd/e2e_test.go` `requiredEnvVars`
- [ ] If adding optional env vars: documented in relevant skill files and README
- [ ] Pipeline YAML changes (`.tekton/`) are syntactically valid
- [ ] No changes that would break the CI pipeline's cluster provisioning flow

## Documentation

- [ ] Commit messages describe the "why", not just the "what"
- [ ] Complex policy configurations or test scenarios have inline comments explaining the intent
