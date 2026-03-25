// Package plugin implements the Kubernetes Device Plugin gRPC server.
// Registration uses kubelet's pluginwatcher mechanism: the plugin places its
// socket under /var/lib/kubelet/plugins_registry/ and implements the
// pluginregistration.RegistrationServer interface. Kubelet discovers the socket
// automatically and calls GetInfo to register the plugin.
package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	registerapi "k8s.io/kubelet/pkg/apis/pluginregistration/v1"
)

const (
	// PluginRegistryDir is where kubelet's pluginwatcher looks for plugin sockets.
	PluginRegistryDir = "/var/lib/kubelet/plugins_registry"
)

// Server is a gRPC device plugin server for a single resource.
// It serves both the pluginregistration and deviceplugin APIs on a single socket.
type Server struct {
	pluginapi.UnimplementedDevicePluginServer
	registerapi.UnimplementedRegistrationServer

	pool       ResourcePool
	grpcServer *grpc.Server
	socketPath string
	stop       chan struct{}
}

// NewServer creates a new device plugin server.
func NewServer(pool ResourcePool) *Server {
	socketName := sanitizeForPath(pool.ResourceName()) + ".sock"
	return &Server{
		pool:       pool,
		socketPath: filepath.Join(PluginRegistryDir, socketName),
		stop:       make(chan struct{}),
	}
}

// Start creates the unix socket and starts the gRPC server.
// Kubelet's pluginwatcher discovers the socket and calls GetInfo to register.
func (s *Server) Start() error {
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing stale socket %s: %w", s.socketPath, err)
	}

	lis, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.socketPath, err)
	}

	s.grpcServer = grpc.NewServer()
	registerapi.RegisterRegistrationServer(s.grpcServer, s)
	pluginapi.RegisterDevicePluginServer(s.grpcServer, s)

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC server failed", "resource", s.pool.ResourceName(), "error", err)
		}
	}()

	slog.Info("device plugin server started", "resource", s.pool.ResourceName(), "socket", s.socketPath)
	return nil
}

// Stop gracefully stops the gRPC server and cleans up.
func (s *Server) Stop() {
	close(s.stop)

	if s.grpcServer != nil {
		s.grpcServer.Stop()
	}

	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove socket", "path", s.socketPath, "error", err)
	}

	if err := s.pool.Cleanup(); err != nil {
		slog.Warn("pool cleanup failed", "resource", s.pool.ResourceName(), "error", err)
	}

	slog.Info("device plugin stopped", "resource", s.pool.ResourceName())
}

// --- pluginregistration.RegistrationServer implementation ---

// GetInfo is called by kubelet's pluginwatcher to discover the plugin.
func (s *Server) GetInfo(_ context.Context, _ *registerapi.InfoRequest) (*registerapi.PluginInfo, error) {
	slog.Info("GetInfo called by kubelet", "resource", s.pool.ResourceName())
	return &registerapi.PluginInfo{
		Type:              registerapi.DevicePlugin,
		Name:              s.pool.ResourceName(),
		Endpoint:          s.socketPath,
		SupportedVersions: []string{pluginapi.Version},
	}, nil
}

// NotifyRegistrationStatus is called by kubelet after registration succeeds or fails.
func (s *Server) NotifyRegistrationStatus(_ context.Context, status *registerapi.RegistrationStatus) (*registerapi.RegistrationStatusResponse, error) {
	if status.PluginRegistered {
		slog.Info("registered with kubelet", "resource", s.pool.ResourceName())
	} else {
		slog.Error("registration failed", "resource", s.pool.ResourceName(), "error", status.Error)
	}
	return &registerapi.RegistrationStatusResponse{}, nil
}

// --- deviceplugin.DevicePluginServer implementation ---

func (s *Server) GetDevicePluginOptions(_ context.Context, _ *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{
		PreStartRequired:                false,
		GetPreferredAllocationAvailable: false,
	}, nil
}

func (s *Server) ListAndWatch(_ *pluginapi.Empty, stream pluginapi.DevicePlugin_ListAndWatchServer) error {
	slog.Info("ListAndWatch called", "resource", s.pool.ResourceName())

	if err := stream.Send(&pluginapi.ListAndWatchResponse{Devices: s.pool.Devices()}); err != nil {
		return fmt.Errorf("sending device list: %w", err)
	}

	// Block until stopped. Device list is static.
	<-s.stop
	return nil
}

func (s *Server) Allocate(_ context.Context, req *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	resp := &pluginapi.AllocateResponse{}
	logger := slog.With(
		"resource", s.pool.ResourceName(),
		"request", req.ContainerRequests,
	)

	logger.Info("Allocate started")

	for _, containerReq := range req.ContainerRequests {
		containerResp := &pluginapi.ContainerAllocateResponse{}

		for _, deviceID := range containerReq.DevicesIds {
			alloc, err := s.pool.Allocate(deviceID)
			if err != nil {
				return nil, fmt.Errorf("allocating device %s: %w", deviceID, err)
			}

			if mounts := alloc.Mounts(); mounts != nil {
				containerResp.Mounts = append(containerResp.Mounts, mounts...)
			}
			if specs := alloc.DeviceSpecs(); specs != nil {
				containerResp.Devices = append(containerResp.Devices, specs...)
			}
			if annotations := alloc.Annotations(); annotations != nil {
				if containerResp.Annotations == nil {
					containerResp.Annotations = make(map[string]string)
				}
				maps.Copy(containerResp.Annotations, annotations)
			}
		}

		resp.ContainerResponses = append(resp.ContainerResponses, containerResp)
	}

	logger.Info("Allocate completed", "response", resp)
	return resp, nil
}

func (s *Server) PreStartContainer(_ context.Context, _ *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (s *Server) GetPreferredAllocation(_ context.Context, _ *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return &pluginapi.PreferredAllocationResponse{}, nil
}

// --- internal helpers ---

var pathSanitizer = strings.NewReplacer("/", "-", ".", "-")

// sanitizeForPath modifies a string ensuring it's safe to be used as a path
func sanitizeForPath(s string) string {
	return pathSanitizer.Replace(s)
}
