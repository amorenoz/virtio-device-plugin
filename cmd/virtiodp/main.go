package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/k8snetworkplumbingwg/virtio-device-plugin/pkg/config"
)

func main() {
	configFile := flag.String("config-file", config.DefaultConfigFile, "path to JSON config file")
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("loaded config with %d resource(s)\n", len(cfg.ResourceList))
	for _, rc := range cfg.ResourceList {
		fmt.Printf("  %s (%d devices)\n", config.FullResourceName(cfg, &rc), rc.NumDevices)
		fmt.Printf("  %v\n", cfg)
	}

	// TODO: start plugin servers, watch kubelet socket, handle signals
}
