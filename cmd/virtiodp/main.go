package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/k8snetworkplumbingwg/virtio-device-plugin/pkg/config"
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

	for _, rc := range cfg.ResourceList {
		slog.Info("resource configured",
			"resource", config.FullResourceName(cfg, &rc),
			"numDevices", rc.NumDevices,
			"baseDir", rc.BaseDir,
		)
	}

	// TODO: start plugin servers, watch kubelet socket, handle signals
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
