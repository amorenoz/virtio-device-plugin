package vhost

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/k8snetworkplumbingwg/virtio-device-plugin/pkg/config"
	"github.com/k8snetworkplumbingwg/virtio-device-plugin/pkg/devinfo"
)

func newTestPool(t *testing.T, numDevices int) (*VhostUserResourcePool, *devinfo.TestStore, string) {
	t.Helper()
	baseDir := t.TempDir()
	store := devinfo.NewTestStore()

	cfg := &config.PluginConfig{
		ResourcePrefix: "virtio.example.com",
	}
	rc := &config.ResourceConfig{
		ResourceName: "net0",
		NumDevices:   numDevices,
		BaseDir:      baseDir,
	}

	pool := NewVhostUserResourcePool(cfg, rc)
	pool.setDevInfoStore(store)
	return pool, store, baseDir
}

type pciDevice struct {
	address  string
	numaNode *int
}

// Single helper function with variadic NUMA (0 or 1 values)
func pci(address string, numa ...int) pciDevice {
	if len(numa) == 0 {
		return pciDevice{address: address, numaNode: nil}
	}
	return pciDevice{address: address, numaNode: &numa[0]}
}

func createFakeSysfs(t *testing.T, devices ...pciDevice) string {
	t.Helper()
	sysfsBase := t.TempDir()
	for _, dev := range devices {
		dir := filepath.Join(sysfsBase, dev.address)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}

		if dev.numaNode != nil {
			numaContent := fmt.Sprintf("%d\n", *dev.numaNode)
			if err := os.WriteFile(filepath.Join(dir, "numa_node"), []byte(numaContent), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return sysfsBase
}

func TestResourceName(t *testing.T) {
	pool, _, _ := newTestPool(t, 10)

	want := "virtio.example.com/net0"
	if got := pool.ResourceName(); got != want {
		t.Errorf("ResourceName() = %q, want %q", got, want)
	}
}

func TestDevicesCount(t *testing.T) {
	pool, _, _ := newTestPool(t, 5)

	devices := pool.Devices()
	if len(devices) != 5 {
		t.Fatalf("expected 5 devices, got %d", len(devices))
	}
	for i, dev := range devices {
		wantID := fmt.Sprintf("%d", i)
		if dev.ID != wantID {
			t.Errorf("device[%d].ID = %q, want %q", i, dev.ID, wantID)
		}
		if dev.Health != "Healthy" {
			t.Errorf("device[%d].Health = %q, want Healthy", i, dev.Health)
		}
	}
}

func TestDevicesNoTopologyByDefault(t *testing.T) {
	pool, _, _ := newTestPool(t, 3)

	for _, dev := range pool.Devices() {
		if dev.Topology != nil {
			t.Errorf("device %s: expected no topology, got %v", dev.ID, dev.Topology)
		}
	}
}

func TestDevicesWithMultiNUMA(t *testing.T) {
	baseDir := t.TempDir()
	sysfsBase := createFakeSysfs(t,
		pci("0000:ab:00.0", 0),
		pci("0000:cd:00.0", 1))

	cfg := &config.PluginConfig{
		ResourcePrefix: "virtio.example.com",
	}
	rc := &config.ResourceConfig{
		ResourceName: "net0",
		NumDevices:   3,
		BaseDir:      baseDir,
		TopologyHintsFrom: []config.TopologyHint{
			{PCIAddress: "0000:ab:00.0"},
			{PCIAddress: "0000:cd:00.0"},
		},
	}

	pool := NewVhostUserResourcePool(cfg, rc)
	pool.setDevInfoStore(devinfo.NewTestStore())
	pool.setSysfsBase(sysfsBase)
	pool.devices = pool.buildDevices(rc)

	for _, dev := range pool.Devices() {
		if dev.Topology == nil {
			t.Fatalf("device %s: expected topology, got nil", dev.ID)
		}
		if len(dev.Topology.Nodes) != 2 {
			t.Fatalf("device %s: expected 2 NUMA nodes, got %d", dev.ID, len(dev.Topology.Nodes))
		}
		ids := map[int64]bool{}
		for _, n := range dev.Topology.Nodes {
			ids[n.ID] = true
		}
		if !ids[0] || !ids[1] {
			t.Errorf("device %s: expected NUMA nodes 0 and 1, got %v", dev.ID, dev.Topology.Nodes)
		}
	}
}

func TestDevicesDedupNUMA(t *testing.T) {
	baseDir := t.TempDir()
	sysfsBase := createFakeSysfs(t,
		pci("0000:ab:00.0", 0),
		pci("0000:cd:00.0", 0))

	cfg := &config.PluginConfig{
		ResourcePrefix: "virtio.example.com",
	}
	rc := &config.ResourceConfig{
		ResourceName: "net0",
		NumDevices:   2,
		BaseDir:      baseDir,
		TopologyHintsFrom: []config.TopologyHint{
			{PCIAddress: "0000:ab:00.0"},
			{PCIAddress: "0000:cd:00.0"},
		},
	}

	pool := NewVhostUserResourcePool(cfg, rc)
	pool.setDevInfoStore(devinfo.NewTestStore())
	pool.setSysfsBase(sysfsBase)
	pool.devices = pool.buildDevices(rc)

	for _, dev := range pool.Devices() {
		if dev.Topology == nil {
			t.Fatalf("device %s: expected topology", dev.ID)
		}
		if len(dev.Topology.Nodes) != 1 {
			t.Errorf("device %s: expected 1 NUMA node (deduped), got %d", dev.ID, len(dev.Topology.Nodes))
		}
	}
}

func TestDevicesWithMissingNUMAConfig(t *testing.T) {
	baseDir := t.TempDir()
	sysfsBase := createFakeSysfs(t,
		pci("0000:ab:00.0"),
		pci("0000:cd:00.0"))

	cfg := &config.PluginConfig{
		ResourcePrefix: "virtio.example.com",
	}
	rc := &config.ResourceConfig{
		ResourceName: "net0",
		NumDevices:   2,
		BaseDir:      baseDir,
		TopologyHintsFrom: []config.TopologyHint{
			{PCIAddress: "0000:ab:00.0"},
			{PCIAddress: "0000:cd:00.0"},
		},
	}

	pool := NewVhostUserResourcePool(cfg, rc)
	pool.setDevInfoStore(devinfo.NewTestStore())
	pool.setSysfsBase(sysfsBase)
	pool.devices = pool.buildDevices(rc)

	for _, dev := range pool.Devices() {
		if dev.Topology != nil {
			t.Errorf("device %s: expected no topology when NUMA config is missing, got %v", dev.ID, dev.Topology)
		}
	}
}

func TestBuildDevicesSkipsExistingDirectories(t *testing.T) {
	baseDir := t.TempDir()

	cfg := &config.PluginConfig{
		ResourcePrefix: "virtio.example.com",
	}
	rc := &config.ResourceConfig{
		ResourceName: "net0",
		NumDevices:   5,
		BaseDir:      baseDir,
	}

	// Pre-create directories for devices 1 and 3 to simulate in-use CNI allocations.
	for _, id := range []string{"1", "3"} {
		dir := filepath.Join(baseDir, fmt.Sprintf("net0_%s", id))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	pool := NewVhostUserResourcePool(cfg, rc)
	pool.setDevInfoStore(devinfo.NewTestStore())

	devices := pool.Devices()
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices (5 minus 2 existing), got %d", len(devices))
	}

	ids := make(map[string]bool)
	for _, dev := range devices {
		ids[dev.ID] = true
	}
	if ids["1"] {
		t.Error("device 1 should have been skipped (directory exists)")
	}
	if ids["3"] {
		t.Error("device 3 should have been skipped (directory exists)")
	}
	if !ids["0"] || !ids["2"] || !ids["4"] {
		t.Errorf("expected devices 0, 2, 4 to be present, got %v", ids)
	}
}

func TestAllocateReturnsMountPath(t *testing.T) {
	pool, _, baseDir := newTestPool(t, 10)

	alloc, err := pool.Allocate("0")
	if err != nil {
		t.Fatalf("Allocate() error: %v", err)
	}

	dir := filepath.Join(baseDir, "net0_0")
	mounts := alloc.Mounts()
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].HostPath != dir {
		t.Errorf("mount HostPath = %q, want %q", mounts[0].HostPath, dir)
	}
	if mounts[0].ContainerPath != dir {
		t.Errorf("mount ContainerPath = %q, want %q", mounts[0].ContainerPath, dir)
	}
	if mounts[0].ReadOnly {
		t.Error("mount should not be read-only")
	}
}

func TestAllocateIsIdempotent(t *testing.T) {
	pool, _, _ := newTestPool(t, 10)

	if _, err := pool.Allocate("0"); err != nil {
		t.Fatalf("first Allocate() error: %v", err)
	}
	if _, err := pool.Allocate("0"); err != nil {
		t.Fatalf("second Allocate() error: %v", err)
	}
}

func TestAllocateDeviceSpecsAndAnnotationsAreNil(t *testing.T) {
	pool, _, _ := newTestPool(t, 10)

	alloc, err := pool.Allocate("0")
	if err != nil {
		t.Fatalf("Allocate() error: %v", err)
	}

	if alloc.DeviceSpecs() != nil {
		t.Error("expected DeviceSpecs() to be nil")
	}
	if alloc.Annotations() != nil {
		t.Error("expected Annotations() to be nil")
	}
}

func TestAllocateWritesDevInfo(t *testing.T) {
	pool, store, baseDir := newTestPool(t, 10)

	if _, err := pool.Allocate("3"); err != nil {
		t.Fatalf("Allocate() error: %v", err)
	}

	devInfo := store.Get("virtio.example.com/net0", "3")
	if devInfo == nil {
		t.Fatal("expected devinfo to be saved")
	}
	if devInfo.Type != devInfoType {
		t.Errorf("devinfo Type = %q, want %q", devInfo.Type, devInfoType)
	}
	if devInfo.VhostUser == nil {
		t.Fatal("expected VhostUser to be set")
	}
	if devInfo.VhostUser.Mode != vhostUserMode {
		t.Errorf("devinfo Mode = %q, want %q", devInfo.VhostUser.Mode, vhostUserMode)
	}

	expectedPath := filepath.Join(baseDir, "net0_3", socketFileName)
	if devInfo.VhostUser.Path != expectedPath {
		t.Errorf("devinfo Path = %q, want %q", devInfo.VhostUser.Path, expectedPath)
	}
}

func TestCleanup(t *testing.T) {
	pool, store, _ := newTestPool(t, 3)

	for _, id := range []string{"0", "1", "2"} {
		if _, err := pool.Allocate(id); err != nil {
			t.Fatalf("Allocate(%s) error: %v", id, err)
		}
	}

	if store.Count() != 3 {
		t.Fatalf("expected 3 devinfo entries, got %d", store.Count())
	}

	if err := pool.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	if store.Count() != 0 {
		t.Errorf("expected 0 devinfo entries after cleanup, got %d", store.Count())
	}
}

func TestCleanupNoDevInfoIsNotError(t *testing.T) {
	pool, _, _ := newTestPool(t, 5)

	if err := pool.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}
}

func TestMultipleDeviceIDs(t *testing.T) {
	pool, _, baseDir := newTestPool(t, 10)

	for _, id := range []string{"0", "4", "9"} {
		alloc, err := pool.Allocate(id)
		if err != nil {
			t.Fatalf("Allocate(%s) error: %v", id, err)
		}

		expectedDir := filepath.Join(baseDir, fmt.Sprintf("net0_%s", id))
		mounts := alloc.Mounts()
		if len(mounts) != 1 || mounts[0].HostPath != expectedDir {
			t.Errorf("device %s: mount HostPath = %q, want %q", id, mounts[0].HostPath, expectedDir)
		}
	}
}
