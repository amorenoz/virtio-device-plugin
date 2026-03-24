// Package vhost implements the ResourcePool for vhost-user socket resources.
package vhost

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/k8snetworkplumbingwg/virtio-device-plugin/pkg/config"
	"github.com/k8snetworkplumbingwg/virtio-device-plugin/pkg/devinfo"
	"github.com/k8snetworkplumbingwg/virtio-device-plugin/pkg/plugin"
	"github.com/k8snetworkplumbingwg/virtio-device-plugin/pkg/topology"
)

const (
	socketFileName = "vhost.sock"
	devInfoVersion = "1.1.0"
	devInfoType    = "vhost-user"
	vhostUserMode  = "client"
)

// VhostUserResourcePool implements plugin.ResourcePool for vhost-user socket resources.
type VhostUserResourcePool struct {
	resourceName string              // full k8s resource name
	shortName    string              // short resource name (used in filesystem paths)
	baseDir      string              // host directory for socket creation
	numDevices   int                 // number of virtual devices
	devices      []*pluginapi.Device // pre-built device list
	logger       *slog.Logger
	devInfo      devinfo.Store // devinfo file operations
	sysfsBase    string        // sysfs base for topology info, overridable for testing
}

// NewVhostUserResourcePool creates a VhostUserResourcePool from config.
func NewVhostUserResourcePool(cfg *config.PluginConfig, rc *config.ResourceConfig) *VhostUserResourcePool {
	resourceName := config.FullResourceName(cfg, rc)
	pool := &VhostUserResourcePool{
		resourceName: resourceName,
		shortName:    rc.ResourceName,
		baseDir:      rc.BaseDir,
		numDevices:   rc.NumDevices,
		logger:       slog.With("resource", resourceName),
		devInfo:      devinfo.DPStore{},
	}

	pool.devices = pool.buildDevices(rc)

	slog.Info("resource pool created",
		"resource", pool.resourceName,
		"numDevices", pool.numDevices,
		"baseDir", pool.baseDir,
	)

	return pool
}

// --- testing helpers ---

// SetDevInfoStore overrides the devinfo store. Intended for testing.
func (p *VhostUserResourcePool) setDevInfoStore(store devinfo.Store) {
	p.devInfo = store
}

// SetSysfsBase overrides the sysfs base for topology resolution. Intended for testing.
func (p *VhostUserResourcePool) setSysfsBase(dir string) {
	p.sysfsBase = dir
}

// --- plugin.ResourcePool implementation ---

// ResourceName returns the full k8s extended resource name.
func (p *VhostUserResourcePool) ResourceName() string {
	return p.resourceName
}

// Devices returns the pre-built device list.
func (p *VhostUserResourcePool) Devices() []*pluginapi.Device {
	return p.devices
}

// Allocate prepares the host for a vhost-user device assignment.
// The DP writes the devinfo file and returns the mount path.
// The actual directory and socket are created by the CNI.
func (p *VhostUserResourcePool) Allocate(deviceID string) (plugin.Allocation, error) {
	dir := p.socketFileDir(deviceID)
	socketPath := p.socketFilePath(deviceID)
	devInfo := &nadv1.DeviceInfo{
		Type:    devInfoType,
		Version: devInfoVersion,
		VhostUser: &nadv1.VhostDevice{
			Mode: vhostUserMode,
			Path: socketPath,
		},
	}

	// Write the DeviceInfo file.
	// Clean first to ensure idempotency (SaveDeviceInfoForDP refuses to overwrite).
	if err := p.devInfo.Clean(p.resourceName, deviceID); err != nil {
		slog.Warn("failed to clean existing DP devinfo", "deviceID", deviceID, "error", err)
	}
	if err := p.devInfo.Save(p.resourceName, deviceID, devInfo); err != nil {
		return nil, fmt.Errorf("saving DP devinfo for device %s: %w", deviceID, err)
	}

	p.logger.Info("vhost socket allocated",
		"deviceID", deviceID,
		"dir", dir,
		"socketPath", socketPath,
	)

	return &vhostUserAllocation{dir: dir}, nil
}

// Cleanup removes all DP devinfo files for this resource.
// Per the device-info-spec (§4.1.1): "The Device Plugin is responsible for
// deleting the files it created when it stops running."
func (p *VhostUserResourcePool) Cleanup() error {
	var firstErr error
	for _, dev := range p.devices {
		deviceID := dev.ID
		if err := p.devInfo.Clean(p.resourceName, dev.ID); err != nil {
			p.logger.Warn("failed to clean DP devinfo", "deviceID", deviceID, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// --- internal helpers ---

// buildDevices creates the device list with multi-NUMA topology.
// Device IDs are "0" through "N-1".
func (p *VhostUserResourcePool) buildDevices(rc *config.ResourceConfig) []*pluginapi.Device {
	numaNodes := p.resolveNUMANodes(rc)

	devices := make([]*pluginapi.Device, 0, p.numDevices)
	for i := 0; i < p.numDevices; i++ {
		deviceID := fmt.Sprintf("%d", i)
		dir := p.socketFileDir(deviceID)

		// Skip devices whose directory already exists as they might be in use by the CNI.
		if _, err := os.Stat(dir); err == nil {
			p.logger.Warn("directory already exists, skipping", "deviceID", deviceID, "dir", dir)
			continue
		}

		dev := &pluginapi.Device{
			ID:     deviceID,
			Health: pluginapi.Healthy,
		}
		if len(numaNodes) > 0 {
			dev.Topology = &pluginapi.TopologyInfo{Nodes: numaNodes}
		}
		devices = append(devices, dev)
	}
	return devices
}

// resolveNUMANodes resolves all unique NUMA nodes from the configured PCI addresses.
// All valid NUMA nodes are returned so kubelet can place the pod on any of them.
func (p *VhostUserResourcePool) resolveNUMANodes(rc *config.ResourceConfig) []*pluginapi.NUMANode {
	seen := make(map[int]bool)
	var nodes []*pluginapi.NUMANode

	for _, th := range rc.TopologyHintsFrom {
		node, err := topology.NUMANodeForPCI(p.sysfsBase, th.PCIAddress)
		if err != nil {
			p.logger.Warn("failed to resolve NUMA node", "pciAddress", th.PCIAddress, "error", err)
			continue
		}
		if node >= 0 && !seen[node] {
			seen[node] = true
			nodes = append(nodes, &pluginapi.NUMANode{ID: int64(node)})
			p.logger.Info("NUMA node resolved", "pciAddress", th.PCIAddress, "numaNode", node)
		}
	}

	return nodes
}

func (p *VhostUserResourcePool) socketFileDir(deviceID string) string {
	return filepath.Join(p.baseDir, p.shortName, deviceID)
}

func (p *VhostUserResourcePool) socketFilePath(deviceID string) string {
	return filepath.Join(p.socketFileDir(deviceID), socketFileName)
}

// vhostUserAllocation implements plugin.Allocation for vhost-user devices.
type vhostUserAllocation struct {
	dir string
}

func (a *vhostUserAllocation) Mounts() []*pluginapi.Mount {
	return []*pluginapi.Mount{{
		HostPath:      a.dir,
		ContainerPath: a.dir,
		ReadOnly:      false,
	}}
}

func (a *vhostUserAllocation) DeviceSpecs() []*pluginapi.DeviceSpec {
	return nil
}

func (a *vhostUserAllocation) Annotations() map[string]string {
	return nil
}
