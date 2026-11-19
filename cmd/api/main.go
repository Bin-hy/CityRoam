package main

import (
	"fmt"
	"os"

	"trippilot/internal/config"
	"trippilot/internal/logging"
)

const version = "0.1.0-dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel)
	logger.Info("trippilot api starting", "version", version, "port", cfg.Port)
}
