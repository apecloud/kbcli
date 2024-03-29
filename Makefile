#
# Copyright 2022 The KubeBlocks Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#     http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

################################################################################
# Variables                                                                    #
################################################################################

APP_NAME = kbcli
VERSION ?= 0.9.0-alpha.0
GITHUB_PROXY ?=
GIT_COMMIT  = $(shell git rev-list -1 HEAD)
GIT_VERSION = $(shell git describe --always --abbrev=0 --tag)
ADDON_BRANCH ?= main

# Go setup
export GO111MODULE = auto
export GOSUMDB = sum.golang.org
export GONOPROXY = github.com/apecloud
export GONOSUMDB = github.com/apecloud
export GOPRIVATE = github.com/apecloud

GO ?= go
GOFMT ?= gofmt
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell $(GO) env GOBIN))
GOBIN=$(shell $(GO) env GOPATH)/bin
else
GOBIN=$(shell $(GO) env GOBIN)
endif
GOPROXY := $(shell go env GOPROXY)
ifeq ($(GOPROXY),)
GOPROXY := https://proxy.golang.org
## use following GOPROXY settings for Chinese mainland developers.
#GOPROXY := https://goproxy.cn
endif
export GOPROXY

# build tags
BUILD_TAGS="containers_image_openpgp"


TAG_LATEST ?= false
BUILDX_ENABLED ?= ""
ifeq ($(BUILDX_ENABLED), "")
	ifeq ($(shell docker buildx inspect 2>/dev/null | awk '/Status/ { print $$2 }'), running)
		BUILDX_ENABLED = true
	else
		BUILDX_ENABLED = false
	endif
endif
BUILDX_BUILDER ?= "x-builder"

define BUILDX_ERROR
buildx not enabled, refusing to run this recipe
endef

DOCKER_BUILD_ARGS =
DOCKER_NO_BUILD_CACHE ?= false

ifeq ($(DOCKER_NO_BUILD_CACHE), true)
	DOCKER_BUILD_ARGS = $(DOCKER_BUILD_ARGS) --no-cache
endif

.DEFAULT_GOAL := help

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php
# https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development
.PHONY: generate
generate: build-kbcli-embed-chart

.PHONY: fmt
fmt: ## Run go fmt against code.
	$(GOFMT) -l -w -s $$(git ls-files --exclude-standard | grep "\.go$$")

.PHONY: vet
vet: ## Run go vet against code.
	GOOS=$(GOOS) $(GO) vet -tags $(BUILD_TAGS) -mod=mod ./...

.PHONY: cue-fmt
cue-fmt: cuetool ## Run cue fmt against code.
	git ls-files --exclude-standard | grep "\.cue$$" | xargs $(CUE) fmt
	git ls-files --exclude-standard | grep "\.cue$$" | xargs $(CUE) fix

.PHONY: lint-fast
lint-fast: staticcheck vet golangci-lint # [INTERNAL] Run all lint job against code.

.PHONY: lint
lint: generate ## Run default lint job against code.
	$(MAKE) golangci-lint

.PHONY: golangci-lint
golangci-lint: golangci generate ## Run golangci-lint against code.
	$(GOLANGCILINT) run ./...

.PHONY: staticcheck
staticcheck: staticchecktool generate ## Run staticcheck against code.
	$(STATICCHECK) -tags $(BUILD_TAGS) ./...

.PHONY: build-checks
build-checks: generate fmt vet goimports lint-fast ## Run build checks.

.PHONY: mod-download
mod-download: ## Run go mod download against go modules.
	$(GO) mod download

.PHONY: module
module: ## Run go mod tidy->verify against go modules.
	$(GO) mod tidy -compat=1.21
	$(GO) mod verify

TEST_PACKAGES ?= ./pkg/... ./cmd/...

OUTPUT_COVERAGE=-coverprofile cover.out
.PHONY: test
test: generate ## Run operator controller tests with current $KUBECONFIG context. if existing k8s cluster is k3d or minikube, specify EXISTING_CLUSTER_TYPE.
	$(GO) test -tags $(BUILD_TAGS) -p 1 $(TEST_PACKAGES) $(OUTPUT_COVERAGE)

