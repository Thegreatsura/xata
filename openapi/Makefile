DOCKER_FLAGS=--rm --user $(shell id -u):$(shell id -g)
DOCKER_API_TOOLS?=docker run $(DOCKER_FLAGS) -v $(PWD):/data openapi-tools
REDOCLY_CLI=$(DOCKER_API_TOOLS) redocly --config ./redocly/config.yaml
OPENAPI_FORMAT=$(DOCKER_API_TOOLS) openapi-format --no-bundle

.PHONY: help
help:  ## This help dialog.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m\033[0m\n"} /^[$$()% 0-9a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: generate
generate: tools lint generate-bundled generate-bundled-json generate-public generate-public-json# Generate creates all generated files (bundles, public API, etc)
	@echo "Done"

generate-bundled:
	for f in $(shell ls *.yaml); do \
		$(REDOCLY_CLI) bundle -o gen/bundled/$$f $$f; \
	done

generate-public:
	$(REDOCLY_CLI) join common/public-info.yaml gen/bundled/* -o gen/public.yaml
	$(REDOCLY_CLI) bundle -o gen/public.yaml gen/public.yaml
	yq -i '.security += [{"oidc": []}, {"apiKey": []}, {"xata": []}]' gen/public.yaml

tools: $(shell find ../dev/docker/openapi-tools -type f)  ## Install/Build tools
	cd ../dev/docker/openapi-tools && $(MAKE)

# lint checks the openapi.yaml file for correctness and formatting
.PHONY: lint lint-spec lint-fmt
lint: lint-spec   ## Lint openapi spec
lint-spec: tools
	$(REDOCLY_CLI) lint auth projects webhook gateway

.PHONY: fmt
fmt: tools ## Format specs
	@for f in $(shell ls *.yaml ./common/*.yaml); do $(OPENAPI_FORMAT) --yaml -o $${f} $${f}; done


.PHONY: generate-bundled-json
generate-bundled-json: tools ## Generate JSON versions of bundled specs
	for f in $(shell ls *.yaml); do \
		$(REDOCLY_CLI) bundle --ext json -o gen/bundled-json/$${f%.yaml}.json $$f; \
	done
.PHONY: generate-public-json
generate-public-json: tools ## Generate joined public OpenAPI spec in JSON format
		$(REDOCLY_CLI) join common/public-info.yaml gen/bundled-json/*.json -o gen/openapi.json && rm -rf gen/bundled-json
		$(REDOCLY_CLI) bundle --ext json -o gen/openapi.json gen/openapi.json
