package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidConfig(t *testing.T) {
	path := writeConfigFile(t, `{
		"resourcePrefix": "openshift.io",
		"resourceList": [
			{
				"resourceName": "vhost-phy0",
				"numDevices": 100,
				"baseDir": "/var/run/ovsdpdk/vhost-user/"
			}
		]
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ResourceNamePrefix != DefaultResourceNamePrefix {
		t.Errorf("expected default resourceNamePrefix %q, got %q", DefaultResourceNamePrefix, cfg.ResourceNamePrefix)
	}
	if cfg.ResourcePrefix != "openshift.io" {
		t.Errorf("expected resourcePrefix %q, got %q", "openshift.io", cfg.ResourcePrefix)
	}
	if len(cfg.ResourceList) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(cfg.ResourceList))
	}
	rc := cfg.ResourceList[0]
	if rc.ResourceName != "vhost-phy0" {
		t.Errorf("expected resourceName %q, got %q", "vhost-phy0", rc.ResourceName)
	}
	if rc.NumDevices != 100 {
		t.Errorf("expected numDevices 100, got %d", rc.NumDevices)
	}
}

func TestLoadCustomPrefix(t *testing.T) {
	path := writeConfigFile(t, `{
		"resourceNamePrefix": "dpdk",
		"resourcePrefix": "example.com",
		"resourceList": [
			{
				"resourceName": "net0",
				"numDevices": 10,
				"baseDir": "/var/run/vhost/"
			}
		]
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ResourceNamePrefix != "dpdk" {
		t.Errorf("expected resourceNamePrefix %q, got %q", "dpdk", cfg.ResourceNamePrefix)
	}
}

func TestFullResourceName(t *testing.T) {
	cfg := &PluginConfig{
		ResourceNamePrefix: "virtio",
		ResourcePrefix:     "openshift.io",
	}
	rc := &ResourceConfig{ResourceName: "vhost-phy0"}

	got := FullResourceName(cfg, rc)
	want := "virtio.openshift.io/vhost-phy0"
	if got != want {
		t.Errorf("FullResourceName() = %q, want %q", got, want)
	}
}

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name:    "missing resourcePrefix",
			config:  `{"resourceList": [{"resourceName": "x1", "numDevices": 1, "baseDir": "/tmp"}]}`,
			wantErr: "resourcePrefix is required",
		},
		{
			name:    "invalid resourcePrefix",
			config:  `{"resourcePrefix": "-bad", "resourceList": [{"resourceName": "x1", "numDevices": 1, "baseDir": "/tmp"}]}`,
			wantErr: "not a valid DNS subdomain",
		},
		{
			name:    "empty resourceList",
			config:  `{"resourcePrefix": "example.com", "resourceList": []}`,
			wantErr: "resourceList must contain at least one entry",
		},
		{
			name:    "missing resourceName",
			config:  `{"resourcePrefix": "example.com", "resourceList": [{"numDevices": 1, "baseDir": "/tmp"}]}`,
			wantErr: "resourceName is required",
		},
		{
			name:    "invalid resourceName",
			config:  `{"resourcePrefix": "example.com", "resourceList": [{"resourceName": "-bad", "numDevices": 1, "baseDir": "/tmp"}]}`,
			wantErr: "not a valid k8s label value",
		},
		{
			name:    "zero numDevices",
			config:  `{"resourcePrefix": "example.com", "resourceList": [{"resourceName": "x1", "numDevices": 0, "baseDir": "/tmp"}]}`,
			wantErr: "numDevices must be > 0",
		},
		{
			name:    "negative numDevices",
			config:  `{"resourcePrefix": "example.com", "resourceList": [{"resourceName": "x1", "numDevices": -5, "baseDir": "/tmp"}]}`,
			wantErr: "numDevices must be > 0",
		},
		{
			name:    "numDevices exceeds max",
			config:  `{"resourcePrefix": "example.com", "resourceList": [{"resourceName": "x1", "numDevices": 99999, "baseDir": "/tmp"}]}`,
			wantErr: "numDevices must be <=",
		},
		{
			name:    "missing baseDir",
			config:  `{"resourcePrefix": "example.com", "resourceList": [{"resourceName": "x1", "numDevices": 1}]}`,
			wantErr: "baseDir is required",
		},
		{
			name:    "relative baseDir",
			config:  `{"resourcePrefix": "example.com", "resourceList": [{"resourceName": "x1", "numDevices": 1, "baseDir": "relative/path"}]}`,
			wantErr: "must be an absolute path",
		},
		{
			name:    "invalid PCI address",
			config:  `{"resourcePrefix": "example.com", "resourceList": [{"resourceName": "x1", "numDevices": 1, "baseDir": "/tmp", "topologyHintsFrom": [{"pciAddress": "bad"}]}]}`,
			wantErr: "invalid PCI address",
		},
		{
			name: "duplicate resource name",
			config: `{"resourcePrefix": "example.com", "resourceList": [
				{"resourceName": "x1", "numDevices": 1, "baseDir": "/tmp"},
				{"resourceName": "x1", "numDevices": 2, "baseDir": "/tmp2"}
			]}`,
			wantErr: "duplicate resource name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfigFile(t, tt.config)
			_, err := Load(path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestLoadWithTopologyHints(t *testing.T) {
	path := writeConfigFile(t, `{
		"resourcePrefix": "openshift.io",
		"resourceList": [
			{
				"resourceName": "vhost-phy0",
				"numDevices": 50,
				"baseDir": "/var/run/vhost/",
				"topologyHintsFrom": [
					{"pciAddress": "0000:ab:cd.0"},
					{"pciAddress": "0000:12:34.7"}
				]
			}
		]
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rc := cfg.ResourceList[0]
	if len(rc.TopologyHintsFrom) != 2 {
		t.Fatalf("expected 2 topology hints, got %d", len(rc.TopologyHintsFrom))
	}
	if rc.TopologyHintsFrom[0].PCIAddress != "0000:ab:cd.0" {
		t.Errorf("unexpected PCI address: %s", rc.TopologyHintsFrom[0].PCIAddress)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	path := writeConfigFile(t, `{not json}`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
