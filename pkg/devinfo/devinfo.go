// Package devinfo abstracts Device Plugin devinfo file operations
// per https://github.com/k8snetworkplumbingwg/device-info-spec.
package devinfo

import (
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nadutils "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/utils"
)

// Store abstracts DP devinfo file operations.
type Store interface {
	Save(resourceName, deviceID string, devInfo *nadv1.DeviceInfo) error
	Clean(resourceName, deviceID string) error
}

// DPStore uses the NAD client's SaveDeviceInfoForDP / CleanDeviceInfoForDP.
// These operate on /var/run/k8s.cni.cncf.io/devinfo/dp/ (DP files, not CNI files).
type DPStore struct{}

// Save writes a DP devinfo file.
func (DPStore) Save(resourceName, deviceID string, devInfo *nadv1.DeviceInfo) error {
	return nadutils.SaveDeviceInfoForDP(resourceName, deviceID, devInfo)
}

// Clean removes a DP devinfo file.
func (DPStore) Clean(resourceName, deviceID string) error {
	return nadutils.CleanDeviceInfoForDP(resourceName, deviceID)
}
