SHELL := /bin/bash

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


GO_INSTALL = ./hack/go-install.sh

LOCALBIN ?= $(shell pwd)/bin
ARTIFACT_DIR ?= .test

KCP ?= $(LOCALBIN)/kcp
KUBECTL_KCP ?= $(LOCALBIN)/kubectl-kcp

KCP_VERSION ?= 0.23.0

OS ?= $(shell go env GOOS )
ARCH ?= $(shell go env GOARCH )

$(KCP): ## Download kcp locally if necessary.
	mkdir -p $(LOCALBIN)
	curl -L -s -o - https://github.com/kcp-dev/kcp/releases/download/v$(KCP_VERSION)/kcp_$(KCP_VERSION)_$(OS)_$(ARCH).tar.gz | tar --directory $(LOCALBIN)/../ -xvzf - bin/kcp
	touch $(KCP) # we download an "old" file, so make will re-download to refresh it unless we make it newer than the owning dir

$(KUBECTL_KCP): ## Download kcp kubectl plugins locally if necessary.
	curl -L -s -o - https://github.com/kcp-dev/kcp/releases/download/v$(KCP_VERSION)/kubectl-kcp-plugin_$(KCP_VERSION)_$(OS)_$(ARCH).tar.gz | tar --directory $(LOCALBIN)/../ -xvzf - bin
	touch $(KUBECTL_KCP) # we download an "old" file, so make will re-download to refresh it unless we make it newer than the owning dir

build: $(KCP) $(KUBECTL_KCP) build-controller

ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

build-controller: ## Build the controller binary.
	go build -o $(LOCALBIN)/kcp-controller ./main.go

.PHONY: kcp-server
kcp-server: $(KCP) $(ARTIFACT_DIR)/kcp ## Run the kcp server.
	@if [[ ! -s $(ARTIFACT_DIR)/kcp.log ]]; then ( $(KCP) start -v 5 --root-directory $(ARTIFACT_DIR)/kcp --kubeconfig-path $(ARTIFACT_DIR)/kcp.kubeconfig --audit-log-maxsize 1024 --audit-log-mode=batch --audit-log-batch-max-wait=1s --audit-log-batch-max-size=1000 --audit-log-batch-buffer-size=10000 --audit-log-batch-throttle-burst=15 --audit-log-batch-throttle-enable=true --audit-log-batch-throttle-qps=10 --audit-policy-file ./test/e2e/audit-policy.yaml --audit-log-path $(ARTIFACT_DIR)/audit.log >$(ARTIFACT_DIR)/kcp.log 2>&1 & ); fi
	@echo "Waiting for kcp server to generate kubeconfig..."
	@while true; do if [[ ! -s $(ARTIFACT_DIR)/kcp.kubeconfig ]]; then sleep 0.2; else break; fi; done
	@echo "Waiting for kcp server to be ready..."
	@while true; do if ! kubectl --kubeconfig $(ARTIFACT_DIR)/kcp.kubeconfig get --raw /readyz >$(ARTIFACT_DIR)/kcp.probe.log 2>&1; then sleep 0.2; else break; fi; done
	@echo "kcp server is ready and running in the background. To stop run 'make test-cleanup'"

.PHONY: kcp-bootstrap
kcp-bootstrap: ## Bootstrap the kcp server.
	@go run ./config/main.go

.PHONY: kcp-controller
kcp-controller: build kcp-bootstrap ## Run the kcp-controller.
	@echo "Starting kcp-controller in the background. To stop run 'make test-cleanup'"
	@if [[ ! -s $(ARTIFACT_DIR)/controller.log ]]; then ( ./bin/kcp-controller >$(ARTIFACT_DIR)/controller.log 2>&1 & ); fi

.PHONY: test-e2e-cleanup
test-cleanup: ## Clean up processes and directories from an end-to-end test run.
	rm -rf $(ARTIFACT_DIR) || true
	pkill -sigterm kcp || true
	pkill -sigterm kubectl || true
	pkill -sigterm kcp-controller || true

$(ARTIFACT_DIR)/kcp: ## Create a directory for the kcp server data.
	mkdir -p $(ARTIFACT_DIR)/kcp

generate: build # Generate code
	./hack/update-codegen-crds.sh

run-local: build-controller kcp-bootstrap
	./bin/kcp-controller

.PHONY: test # Run tests
test:
	go test ./... --workspace=root --kubeconfig=$(CURDIR)/$(ARTIFACT_DIR)/kcp.kubeconfig
