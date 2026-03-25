BINARY   := virtiodp
IMAGE    := quay.io/amorenoz/virtio-device-plugin
TAG      ?= latest
GOFLAGS  ?= -trimpath

GOLANGCI_LINT_VERSION ?= v2.11.4

.PHONY: build test lint clean image e2e

build:
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/virtiodp

test:
	go test ./pkg/...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

image:
	docker build -t $(IMAGE):$(TAG) .

e2e: IMAGE=localhost/virtio-device-plugin
e2e: TAG=e2e
e2e: image
	go test -v -count=1 -timeout 10m ./test/e2e/
