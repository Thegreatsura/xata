GOVERSION := $(shell go version | awk '{print $$3}')
GO := GOTOOLCHAIN=$(GOVERSION) go
GOLANGCI := $(GO) run github.com/golangci/golangci-lint/cmd/golangci-lint

# docker image build args
SERVICE_NAME ?= $(notdir $(CURDIR))
GIT_COMMIT ?= $(shell git rev-parse HEAD)
DOCKER_REGISTRY ?= docker.io
DOCKER_IMAGE_TAG ?= $(DOCKER_REGISTRY)/xata_$(SERVICE_NAME):$(GIT_COMMIT)

.PHONY: help
help:  ## This help dialog.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m\033[0m\n"} /^[$$()% 0-9a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: check
check: lint  ## CI code checks

.PHONY: lint
lint: ## Lint source code
	$(GOLANGCI) run ./...

.PHONY: fmt
fmt:     ## Format source code
	$(GO) run mvdan.cc/gofumpt -w -modpath xata .

.PHONY: generate
generate: ## Generate code
	$(GO) generate ./...

.PHONY: test
test:   ## Run unit and integration tests
	$(GO) test -timeout 5m -race -failfast -v ./...

docker-build:  ## Build docker image (generic implementation, can be overridden)
	docker build -t $(DOCKER_IMAGE_TAG) --build-arg SERVICE_NAME=$(SERVICE_NAME) -f ../../Dockerfile ../../

docker-push:  ## Push docker image
	docker push $(DOCKER_IMAGE_TAG)
