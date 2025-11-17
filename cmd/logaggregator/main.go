package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/therealutkarshpriyadarshi/log/internal/checkpoint"
	"github.com/therealutkarshpriyadarshi/log/internal/config"
	"github.com/therealutkarshpriyadarshi/log/internal/logging"
	"github.com/therealutkarshpriyadarshi/log/internal/tailer"
)

var (
	configFile = flag.String("config", "config.yaml", "Path to configuration file")
	version    = "0.1.0"
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load(*configFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize logger
	logger := logging.New(logging.Config{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	})
	logging.SetGlobal(logger)

	logger.Info().Str("version", version).Msg("Starting log aggregator")

	// Process each file input
	for _, fileInput := range cfg.Inputs.Files {
		if err := processFileInput(fileInput, logger); err != nil {
			return err
		}
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info().Msg("Shutdown signal received")
	return nil
}

func processFileInput(fileInput config.FileInputConfig, logger *logging.Logger) error {
	// Create checkpoint manager
	ckptMgr, err := checkpoint.NewManager(
		fileInput.CheckpointPath,
		fileInput.CheckpointInterval,
	)
	if err != nil {
		return fmt.Errorf("failed to create checkpoint manager: %w", err)
	}

	// Load existing checkpoints
	if err := ckptMgr.Load(); err != nil {
		logger.Warn().Err(err).Msg("Failed to load checkpoints, starting fresh")
	}

	// Start checkpoint manager
	ckptMgr.Start()

	// Create tailer
	t, err := tailer.New(fileInput.Paths, ckptMgr, logger)
	if err != nil {
		return fmt.Errorf("failed to create tailer: %w", err)
	}

	// Start tailing
	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to start tailer: %w", err)
	}

	// Process events
	go func() {
		for event := range t.Events() {
			// For now, just print to stdout
			fmt.Print(event.Message)
		}
	}()

	// Set up cleanup on shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logger.Info().Msg("Stopping tailer")
		t.Stop()
		ckptMgr.Stop()
	}()

	return nil
}
