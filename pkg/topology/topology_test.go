package topology

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFakeSysfs(t *testing.T, pciAddr, content string) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, pciAddr)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "numa_node"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return base
}

func TestNUMANodeForPCI(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantNode int
		wantErr  bool
	}{
		{
			name:     "node 0",
			content:  "0\n",
			wantNode: 0,
		},
		{
			name:     "node 1",
			content:  "1\n",
			wantNode: 1,
		},
		{
			name:     "no affinity returns -1",
			content:  "-1\n",
			wantNode: -1,
		},
		{
			name:     "no trailing newline",
			content:  "2",
			wantNode: 2,
		},
		{
			name:     "garbage content",
			content:  "not-a-number\n",
			wantNode: -1,
			wantErr:  true,
		},
	}

	pciAddr := "0000:ab:cd.0"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := writeFakeSysfs(t, pciAddr, tt.content)
			got, err := NUMANodeForPCI(base, pciAddr)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantNode {
				t.Errorf("NUMANodeForPCI() = %d, want %d", got, tt.wantNode)
			}
		})
	}
}

func TestNUMANodeForPCI_MissingFile(t *testing.T) {
	base := t.TempDir()
	got, err := NUMANodeForPCI(base, "0000:ff:ff.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != -1 {
		t.Errorf("expected -1 for missing file, got %d", got)
	}
}

func TestNUMANodeForPCI_DefaultSysfsBase(t *testing.T) {
	// With empty sysfsBase and a non-existent PCI address, should return -1
	// (file not found is not an error, just no affinity).
	got, err := NUMANodeForPCI("", "9999:99:99.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != -1 {
		t.Errorf("expected -1, got %d", got)
	}
}
