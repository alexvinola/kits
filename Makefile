GO ?= go
BINARY ?= kits
PLATFORMS ?= darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64
MODULE := github.com/alexvinola/kits

# VERSION overrides what `kits version` reports. Empty keeps the default in
# internal/version.
VERSION ?=
LDFLAGS := $(if $(VERSION),-X '$(MODULE)/internal/version.Version=$(VERSION)',)

.PHONY: all
all: fmt vet test build

.PHONY: fmt
fmt:
	gofmt -w .

.PHONY: fmt-check
fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: test
test:
	$(GO) test ./...

.PHONY: build
build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/kits

.PHONY: cross
cross:
	@set -e; for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "building $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" \
			-o dist/$(BINARY)-$$os-$$arch$$ext ./cmd/kits; \
	done

.PHONY: verify
verify: fmt-check vet test cross

.PHONY: clean
clean:
	rm -rf dist $(BINARY)
