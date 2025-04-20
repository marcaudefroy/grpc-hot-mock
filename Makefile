BINARY := grpc-hot-mock
CMD_PATH := ./cmd

GRPC_PORT ?= :50051
HTTP_PORT ?= :8080

GOLANGCI_LINT_VERSION := v1.56.0

GOBIN ?= $(shell go env GOPATH)/bin

.PHONY: run ensure-golangci-lint lint build test ci

run:
	go run $(CMD_PATH) \
		--grpc_port=$(GRPC_PORT) \
		--http_port=$(HTTP_PORT)

ensure-golangci-lint:
	@if ! command -v golangci-lint >/dev/null; then \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) $(GOLANGCI_LINT_VERSION); \
	else \
		echo "golangci-lint already installed"; \
	fi

lint: ensure-golangci-lint
	golangci-lint run

build:
	@mkdir -p bin
	go build -o bin/$(BINARY) $(CMD_PATH)

test:
	go test ./...

ci: lint test build

