package plugin

import (
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

// ResourcePool provides the API for a ResourcePool.
// Different device types (e.g: vhost-user, vduse) can provide different implementations.
type ResourcePool interface {
	// ResourceName returns the full k8s extended resource name,
	// e.g. "virtio.openshift.io/vhost-phy0".
	ResourceName() string

	// Devices returns the list of devices to advertise to kubelet.
	Devices() []*pluginapi.Device

	// Allocate performs a device allocation.
	// All necessary host operations (e.g: device creation, permissions, etc)
	// must be done here. Also, the DeviceInfo file must be created.
	// Returns an Allocation describing what to inject into the Pod.
	Allocate(deviceID string) (Allocation, error)

	// Cleanup performs best-effort cleanup of artifacts (devinfo files, etc.)
	// for this resource. Called on plugin shutdown and error paths.
	Cleanup() error
}

// Allocation describes what to inject into a Pod for a single device.
type Allocation interface {
	// Mounts returns the bind mounts to add to the Pod.
	Mounts() []*pluginapi.Mount

	// DeviceSpecs returns the device nodes to expose in the Pod.
	DeviceSpecs() []*pluginapi.DeviceSpec

	// Annotations returns annotations to add to the Pod.
	Annotations() map[string]string
}

// Stub implementations.
type StubAllocation struct{}

func (s *StubAllocation) Mounts() []*pluginapi.Mount {
	return nil
}

func (s *StubAllocation) DeviceSpecs() []*pluginapi.DeviceSpec {
	return nil
}

func (s *StubAllocation) Annotations() map[string]string {
	return nil
}

type StubResourcePool struct{}

func (s *StubResourcePool) ResourceName() string {
	return ""
}

func (s *StubResourcePool) Devices() []*pluginapi.Device {
	return nil
}

func (s *StubResourcePool) Allocate(deviceID string) (Allocation, error) {
	return &StubAllocation{}, nil
}

func (s *StubResourcePool) Cleanup() error {
	return nil
}
