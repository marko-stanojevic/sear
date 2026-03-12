MODULE  := github.com/sear-project/sear
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -s -w"

BINARIES := sear-daemon sear-client
PLATFORMS := linux/amd64 linux/arm64 windows/amd64 darwin/amd64 darwin/arm64

.PHONY: all build release test lint clean tidy

all: build

## build: compile binaries for the current platform
build:
	@mkdir -p bin
	go build $(LDFLAGS) -o bin/sear-daemon ./cmd/sear-daemon
	go build $(LDFLAGS) -o bin/sear-client ./cmd/sear-client
	@echo "Built: bin/sear-daemon  bin/sear-client"

## release: cross-compile for all platforms into dist/
release: tidy
	@mkdir -p dist
	@$(foreach PLATFORM,$(PLATFORMS), \
		$(eval OS   := $(word 1,$(subst /, ,$(PLATFORM)))) \
		$(eval ARCH := $(word 2,$(subst /, ,$(PLATFORM)))) \
		$(eval EXT  := $(if $(filter windows,$(OS)),.exe,)) \
		$(foreach BIN,$(BINARIES), \
			echo "Building $(BIN) → $(OS)/$(ARCH)…"; \
			GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) \
				-o dist/$(BIN)-$(OS)-$(ARCH)$(EXT) \
				./cmd/$(BIN); \
		) \
	)
	@echo "\nRelease artifacts in dist/:"
	@ls -lh dist/

## test: run all tests
test:
	go test ./... -v -count=1

## test-race: run tests with race detector
test-race:
	go test ./... -race -count=1

## lint: run golangci-lint (requires golangci-lint installed)
lint:
	golangci-lint run ./...

## tidy: update go.sum
tidy:
	go mod tidy

## clean: remove build artifacts
clean:
	rm -rf bin/ dist/
