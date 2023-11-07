#
# Copyright 2022 The Kbcli Authors
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
VERSION ?= 0.8.0-alpha.0
GIT_COMMIT  = $(shell git rev-list -1 HEAD)
GIT_VERSION = $(shell git describe --always --abbrev=0 --tag)
KB_ADDONS_HELM_REPO = kubeblocks-addon
KB_ADDONS_HELM_REPO_URL = https://jihulab.com/api/v4/projects/150246/packages/helm/stable
CONTROLLER_TOOLS_VERSION ?= v0.12.1
LOCALBIN ?= $(shell pwd)/bin
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen


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

##@ CLI
K3S_VERSION ?= v1.23.8+k3s1
K3D_VERSION ?= 5.4.4
K3S_IMG_TAG ?= $(subst +,-,$(K3S_VERSION))

CLI_LD_FLAGS ="-s -w \
	-X github.com/apecloud/kbcli/version.BuildDate=`date -u +'%Y-%m-%dT%H:%M:%SZ'` \
	-X github.com/apecloud/kbcli/version.GitCommit=$(GIT_COMMIT) \
	-X github.com/apecloud/kbcli/version.GitVersion=$(GIT_VERSION) \
	-X github.com/apecloud/kbcli/version.Version=$(VERSION) \
	-X github.com/apecloud/kbcli/version.K3sImageTag=$(K3S_IMG_TAG) \
	-X github.com/apecloud/kbcli/version.K3dVersion=$(K3D_VERSION) \
	-X github.com/apecloud/kbcli/version.DefaultKubeBlocksVersion=$(VERSION)"

bin/kbcli.%:  ## Cross build bin/kbcli.$(OS).$(ARCH).
	GOOS=$(word 2,$(subst ., ,$@)) GOARCH=$(word 3,$(subst ., ,$@)) $(GO) build -tags $(BUILD_TAGS) -ldflags=${CLI_LD_FLAGS} -o $@ cmd/cli/main.go

.PHONY: kbcli
kbcli: OS=$(shell $(GO) env GOOS)
kbcli: ARCH=$(shell $(GO) env GOARCH)
kbcli: build-kbcli-embed-chart
	$(MAKE) bin/kbcli.$(OS).$(ARCH)
	@mv bin/kbcli.$(OS).$(ARCH) bin/kbcli

create-kbcli-embed-charts-dir:
	mkdir -p internal/cluster/charts/

build-single-kbcli-embed-chart.%: chart=$(word 2,$(subst ., ,$@))
build-single-kbcli-embed-chart.%:
	$(HELM) pull $(KB_ADDONS_HELM_REPO)/$(chart)
	mv $(chart)-*.tgz internal/cluster/charts/$(chart).tgz


.PHONY: build-kbcli-embed-chart
build-kbcli-embed-chart: helmtool create-kbcli-embed-charts-dir \
	build-single-kbcli-embed-chart.apecloud-mysql-cluster \
	build-single-kbcli-embed-chart.redis-cluster \
	build-single-kbcli-embed-chart.postgresql-cluster \
	build-single-kbcli-embed-chart.kafka-cluster \
	build-single-kbcli-embed-chart.mongodb-cluster \
	build-single-kbcli-embed-chart.llm-cluster \
#	build-single-kbcli-embed-chart.neon-cluster
#	build-single-kbcli-embed-chart.postgresql-cluster \
#	build-single-kbcli-embed-chart.clickhouse-cluster \
#	build-single-kbcli-embed-chart.milvus-cluster \
#	build-single-kbcli-embed-chart.qdrant-cluster \
#	build-single-kbcli-embed-chart.weaviate-cluster


.PHONY: clean-kbcli
clean-kbcli: ## Clean bin/kbcli*.
	rm -f bin/kbcli*

.PHONY: kbcli-doc
kbcli-doc:   ## generate CLI command reference manual.
	$(GO) run -tags $(BUILD_TAGS) ./hack/docgen/cli/main.go ./docs/user_docs/cli

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

.PHONY: check_helm_repo
check_helm_repo: helmtool
	@if ! $(HELM) repo list | grep -q $(KB_ADDONS_HELM_REPO); then \
		$(HELM) repo add $(KB_ADDONS_HELM_REPO) $(KB_ADDONS_HELM_REPO_URL); \
	fi


.PHONY: staticcheck
staticcheck: staticchecktool  ## Run staticcheck against code.
	$(STATICCHECK) -tags $(BUILD_TAGS) ./...



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

.PHONY: vet
vet: ## Run go vet against code.
	GOOS=$(GOOS) $(GO) vet -tags $(BUILD_TAGS) -mod=mod ./...

.PHONY: fmt
fmt: ## Run go fmt against code.
	$(GOFMT) -l -w -s $$(git ls-files --exclude-standard | grep "\.go$$" )


.PHONY: golangci-lint
golangci-lint: golangci  ## Run golangci-lint against code.
	$(GOLANGCILINT) run ./...


.PHONY: golangci
golangci: GOLANGCILINT_VERSION = v1.54.2
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


.PHONY: lint
lint: staticcheck vet golangci-lint