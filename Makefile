PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
GO ?= go
BIN ?= dockan
VERSION ?= dev
LDFLAGS ?= -s -w -X main.version=$(VERSION)
PLATFORMS ?= linux/amd64 linux/arm64 linux/arm/v7 linux/386 linux/riscv64

.PHONY: build test install clean release packages

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/dockan.go

test:
	CGO_ENABLED=0 $(GO) test ./...

install: build
	install -d "$(DESTDIR)$(BINDIR)"
	install -m 0755 "$(BIN)" "$(DESTDIR)$(BINDIR)/$(BIN)"

clean:
	rm -rf dist "$(BIN)"

release:
	rm -rf dist
	mkdir -p dist
	for platform in $(PLATFORMS); do \
		os=$${platform%%/*}; \
		arch_variant=$${platform#*/}; \
		arch=$${arch_variant%%/*}; \
		arm=$${arch_variant#*/}; \
		goarm=$${arm#v}; \
		out="dist/$(BIN)-$${os}-$${arch}"; \
		if [ "$${arch}" = "arm" ] && [ "$${arm}" != "$${arch}" ]; then out="$${out}v$${goarm}"; fi; \
		if [ "$${arch}" = "arm" ] && [ "$${arm}" != "$${arch}" ]; then \
			GOOS=$${os} GOARCH=$${arch} GOARM=$${goarm} CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o "$${out}" ./cmd/dockan.go; \
		else \
			GOOS=$${os} GOARCH=$${arch} CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o "$${out}" ./cmd/dockan.go; \
		fi; \
	done

packages: release
	VERSION="$(VERSION)" sh scripts/package.sh
