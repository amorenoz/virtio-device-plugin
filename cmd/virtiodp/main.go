package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/k8snetworkplumbingwg/virtio-device-plugin/pkg/config"
	"github.com/k8snetworkplumbingwg/virtio-device-plugin/pkg/plugin"
)

func main() {
	configFile := flag.String("config-file", config.DefaultConfigFile, "Path to JSON config file")
	logLevel := flag.String("log-level", "info", "Log level: debug, info, warn, error. Default: info")
	logFormat := flag.String("log-format", "text", "Log format: text, json. Default: text")
	flag.Parse()

	setupLogger(*logLevel, *logFormat)

	cfg, err := config.Load(*configFile)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	servers := startServers(cfg)
	if len(servers) == 0 {
		slog.Error("no servers started")
		os.Exit(1)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	s := <-sig
	slog.Info("received signal, shutting down", "signal", s)

	for _, srv := range servers {
		srv.Stop()
	}
}

func startServers(cfg *config.PluginConfig) []*plugin.Server {
	var servers []*plugin.Server

	for _, rc := range cfg.ResourceList {
		// FIXME: Replace with actual resource pool.
		pool := plugin.StubResourcePool{}
		srv := plugin.NewServer(&pool)

		if err := srv.Start(); err != nil {
			slog.Error("failed to start server", "resource", rc.ResourceName, "error", err)
			continue
		}

		servers = append(servers, srv)
	}

	return servers
}

func setupLogger(level, format string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(handler))
}
