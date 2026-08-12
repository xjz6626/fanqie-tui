BINARY := build/fanqie
VERSION ?= 0.2.0
GOFLAGS := -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION)
CGO_ENABLED ?= 0
export CGO_ENABLED

.PHONY: all build install test vet fmt clean

all: test build

build:
	@mkdir -p build
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/fanqie

install:
	./install.sh

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal

clean:
	$(RM) $(BINARY) coverage.out
