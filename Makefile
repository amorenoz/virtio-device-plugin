BINARY   := virtiodp
IMAGE    := ghcr.io/k8snetworkplumbingwg/virtio-device-plugin
TAG      ?= latest
GOFLAGS  ?= -trimpath

GOLANGCI_LINT_VERSION ?= v2.11.4

.PHONY: build test lint clean image

build:
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/virtiodp

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

image:
	docker build -t $(IMAGE):$(TAG) .
