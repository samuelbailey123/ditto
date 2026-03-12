.PHONY: build test test-race lint fmt install clean coverage

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  = -s -w -X github.com/samuelbailey123/ditto/internal/version.Version=$(VERSION) \
           -X github.com/samuelbailey123/ditto/internal/version.Commit=$(COMMIT) \
           -X github.com/samuelbailey123/ditto/internal/version.Date=$(DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o ditto ./cmd/ditto

test:
	go test ./... -count=1

test-race:
	go test ./... -count=1 -race

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .
	goimports -w .

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/ditto

clean:
	rm -f ditto coverage.out coverage.html
