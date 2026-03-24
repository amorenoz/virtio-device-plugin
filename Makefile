BINARY   := virtiodp
IMAGE    := ghcr.io/k8snetworkplumbingwg/virtio-device-plugin
TAG      ?= latest
GOFLAGS  ?= -trimpath

.PHONY: build test clean image

build:
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/virtiodp

test:
	go test ./...

clean:
	rm -rf bin/

image:
	docker build -t $(IMAGE):$(TAG) .