.PHONY: test-fast
test-fast:
	$(GO) test -tags $(BUILD_TAGS) -short $(TEST_PACKAGES)  $(OUTPUT_COVERAGE)

.PHONY: cover-report
cover-report: ## Generate cover.html from cover.out
	$(GO) tool cover -html=cover.out -o cover.html
ifeq ($(GOOS), darwin)
	open ./cover.html
else
	echo "open cover.html with a HTML viewer."
endif

.PHONY: goimports
goimports: goimportstool ## Run goimports against code.
	$(GOIMPORTS) -local github.com/apecloud/kbcli -w $$(git ls-files|grep "\.go$$")


##@ CLI
K3S_VERSION ?= v1.23.8+k3s1
K3D_VERSION ?= 5.4.4
K3S_IMG_TAG ?= $(subst +,-,$(K3S_VERSION))
FETCH_ADDON_ENABLED ?= true

CLI_LD_FLAGS ="-s -w \
	-X github.com/apecloud/kbcli/version.BuildDate=`date -u +'%Y-%m-%dT%H:%M:%SZ'` \
	-X github.com/apecloud/kbcli/version.GitCommit=$(GIT_COMMIT) \
	-X github.com/apecloud/kbcli/version.GitVersion=$(GIT_VERSION) \
	-X github.com/apecloud/kbcli/version.Version=$(VERSION) \
	-X github.com/apecloud/kbcli/version.K3sImageTag=$(K3S_IMG_TAG) \
	-X github.com/apecloud/kbcli/version.K3dVersion=$(K3D_VERSION) \
	-X github.com/apecloud/kbcli/version.DefaultKubeBlocksVersion=$(VERSION)"

bin/kbcli.%: ## Cross build bin/kbcli.$(OS).$(ARCH).
	GOOS=$(word 2,$(subst ., ,$@)) GOARCH=$(word 3,$(subst ., ,$@)) $(GO) build -tags $(BUILD_TAGS) -ldflags=${CLI_LD_FLAGS} -o $@ cmd/cli/main.go

.PHONY: fetch-addons
fetch-addons: ## update addon submodule
ifeq ($(FETCH_ADDON_ENABLED), true)
	git submodule update --init --recursive --remote --force
	git submodule
endif

.PHONY: kbcli-fast
kbcli-fast: OS=$(shell $(GO) env GOOS)
kbcli-fast: ARCH=$(shell $(GO) env GOARCH)
kbcli-fast: build-kbcli-embed-chart
	$(MAKE) bin/kbcli.$(OS).$(ARCH)
	@mv bin/kbcli.$(OS).$(ARCH) bin/kbcli

create-kbcli-embed-charts-dir:
	mkdir -p pkg/cluster/charts/

build-single-kbcli-embed-chart.%: chart=$(word 2,$(subst ., ,$@))
build-single-kbcli-embed-chart.%:
	$(HELM) dependency update addons/addons/$(chart) --skip-refresh
	$(HELM) package addons/addons/$(chart)
	- bash -c "diff <($(HELM) template $(chart)-*.tgz) <($(HELM) template pkg/cluster/charts/$(chart).tgz)" > chart.diff
	@if [ -s chart.diff ]; then \
 	  mv $(chart)-*.tgz pkg/cluster/charts/$(chart).tgz; \
 	else \
 	  rm $(chart)-*.tgz; \
	fi
	rm chart.diff

.PHONY: build-kbcli-embed-chart
build-kbcli-embed-chart: helmtool fetch-addons create-kbcli-embed-charts-dir \
	build-single-kbcli-embed-chart.apecloud-mysql-cluster \
	build-single-kbcli-embed-chart.redis-cluster \
	build-single-kbcli-embed-chart.postgresql-cluster \
	build-single-kbcli-embed-chart.kafka-cluster \
	build-single-kbcli-embed-chart.mongodb-cluster \
	build-single-kbcli-embed-chart.llm-cluster \
	build-single-kbcli-embed-chart.xinference-cluster \
	build-single-kbcli-embed-chart.neon-cluster \
	build-single-kbcli-embed-chart.clickhouse-cluster \
	build-single-kbcli-embed-chart.milvus-cluster \
	build-single-kbcli-embed-chart.qdrant-cluster \
	build-single-kbcli-embed-chart.weaviate-cluster \
	build-single-kbcli-embed-chart.oceanbase-ce-cluster \
	build-single-kbcli-embed-chart.elasticsearch-cluster

