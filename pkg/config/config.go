package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	DefaultResourceNamePrefix = "virtio"
	DefaultConfigFile         = "/etc/virtiodp/config.json"
	MaxDevicesPerResource     = 10000
)

// PluginConfig is the top-level configuration read from the ConfigMap.
type PluginConfig struct {
	ResourceNamePrefix string           `json:"resourceNamePrefix,omitempty"`
	ResourcePrefix     string           `json:"resourcePrefix"`
	SocketUser         string           `json:"socketUser,omitempty"`
	ResourceList       []ResourceConfig `json:"resourceList"`
}

// ResourceConfig describes a single vhost-user resource pool.
type ResourceConfig struct {
	ResourceName      string         `json:"resourceName"`
	NumDevices        int            `json:"numDevices"`
	BaseDir           string         `json:"baseDir"`
	TopologyHintsFrom []TopologyHint `json:"topologyHintsFrom,omitempty"`
}

// TopologyHint references a PCI device whose NUMA topology is inherited by
// all devices in the resource pool.
type TopologyHint struct {
	PCIAddress string `json:"pciAddress"`
}

// BDF PCI address: dddd:BB:DD.f
var pciAddrRegexp = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]$`)

// Unix username: alphanumeric, hyphen, underscore, 1-32 chars, must start with letter or underscore.
var unixUserRegexp = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// Load reads the config file, unmarshals it, applies defaults, and validates.
func Load(path string) (*PluginConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	cfg := &PluginConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func applyDefaults(cfg *PluginConfig) {
	if cfg.ResourceNamePrefix == "" {
		cfg.ResourceNamePrefix = DefaultResourceNamePrefix
	}
}

func validate(cfg *PluginConfig) error {
	if cfg.ResourcePrefix == "" {
		return fmt.Errorf("resourcePrefix is required")
	}
	if errs := validation.IsDNS1123Subdomain(cfg.ResourcePrefix); len(errs) > 0 {
		return fmt.Errorf("resourcePrefix %q is not a valid DNS subdomain: %s", cfg.ResourcePrefix, strings.Join(errs, "; "))
	}

	if cfg.SocketUser != "" && !unixUserRegexp.MatchString(cfg.SocketUser) {
		return fmt.Errorf("socketUser %q is not a valid Unix username", cfg.SocketUser)
	}

	if len(cfg.ResourceList) == 0 {
		return fmt.Errorf("resourceList must contain at least one entry")
	}

	seen := make(map[string]bool)
	for i, rc := range cfg.ResourceList {
		if err := validateResource(rc); err != nil {
			return fmt.Errorf("resourceList[%d]: %w", i, err)
		}
		fullName := FullResourceName(cfg, &rc)
		if seen[fullName] {
			return fmt.Errorf("resourceList[%d]: duplicate resource name %q", i, fullName)
		}
		seen[fullName] = true
	}

	return nil
}

func validateResource(rc ResourceConfig) error {
	if rc.ResourceName == "" {
		return fmt.Errorf("resourceName is required")
	}
	if errs := validation.IsValidLabelValue(rc.ResourceName); len(errs) > 0 {
		return fmt.Errorf("resourceName %q is not a valid k8s label value: %s", rc.ResourceName, strings.Join(errs, "; "))
	}

	if rc.NumDevices <= 0 {
		return fmt.Errorf("numDevices must be > 0, got %d", rc.NumDevices)
	}
	if rc.NumDevices > MaxDevicesPerResource {
		return fmt.Errorf("numDevices must be <= %d, got %d", MaxDevicesPerResource, rc.NumDevices)
	}

	if rc.BaseDir == "" {
		return fmt.Errorf("baseDir is required")
	}
	if !filepath.IsAbs(rc.BaseDir) {
		return fmt.Errorf("baseDir %q must be an absolute path", rc.BaseDir)
	}

	for j, th := range rc.TopologyHintsFrom {
		if !pciAddrRegexp.MatchString(th.PCIAddress) {
			return fmt.Errorf("topologyHintsFrom[%d]: invalid PCI address %q (expected dddd:BB:DD.f)", j, th.PCIAddress)
		}
	}

	return nil
}

// FullResourceName returns the fully qualified k8s extended resource name,
// e.g. "virtio.openshift.io/vhost-phy0".
func FullResourceName(cfg *PluginConfig, rc *ResourceConfig) string {
	return cfg.ResourceNamePrefix + "." + cfg.ResourcePrefix + "/" + rc.ResourceName
}
