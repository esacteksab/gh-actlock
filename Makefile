MAKEFLAGS += --warn-undefined-variables
SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := all
.DELETE_ON_ERROR:
.SUFFIXES:

.PHONY: audit
audit: tidy fmt
	go vet ./...
	go tool -modfile=go.tool.mod staticcheck ./...
	go tool -modfile=go.tool.mod govulncheck ./...
	golangci-lint run -v


.PHONY: build
build:

	goreleaser build --clean --single-target --snapshot

.PHONY: clean
clean:
ifneq (,$(wildcard ./dist))
	rm -rf dist/

endif

ifneq (,$(wildcard ./coverage))
	rm -rf coverage/

endif

.PHONY: container
container: tidy
	./scripts/build-container.sh

.PHONY: fmt
fmt:
	go tool -modfile=go.tool.mod golines --base-formatter=gofumpt -w .
	go tool -modfile=go.tool.mod gofumpt -l -w -extra .

.PHONY: lint
lint:
	golangci-lint run -v

.PHONY: test
test:
	go test ./... -cover

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: update
update:
	go get -u ./...
	go mod tidy