.PHONY: kbcli
kbcli: build-checks kbcli-fast ## Build bin/kbcli.

.PHONY: clean-kbcli
clean-kbcli: ## Clean bin/kbcli*.
	rm -f bin/kbcli*

.PHONY: kbcli-doc
kbcli-doc: ## generate CLI command reference manual.

	$(GO) run -tags $(BUILD_TAGS) ./hack/docgen/cli/main.go ./docs/user_docs/cli

.PHONY: install-docker-buildx
install-docker-buildx: ## Create `docker buildx` builder.
	@if ! docker buildx inspect $(BUILDX_BUILDER) > /dev/null; then \
		echo "Buildx builder $(BUILDX_BUILDER) does not exist, creating..."; \
		docker buildx create --name=$(BUILDX_BUILDER) --use --driver=docker-container --platform linux/amd64,linux/arm64; \
	else \
		echo "Buildx builder $(BUILDX_BUILDER) already exists"; \
	fi

.PHONY: golangci
golangci: GOLANGCILINT_VERSION = v1.55.2
golangci: ## Download golangci-lint locally if necessary.
ifneq ($(shell which golangci-lint),)
	@echo golangci-lint is already installed
GOLANGCILINT=$(shell which golangci-lint)
else ifeq (, $(shell which $(GOBIN)/golangci-lint))
	@{ \
	set -e ;\
	echo 'installing golangci-lint-$(GOLANGCILINT_VERSION)' ;\
	curl -sSfL $(GITHUB_PROXY)https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) $(GOLANGCILINT_VERSION) ;\
	echo 'Successfully installed' ;\
	}
GOLANGCILINT=$(GOBIN)/golangci-lint
else
	@echo golangci-lint is already installed
GOLANGCILINT=$(GOBIN)/golangci-lint
endif

.PHONY: staticchecktool
staticchecktool: ## Download staticcheck locally if necessary.
ifeq (, $(shell which staticcheck))
	@{ \
	set -e ;\
	echo 'installing honnef.co/go/tools/cmd/staticcheck' ;\
	go install honnef.co/go/tools/cmd/staticcheck@latest;\
	}
STATICCHECK=$(GOBIN)/staticcheck
else
STATICCHECK=$(shell which staticcheck)
endif

.PHONY: goimportstool
goimportstool: ## Download goimports locally if necessary.
ifeq (, $(shell which goimports))
	@{ \
	set -e ;\
	go install golang.org/x/tools/cmd/goimports@latest ;\
	}
GOIMPORTS=$(GOBIN)/goimports
else
GOIMPORTS=$(shell which goimports)
endif

.PHONY: cuetool
cuetool: ## Download cue locally if necessary.
ifeq (, $(shell which cue))
	@{ \
	set -e ;\
	go install cuelang.org/go/cmd/cue@$(CUE_VERSION) ;\
	}
CUE=$(GOBIN)/cue
else
CUE=$(shell which cue)
endif

.PHONY: helmtool
helmtool: ## Download helm locally if necessary.
ifeq (, $(shell which helm))
	@{ \
	set -e ;\
	echo 'installing helm' ;\
	curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash;\
	echo 'Successfully installed' ;\
	}
HELM=$(GOBIN)/helm
else
HELM=$(shell which helm)
endif

.PHONY: kubectl
kubectl: ## Download kubectl locally if necessary.
ifeq (, $(shell which kubectl))
	@{ \
	set -e ;\
	echo 'installing kubectl' ;\
	curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/$(GOOS)/$(GOARCH)/kubectl" && chmod +x kubectl && sudo mv kubectl /usr/local/bin ;\
	echo 'Successfully installed' ;\
	}
endif
KUBECTL=$(shell which kubectl)

.PHONY: fix-license-header
fix-license-header: ## Run license header fix.
	@./hack/license/header-check.sh fix

# NOTE: include must be placed at the end
include docker/docker.mk
