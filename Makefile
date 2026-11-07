MODULE  := github.com/jaraslaukunin/vynsh
BINARY  := vynsh
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0-dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(MODULE)/internal/cli.Version=$(VERSION) \
	-X $(MODULE)/internal/cli.Commit=$(COMMIT) \
	-X $(MODULE)/internal/cli.Date=$(DATE)

PLATFORMS := darwin-amd64 darwin-arm64 windows-amd64 windows-arm64 linux-amd64 linux-arm64

.PHONY: all build test race vet fmt check install clean release $(PLATFORMS)

all: build

## build: build the binary for the current platform into ./bin
build:
	@mkdir -p bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/vynsh
	@echo "built bin/$(BINARY) ($(VERSION))"

## install: install into GOBIN
install:
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/vynsh

## test: run the unit tests
test:
	go test ./...

## race: run the unit tests with the race detector
race:
	go test -race ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: format the source
fmt:
	gofmt -l -w .

## check: everything CI does
check: vet test
	@test -z "$$(gofmt -l .)" || (echo "gofmt found unformatted files:"; gofmt -l .; exit 1)

## release: cross-compile every supported platform into ./dist
release: $(PLATFORMS)

$(PLATFORMS):
	@mkdir -p dist
	@target="$@"; os=$${target%%-*}; arch=$${target##*-}; ext=""; \
	if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
	echo "building dist/$(BINARY)-$$os-$$arch$$ext"; \
	GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" \
		-o dist/$(BINARY)-$$os-$$arch$$ext ./cmd/vynsh

## clean: remove build output
clean:
	rm -rf bin dist
