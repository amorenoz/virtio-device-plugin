package devinfo

import (
	"sync"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

// TestStore is a fake implementation of devinfo.Store used for testing.
// It stores the DeviceInfo objects in an internal map and allows fetching them.
type TestStore struct {
	mu    sync.Mutex
	saved map[string]*nadv1.DeviceInfo // key: "resourceName/deviceID"
}

// NewTestStore creates a TestStore.
func NewTestStore() *TestStore {
	return &TestStore{saved: make(map[string]*nadv1.DeviceInfo)}
}

// Save stores a deviceInfo entry.
func (s *TestStore) Save(resourceName, deviceID string, devInfo *nadv1.DeviceInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved[resourceName+"/"+deviceID] = devInfo
	return nil
}

// Clean removes a deviceInfo entry.
func (s *TestStore) Clean(resourceName, deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.saved, resourceName+"/"+deviceID)
	return nil
}

// Get returns the saved deviceInfo for a resource/deviceID, or nil if not found.
func (s *TestStore) Get(resourceName, deviceID string) *nadv1.DeviceInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saved[resourceName+"/"+deviceID]
}

// Count returns the number of saved deviceInfo entries.
func (s *TestStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.saved)
}
