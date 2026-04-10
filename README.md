# Virtio Device Plugin

A Kubernetes Device Plugin that allocates virtio control plane devices
for userspace virtio backends (e.g: DPDK).

Currently, the only device type supported is **vhost-user**.

## Features

- Allocates a configurable number of unique host paths to store vhost-user
socket files.
- Adds topology information to resources based on PCI devices to allow the
[integration with the Topology Manager](https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/device-plugins/#device-plugin-integration-with-the-topology-manager)
- **DOES NOT** handle the actual directory creation or its permissions, that
must be handled by the CNI.

## Configuration

This plugin creates device plugin endpoints based on the configurations
given in the config map associated Virtio Device Plugin.

Here is an example json config:

```json
{
    "resourceNamePrefix": "virtio.k8s.io",
    "resourceList": [
        {
            "resourceName": "vhost-phy0",
            "numDevices": 100,
            "baseDir": "/var/run/virtio-dp/",
            "topologyFrom": [
                {
                    "pciAddress": "0000:ab:cd.0"
                }
            ]
        }
    ]
}
```

Top level fields are:

| Field | Required | Description | Type/Defaults | Example/Accepted values |
|---|---|---|---|---|
| "resourcePrefix" | N | Resource name prefix. Should not contain special characters and resemple a DNS name | string Default: "virtio.k8s.io" | "vhost-devices.yourcompany.com" |
| "resourceList" | Y | List of objects defining each resource pool | resourceConfig | {   "resourceName": "vhost-phy0",   "numDevices": 100 } |


Each `resourceConfig` element in the `resourceList` list must contain the following fields:

| Field | Required | Description | Type/Defaults | Example/Accepted values |
|---|---|---|---|---|
| "resourceName" | Y | Endpoint resource name. Should not contain special characters and must be unique | string | "vhost-devs" |
| "numDevices" | Y | Number of devices to allocate. | number | Any integer less than 10k |
| "baseDir" | Y | Base directory where the directories will be created. Should be a valid absolute path | string | "/var/run/virtio-dp" |
| "topologyFrom" | N | List of objects representing PCI devices from which NUMA topology is inherited | object | Example { "pciAddress": "0000:ab:cd.0" } |


# Development

## Building

```sh
# Run Go linter
make lint

# Build binary
make build


# Build binary of a specific component
IMAGE=quay.io/example/virtio-device-plugin TAG=test make image
```


## Testing

Basic Go unit tests can be run with:
```sh
# Run unit tests
make test
```

To run end-to-end tests that use [kind](kind.sigs.k8s.io/) to deploy a cluster and perform some tests, run:

```sh
# Run e2e tests (requires 'kind' installed)
make e2e
```
