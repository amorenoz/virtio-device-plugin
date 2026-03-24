// Package topology resolves NUMA node affinity from PCI device addresses.
package topology

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultSysfsBase = "/sys/bus/pci/devices"

// NUMANodeForPCI reads the numa_node file for the given PCI address from sysfs
// and returns the NUMA node ID. Returns -1 if the file doesn't exist or the
// kernel reports -1 (no NUMA affinity). sysfsBase can be overridden for testing;
// pass "" to use the default /sys/bus/pci/devices.
func NUMANodeForPCI(sysfsBase, pciAddr string) (int, error) {
	if sysfsBase == "" {
		sysfsBase = defaultSysfsBase
	}

	path := filepath.Join(sysfsBase, pciAddr, "numa_node")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return -1, nil
		}
		return -1, fmt.Errorf("reading %s: %w", path, err)
	}

	val, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return -1, fmt.Errorf("parsing numa_node from %s: %w", path, err)
	}

	if val < 0 {
		return -1, nil
	}

	return val, nil
}
