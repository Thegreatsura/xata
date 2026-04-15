DOCKER_FLAGS=--rm -v $(PWD):/build -v keycloak-turnstile-m2-cache:/root/.m2 -w /build
MAVEN_IMAGE=maven:3.9-eclipse-temurin-17

.PHONY: help build test lint fmt clean all

help:  ## This help dialog
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

build: ## Build the plugin JAR
	docker run $(DOCKER_FLAGS) $(MAVEN_IMAGE) mvn clean package -DskipTests

test: ## Run tests
	docker run $(DOCKER_FLAGS) $(MAVEN_IMAGE) mvn test

lint: ## Check code style with ktlint
	docker run $(DOCKER_FLAGS) $(MAVEN_IMAGE) mvn ktlint:check

fmt: ## Format code with ktlint
	docker run $(DOCKER_FLAGS) $(MAVEN_IMAGE) mvn ktlint:format

clean: ## Clean build artifacts
	docker run $(DOCKER_FLAGS) $(MAVEN_IMAGE) mvn clean

all: fmt lint test build ## Run all checks and build
