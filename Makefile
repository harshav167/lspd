GO ?= go

.PHONY: build test race vet integration

build:
	$(GO) build ./...

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

integration:
	$(GO) test -race -tags integration ./test/integration/...
