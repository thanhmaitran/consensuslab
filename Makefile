.PHONY: all fmt fmt-check vet test race build smoke experiments web clean

BIN := bin/consensuslab
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

all: fmt-check vet test build

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o $(BIN) ./cmd/consensuslab

smoke: build
	$(BIN) simulate -config configs/smoke.json -nodes 3 -seed 1

experiments: build
	@for f in configs/exp-*.json; do \
	  name=$$(basename $$f .json); \
	  $(BIN) experiment -config $$f -out results/runs/$$name -workers 4 || exit 1; \
	done

web:
	cd web && npm install && npm run build

clean:
	rm -rf bin web/dist
