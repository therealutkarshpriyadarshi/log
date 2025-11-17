package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/therealutkarshpriyadarshi/log/internal/checkpoint"
	"github.com/therealutkarshpriyadarshi/log/internal/config"
	"github.com/therealutkarshpriyadarshi/log/internal/logging"
	"github.com/therealutkarshpriyadarshi/log/internal/parser"
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

	// Create parser if configured
	var logParser parser.Parser
	if fileInput.Parser != nil {
		parserCfg := &parser.ParserConfig{
			Type:         parser.ParserType(fileInput.Parser.Type),
			Pattern:      fileInput.Parser.Pattern,
			GrokPattern:  fileInput.Parser.GrokPattern,
			TimeFormat:   fileInput.Parser.TimeFormat,
			TimeField:    fileInput.Parser.TimeField,
			LevelField:   fileInput.Parser.LevelField,
			MessageField: fileInput.Parser.MessageField,
			CustomFields: fileInput.Parser.CustomFields,
		}

		if fileInput.Parser.Multiline != nil {
			parserCfg.Multiline = &parser.MultilineConfig{
				Pattern:  fileInput.Parser.Multiline.Pattern,
				Negate:   fileInput.Parser.Multiline.Negate,
				Match:    fileInput.Parser.Multiline.Match,
				MaxLines: fileInput.Parser.Multiline.MaxLines,
				Timeout:  fileInput.Parser.Multiline.Timeout,
			}
		}

		logParser, err = parser.New(parserCfg)
		if err != nil {
			return fmt.Errorf("failed to create parser: %w", err)
		}
		logger.Info().Str("parser", logParser.Name()).Msg("Parser initialized")
	}

	// Create transform pipeline if configured
	var transformPipeline *parser.TransformPipeline
	if len(fileInput.Transforms) > 0 {
		transformConfigs := make([]parser.TransformConfig, len(fileInput.Transforms))
		for i, tc := range fileInput.Transforms {
			transformConfigs[i] = parser.TransformConfig{
				Type:          tc.Type,
				Fields:        tc.Fields,
				IncludeFields: tc.IncludeFields,
				ExcludeFields: tc.ExcludeFields,
				Rename:        tc.Rename,
				Add:           tc.Add,
				Patterns:      tc.Patterns,
				FieldSplit:    tc.FieldSplit,
				ValueSplit:    tc.ValueSplit,
				Prefix:        tc.Prefix,
			}
		}

		transformPipeline, err = parser.NewTransformPipeline(transformConfigs)
		if err != nil {
			return fmt.Errorf("failed to create transform pipeline: %w", err)
		}
		logger.Info().Int("transforms", len(transformConfigs)).Msg("Transform pipeline initialized")
	}

	// Start tailing
	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to start tailer: %w", err)
	}

	// Process events
	go func() {
		for event := range t.Events() {
			// If parser is configured, parse the log line
			if logParser != nil {
				parsedEvent, err := logParser.Parse(event.Message, event.Source)
				if err != nil {
					logger.Warn().Err(err).Str("line", event.Message).Msg("Failed to parse log line")
					// Output raw line if parsing fails
					fmt.Println(event.Message)
					continue
				}

				// Store raw line
				parsedEvent.Raw = event.Message

				// Apply transformations if configured
				if transformPipeline != nil {
					parsedEvent, err = transformPipeline.Transform(parsedEvent)
					if err != nil {
						logger.Warn().Err(err).Msg("Failed to transform event")
					}
				}

				// Output parsed event as JSON
				output, err := json.Marshal(parsedEvent)
				if err != nil {
					logger.Warn().Err(err).Msg("Failed to marshal event")
					fmt.Println(event.Message)
				} else {
					fmt.Println(string(output))
				}
			} else {
				// No parser configured, output raw line
				fmt.Print(event.Message)
			}
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
