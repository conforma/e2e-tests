GOBIN ?= $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN = $(shell go env GOPATH)/bin
endif

.PHONY: test-e2e
test-e2e:
	cd e2e-tests && go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo && \
	$(GOBIN)/ginkgo -v --timeout=60m --label-filter="ec" --junit-report=e2e-report.xml ./cmd

.PHONY: test-e2e-dry-run
test-e2e-dry-run:
	cd e2e-tests && go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo && \
	$(GOBIN)/ginkgo -v --dry-run --label-filter="ec" ./cmd

.PHONY: build
build:
	cd e2e-tests && go build ./...

.PHONY: tidy
tidy:
	cd e2e-tests && go mod tidy
