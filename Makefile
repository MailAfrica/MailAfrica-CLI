BIN      := mailafrica
GO       ?= go
PKG      := github.com/MailAfrica/MailAfrica-CLI
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build
build:
	$(GO) build -trimpath -ldflags "-X $(PKG)/internal/version.Version=$(VERSION)" -o bin/$(BIN) ./cmd/mailafrica

.PHONY: test
test:
	$(GO) test ./...

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: fmt
fmt:
	gofmt -l -w .

.PHONY: install
install:
	$(GO) install -trimpath ./cmd/mailafrica