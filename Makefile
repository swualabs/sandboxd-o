SHELL := /usr/bin/env bash

BIN_DIR := ./build
SANDBOXD_BIN := $(BIN_DIR)/sandboxd
ORCH_BIN := $(BIN_DIR)/orchestrator
SWAG := $(or $(shell command -v swag 2>/dev/null),$(shell go env GOPATH)/bin/swag)

SANDBOXD_CMD := ./cmd/sandboxd
ORCH_CMD := ./cmd/orchestrator

.PHONY: help fmt vet test test-cover build build-sandboxd build-orchestrator clean swagger swagger-sandboxd swagger-orchestrator \
	run-sandboxd run-orchestrator install \
	start-sandboxd stop-sandboxd start-orchestrator stop-orchestrator

help:
	@echo "Targets:"
	@echo "  make fmt                - gofmt all go files"
	@echo "  make vet                - go vet ./..."
	@echo "  make test               - go test ./..."
	@echo "  make test-cover         - go test with coverage profile (coverage.out)"
	@echo "  make build              - build sandboxd + orchestrator"
	@echo "  make build-sandboxd     - build sandboxd binary"
	@echo "  make build-orchestrator - build orchestrator binary"
	@echo "  make swagger            - generate swagger docs for sandboxd + orchestrator"
	@echo "  make swagger-sandboxd   - generate swagger docs for sandboxd"
	@echo "  make swagger-orchestrator - generate swagger docs for orchestrator"
	@echo "  make run-sandboxd       - run sandboxd"
	@echo "  make run-orchestrator   - run orchestrator"
	@echo "  make clean              - remove build artifacts"
	@echo "  make install            - run scripts/install.sh"
	@echo "  make start-sandboxd     - start sandboxd in background (/tmp/sandboxd.log)"
	@echo "  make stop-sandboxd      - stop background sandboxd"
	@echo "  make start-orchestrator - start orchestrator in background (/tmp/orchestrator.log)"
	@echo "  make stop-orchestrator  - stop background orchestrator"

fmt:
	@go fmt ./...

vet:
	@go vet ./...

test:
	@go test ./...

test-cover:
	@go test -p=1 -v -coverprofile=coverage.out ./...

build: build-sandboxd build-orchestrator

build-sandboxd:
	@mkdir -p $(BIN_DIR)
	@go build -o $(SANDBOXD_BIN) $(SANDBOXD_CMD)

build-orchestrator:
	@mkdir -p $(BIN_DIR)
	@go build -o $(ORCH_BIN) $(ORCH_CMD)

swagger: swagger-sandboxd swagger-orchestrator

swagger-sandboxd:
	@$(SWAG) init -g main.go -d cmd/sandboxd,sandboxd/http,sandboxd/model,sandboxd/config -o sandboxd/docs --instanceName sandboxd --parseDependency --parseInternal

swagger-orchestrator:
	@$(SWAG) init -g main.go -d cmd/orchestrator,orchestrator/http,orchestrator/http/handlers,orchestrator/service,orchestrator/types,orchestrator/config -o orchestrator/docs --instanceName orchestrator --parseDependency --parseInternal

clean:
	@rm -rf $(BIN_DIR)

run-sandboxd:
	@go run $(SANDBOXD_CMD)

run-orchestrator:
	@go run $(ORCH_CMD)

install:
	@./scripts/install.sh

start-sandboxd:
	@nohup go run $(SANDBOXD_CMD) >/tmp/sandboxd.log 2>&1 & echo $$! >/tmp/sandboxd.pid
	@echo "started sandboxd pid=$$(cat /tmp/sandboxd.pid), log=/tmp/sandboxd.log"

stop-sandboxd:
	@if [[ -f /tmp/sandboxd.pid ]]; then \
		kill "$$(cat /tmp/sandboxd.pid)" && rm -f /tmp/sandboxd.pid && echo "stopped sandboxd"; \
	else \
		echo "no sandboxd pid file"; \
	fi

start-orchestrator:
	@nohup go run $(ORCH_CMD) >/tmp/orchestrator.log 2>&1 & echo $$! >/tmp/orchestrator.pid
	@echo "started orchestrator pid=$$(cat /tmp/orchestrator.pid), log=/tmp/orchestrator.log"

stop-orchestrator:
	@if [[ -f /tmp/orchestrator.pid ]]; then \
		kill "$$(cat /tmp/orchestrator.pid)" && rm -f /tmp/orchestrator.pid && echo "stopped orchestrator"; \
	else \
		echo "no orchestrator pid file"; \
	fi
