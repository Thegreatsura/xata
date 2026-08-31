SHELL := /bin/bash
GOVERSION := $(shell go version | awk '{print $$3}')
GO := GOTOOLCHAIN=$(GOVERSION) go
BUF := $(GO) run github.com/bufbuild/buf/cmd/buf
GOLANGCI := $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint
GOMPLATE := $(GO) run github.com/hairyhenderson/gomplate/v4/cmd/gomplate@latest
DOCKER_FLAGS=--rm --user $(shell id -u):$(shell id -g)
DOCKER_OPA := docker run $(DOCKER_FLAGS) -v $(PWD)/internal/opa:/policy openpolicyagent/opa:latest
DOCKER_JQ := docker run $(DOCKER_FLAGS) -v $(PWD):/data -w /data jq-tools
DEV_API_KEY := kubectl -n xata exec deploy/auth -c auth -- /server create_dev_api_key
GIT_COMMIT_SHORT := $(shell git rev-parse --short=7 HEAD)
SOURCE_URL := $(or $(GITHUB_SERVER_URL),https://github.com)/$(or $(GITHUB_REPOSITORY),xataio/maki)
WORKFLOW_FILES := $(wildcard .github/workflows/* oss/.github/workflows/*)
BAKE_OVERRIDES := docker-bake.override.hcl $(wildcard oss/docker-bake.override.hcl)
CHART_DIRS ?= charts $(wildcard saas-charts)
GIT_TOKEN ?=
export SOURCE_URL GIT_TOKEN

.PHONY: help
help:  ## This help dialog.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m\033[0m\n"} /^[$$()% 0-9a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: machine
machine: ## Launch a personal EC2 machine
	@pulumi env run -i xata/default/account-sandbox -- ./dev/scripts/machine.sh

.PHONY: machine-start
machine-start: ## Start your personal EC2 machine
	@pulumi env run -i xata/default/account-sandbox -- ./dev/scripts/machine.sh start

.PHONY: machine-stop
machine-stop: ## Stop your personal EC2 machine
	@pulumi env run -i xata/default/account-sandbox -- ./dev/scripts/machine.sh stop

.PHONY: machine-destroy
machine-destroy: ## Destroy your personal EC2 machine
	@pulumi env run -i xata/default/account-sandbox -- ./dev/scripts/machine.sh destroy

.PHONY: check
check: lint check-playbooks  ## CI code checks

.PHONY: lint
lint: lint-openapi lint-go lint-buf lint-opa lint-keycloak-turnstile lint-charts lint-kube ## Lint source code
	@echo "All lint tasks completed at $$(date)"

.PHONY: lint-charts
lint-charts: ## Lint Helm charts
	@cd charts && $(MAKE) lint

.PHONY: lint-kube
lint-kube: ## Lint Kubernetes manifests
	@cd kustomize && $(MAKE) lint

.PHONY: lint-openapi
lint-openapi: ## Lint OpenAPI code
	@cd openapi && $(MAKE) lint

.PHONY: lint-go
lint-go: ## Lint Go code
	@$(GOLANGCI) run ./...

.PHONY: lint-buf
lint-buf:
	@$(BUF) lint


.PHONY: lint-opa
lint-opa:
	@$(DOCKER_OPA) check /policy

.PHONY: lint-keycloak-turnstile
lint-keycloak-turnstile: ## Lint Keycloak Turnstile plugin (Kotlin)
	@cd dev/docker/keycloak/keycloak-turnstile && $(MAKE) lint

.PHONY: lint-workflows
lint-workflows: ## Lint GitHub Actions workflows
	@command -v actionlint >/dev/null 2>&1 || $(GO) install github.com/rhysd/actionlint/cmd/actionlint@latest
	@actionlint $(WORKFLOW_FILES)

.PHONY: lint-bake
lint-bake: ## Validate private and OSS Docker Bake definitions
	@set -e; \
	for bake_override in $(BAKE_OVERRIDES); do \
		docker buildx bake --file docker-bake.hcl --file "$$bake_override" --check; \
		docker buildx bake --file docker-bake.hcl --file "$$bake_override" --print >/dev/null; \
	done

.PHONY: fmt
fmt: tools fmt-openapi fmt-go fmt-buf fmt-opa fmt-json fmt-keycloak-turnstile ## Format source code
	@echo "All format tasks completed at $$(date)"

.PHONY: fmt-openapi
fmt-openapi:
	@cd openapi && $(MAKE) fmt

.PHONY: fmt-go
fmt-go: ## Format Go code (use FILES="path1 path2" for specific files/dirs)
	@FMT_ARGS=$$(if [ -z "$(FILES)" ]; then echo "."; else echo "$(FILES)"; fi); \
	$(GO) run mvdan.cc/gofumpt -w -modpath xata $$FMT_ARGS

.PHONY: fmt-buf
fmt-buf:
	@$(BUF) format -w

.PHONY: fmt-opa
fmt-opa:
	@$(DOCKER_OPA) fmt -w /policy

.PHONY: fmt-json
fmt-json:
	@$(DOCKER_JQ) jq -L /jq -f /jq/clean-realm.jq charts/keycloak/files/realm.json > charts/keycloak/files/realm.json.tmp && mv charts/keycloak/files/realm.json.tmp charts/keycloak/files/realm.json

.PHONY: fmt-keycloak-turnstile
fmt-keycloak-turnstile: ## Format Keycloak Turnstile plugin (Kotlin)
	@cd dev/docker/keycloak/keycloak-turnstile && $(MAKE) fmt

.PHONY: generate
generate: generate-openapi generate-buf generate-go generate-agents generate-playbooks ## Generate code
	@echo "All generate tasks completed at $$(date)"

.PHONY: generate-openapi
generate-openapi:
	@cd openapi && $(MAKE) generate

.PHONY: generate-buf
generate-buf:
	@$(BUF) generate

.PHONY: generate-go
generate-go: ## Generate Go code (use FILES="path1 path2" for specific files/dirs)
	@GEN_ARGS=$$(if [ -z "$(FILES)" ]; then echo "./..."; else echo "$(FILES)"; fi); \
	GODEBUG=gotypesalias=0 $(GO) generate $$GEN_ARGS

.PHONY: generate-agents
generate-agents: ## Generate agent files
	cp AGENTS.md CLAUDE.md

.PHONY: generate-playbooks
generate-playbooks: ## Generate the playbooks index
	@$(GOMPLATE) -f playbooks/README.md.tmpl -o playbooks/README.md

.PHONY: check-playbooks
check-playbooks: ## Check that the playbooks index is generated
	@tmp=$$(mktemp); \
	trap 'rm -f "$$tmp"' EXIT; \
	$(GOMPLATE) -f playbooks/README.md.tmpl -o "$$tmp"; \
	diff -u playbooks/README.md "$$tmp"

.PHONY: test
test: ## Run unit and integration tests
	$(GO) test -coverprofile=coverage -timeout 5m -race -failfast -v ./...
	$(DOCKER_OPA) test /policy
	@cd dev/docker/keycloak/keycloak-turnstile && $(MAKE) test

.PHONY: test-e2e
test-e2e:
	$(GO) test -tags=e2e -timeout 5m -v ./e2e/...

.PHONY: test-e2e-local
test-e2e-local: ## Run the e2e tests against the local cluster, on a freshly minted dev API key
	@key=$$($(DEV_API_KEY)) && XATA_E2E_API_KEY=$$key $(MAKE) test-e2e

tools: $(shell find ./dev/docker/jq-tools -type f)  ## Install/Build tools
	cd ./dev/docker/jq-tools && $(MAKE)

.PHONY: push-image
push-image: ## Build and push a Bake target. Requires TARGET and comma-separated DESTINATIONS. Optional: TAG_AS_LATEST, GIT_TOKEN, SOURCE_URL, OUTPUT_FILE.
	@set -euo pipefail; \
	if [[ -z "$(TARGET)" || -z "$(DESTINATIONS)" ]]; then \
		echo "TARGET and DESTINATIONS are required" >&2; exit 1; \
	fi; \
	metadata=$$(mktemp); \
	trap 'rm -f "$$metadata"' EXIT; \
	DESTINATIONS="$(DESTINATIONS)" TAG="$(GIT_COMMIT_SHORT)" LATEST="$(or $(TAG_AS_LATEST),false)" \
		docker buildx bake "$(TARGET)" --push --provenance=false --progress=plain --metadata-file "$$metadata"; \
	digest=$$(jq -r --arg target "$(TARGET)" '.[$$target]["containerimage.digest"]' "$$metadata"); \
	destinations="$(DESTINATIONS)"; \
	image="$${destinations%%,*}/$(TARGET)"; \
	if [[ -n "$(OUTPUT_FILE)" ]]; then \
		printf 'image=%s\ndigest=%s\n' "$$image" "$$digest" >> "$(OUTPUT_FILE)"; \
	else \
		printf 'image=%s\ndigest=%s\n' "$$image" "$$digest"; \
	fi

.PHONY: inject-chart-digests
inject-chart-digests: ## Inject IMAGE_SPECS into charts. Optional: CHART_DIRS.
	@set -e; \
	while IFS= read -r line; do \
		[[ -z "$$line" ]] && continue; \
		read -r image_spec digest_spec <<< "$$line"; \
		image="$${image_spec#IMAGE=}"; digest="$${digest_spec#DIGEST=}"; \
		for chart_dir in $(CHART_DIRS); do \
			$(MAKE) -C "$$chart_dir" inject-digest IMAGE="$$image" DIGEST="$$digest"; \
		done; \
	done <<< "$$IMAGE_SPECS"

.PHONY: publish-charts
publish-charts: ## Package and push charts. Optional: CHART_DIRS, CHARTS, CHART_REGISTRY.
	@set -e; \
	for chart_dir in $(CHART_DIRS); do \
		$(MAKE) -C "$$chart_dir" push-charts; \
	done

.PHONY: chart-info
chart-info: ## Print chart metadata as JSON. Optional: CHART_DIRS, CHARTS, CHART_REGISTRY, OUTPUT_FILE.
	@set -euo pipefail; \
	info=$$(for chart_dir in $(CHART_DIRS); do $(MAKE) -C "$$chart_dir" --no-print-directory chart-info; done); \
	charts=$$(jq -Rsc 'split("\n") | map(select(length > 0) | split("\t")) | map({name: .[0], repo: .[1], version: .[2]})' <<< "$$info"); \
	if [[ -n "$(OUTPUT_FILE)" ]]; then \
		printf 'json=%s\n' "$$charts" >> "$(OUTPUT_FILE)"; \
	else \
		printf '%s\n' "$$charts"; \
	fi

.PHONY: get-pr-info
get-pr-info: ## Get PR info for a commit (requires COMMIT=<sha> REPO=<owner/repo>)
	@if [ -z "$(COMMIT)" ] || [ -z "$(REPO)" ]; then \
		echo '{"error": "COMMIT and REPO are required"}'; \
		exit 1; \
	fi; \
	gh api "repos/$(REPO)/commits/$(COMMIT)/pulls" 2>/dev/null | \
		jq -c 'if .[0] then {number: .[0].number, url: .[0].html_url} else {} end' || \
		echo '{}'
