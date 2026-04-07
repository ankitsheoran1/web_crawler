.PHONY: build run tidy clean vet fmt test test-race cover help

BINARY      ?= bin/server
MAIN        ?= ./cmd/server
CONFIG      ?= config.yaml
GO          ?= go

help:
	@echo "Targets:"
	@echo "  make build   - compile $(MAIN) to $(BINARY)"
	@echo "  make run     - build and run server (uses CONFIG=$(CONFIG) if set)"
	@echo "  make tidy    - go mod tidy"
	@echo "  make vet     - go vet ./..."
	@echo "  make fmt     - go fmt ./..."
	@echo "  make test    - go test ./... -count=1"
	@echo "  make test-race - go test -race ./... -count=1"
	@echo "  make cover   - go test ./... -coverprofile=coverage.out"
	@echo "  make clean   - remove $(BINARY)"

build:
	@mkdir -p $(dir $(BINARY))
	$(GO) build -o $(BINARY) $(MAIN)

run: build
	CRAWLER_CONFIG=$(CONFIG) ./$(BINARY)

tidy:
	$(GO) mod tidy

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./... -count=1

test-race:
	$(GO) test -race ./... -count=1

cover:
	$(GO) test ./... -count=1 -coverprofile=coverage.out

clean:
	rm -rf bin
