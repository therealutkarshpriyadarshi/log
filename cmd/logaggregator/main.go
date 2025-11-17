package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/therealutkarshpriyadarshi/log/internal/checkpoint"
	"github.com/therealutkarshpriyadarshi/log/internal/config"
	"github.com/therealutkarshpriyadarshi/log/internal/input"
	"github.com/therealutkarshpriyadarshi/log/internal/logging"
	"github.com/therealutkarshpriyadarshi/log/internal/parser"
	"github.com/therealutkarshpriyadarshi/log/internal/tailer"
)

var (
	configFile = flag.String("config", "config.yaml", "Path to configuration file")
	version    = "0.2.0"
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

	var wg sync.WaitGroup
	var inputs []input.Input

	// Process file inputs
	for _, fileInput := range cfg.Inputs.Files {
		fileInputCopy := fileInput
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := processFileInput(fileInputCopy, logger); err != nil {
				logger.Error().Err(err).Msg("Failed to process file input")
			}
		}()
	}

	// Process syslog inputs
	for _, syslogInput := range cfg.Inputs.Syslog {
		syslogConfig := &input.SyslogConfig{
			Protocol:   syslogInput.Protocol,
			Address:    syslogInput.Address,
			Format:     syslogInput.Format,
			TLSEnabled: syslogInput.TLSEnabled,
			TLSCert:    syslogInput.TLSCert,
			TLSKey:     syslogInput.TLSKey,
			RateLimit:  syslogInput.RateLimit,
			BufferSize: syslogInput.BufferSize,
		}

		inp, err := input.NewSyslogInput(syslogInput.Name, syslogConfig, logger)
		if err != nil {
			return fmt.Errorf("failed to create syslog input '%s': %w", syslogInput.Name, err)
		}

		if err := inp.Start(); err != nil {
			return fmt.Errorf("failed to start syslog input '%s': %w", syslogInput.Name, err)
		}

		inputs = append(inputs, inp)

		// Process events from this input
		wg.Add(1)
		go func(i input.Input, parserCfg *config.ParserConfig, transforms []config.TransformConfig) {
			defer wg.Done()
			processInputEvents(i, parserCfg, transforms, logger)
		}(inp, syslogInput.Parser, syslogInput.Transforms)

		logger.Info().Str("name", syslogInput.Name).Str("type", "syslog").Msg("Input started")
	}

	// Process HTTP inputs
	for _, httpInput := range cfg.Inputs.HTTP {
		httpConfig := &input.HTTPConfig{
			Address:      httpInput.Address,
			Path:         httpInput.Path,
			BatchPath:    httpInput.BatchPath,
			APIKeys:      httpInput.APIKeys,
			RateLimit:    httpInput.RateLimit,
			MaxBodySize:  httpInput.MaxBodySize,
			TLSEnabled:   httpInput.TLSEnabled,
			TLSCert:      httpInput.TLSCert,
			TLSKey:       httpInput.TLSKey,
			BufferSize:   httpInput.BufferSize,
			ReadTimeout:  httpInput.ReadTimeout,
			WriteTimeout: httpInput.WriteTimeout,
		}

		inp, err := input.NewHTTPInput(httpInput.Name, httpConfig, logger)
		if err != nil {
			return fmt.Errorf("failed to create HTTP input '%s': %w", httpInput.Name, err)
		}

		if err := inp.Start(); err != nil {
			return fmt.Errorf("failed to start HTTP input '%s': %w", httpInput.Name, err)
		}

		inputs = append(inputs, inp)

		// Process events from this input
		wg.Add(1)
		go func(i input.Input, parserCfg *config.ParserConfig, transforms []config.TransformConfig) {
			defer wg.Done()
			processInputEvents(i, parserCfg, transforms, logger)
		}(inp, httpInput.Parser, httpInput.Transforms)

		logger.Info().Str("name", httpInput.Name).Str("type", "http").Msg("Input started")
	}

	// Process Kubernetes inputs
	for _, k8sInput := range cfg.Inputs.Kubernetes {
		k8sConfig := &input.KubernetesConfig{
			Kubeconfig:       k8sInput.Kubeconfig,
			Namespace:        k8sInput.Namespace,
			LabelSelector:    k8sInput.LabelSelector,
			FieldSelector:    k8sInput.FieldSelector,
			ContainerPattern: k8sInput.ContainerPattern,
			Follow:           k8sInput.Follow,
			IncludePrevious:  k8sInput.IncludePrevious,
			TailLines:        k8sInput.TailLines,
			EnrichMetadata:   k8sInput.EnrichMetadata,
			BufferSize:       k8sInput.BufferSize,
		}

		inp, err := input.NewKubernetesInput(k8sInput.Name, k8sConfig, logger)
		if err != nil {
			return fmt.Errorf("failed to create Kubernetes input '%s': %w", k8sInput.Name, err)
		}

		if err := inp.Start(); err != nil {
			return fmt.Errorf("failed to start Kubernetes input '%s': %w", k8sInput.Name, err)
		}

		inputs = append(inputs, inp)

		// Process events from this input
		wg.Add(1)
		go func(i input.Input, parserCfg *config.ParserConfig, transforms []config.TransformConfig) {
			defer wg.Done()
			processInputEvents(i, parserCfg, transforms, logger)
		}(inp, k8sInput.Parser, k8sInput.Transforms)

		logger.Info().Str("name", k8sInput.Name).Str("type", "kubernetes").Msg("Input started")
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info().Msg("Shutdown signal received")

	// Stop all inputs
	for _, inp := range inputs {
		if err := inp.Stop(); err != nil {
			logger.Error().Err(err).Str("name", inp.Name()).Msg("Failed to stop input")
		}
	}

	// Wait for all goroutines to finish
	wg.Wait()

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

func processInputEvents(inp input.Input, parserCfg *config.ParserConfig, transforms []config.TransformConfig, logger *logging.Logger) {
	// Create parser if configured
	var logParser parser.Parser
	var err error

	if parserCfg != nil {
		pCfg := &parser.ParserConfig{
			Type:         parser.ParserType(parserCfg.Type),
			Pattern:      parserCfg.Pattern,
			GrokPattern:  parserCfg.GrokPattern,
			TimeFormat:   parserCfg.TimeFormat,
			TimeField:    parserCfg.TimeField,
			LevelField:   parserCfg.LevelField,
			MessageField: parserCfg.MessageField,
			CustomFields: parserCfg.CustomFields,
		}

		if parserCfg.Multiline != nil {
			pCfg.Multiline = &parser.MultilineConfig{
				Pattern:  parserCfg.Multiline.Pattern,
				Negate:   parserCfg.Multiline.Negate,
				Match:    parserCfg.Multiline.Match,
				MaxLines: parserCfg.Multiline.MaxLines,
				Timeout:  parserCfg.Multiline.Timeout,
			}
		}

		logParser, err = parser.New(pCfg)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to create parser")
		} else {
			logger.Info().Str("parser", logParser.Name()).Msg("Parser initialized for input")
		}
	}

	// Create transform pipeline if configured
	var transformPipeline *parser.TransformPipeline
	if len(transforms) > 0 {
		transformConfigs := make([]parser.TransformConfig, len(transforms))
		for i, tc := range transforms {
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
			logger.Error().Err(err).Msg("Failed to create transform pipeline")
		} else {
			logger.Info().Int("transforms", len(transformConfigs)).Msg("Transform pipeline initialized for input")
		}
	}

	// Process events
	for event := range inp.Events() {
		// If parser is configured, parse the log line
		if logParser != nil {
			parsedEvent, err := logParser.Parse(event.Message, event.Source)
			if err != nil {
				logger.Warn().Err(err).Str("line", event.Message).Msg("Failed to parse log line")
				// Output as-is with existing fields
				output, _ := json.Marshal(event)
				fmt.Println(string(output))
				continue
			}

			// Merge existing fields from event (e.g., Kubernetes metadata)
			if event.Fields != nil {
				if parsedEvent.Fields == nil {
					parsedEvent.Fields = make(map[string]interface{})
				}
				for k, v := range event.Fields {
					parsedEvent.Fields[k] = v
				}
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
			// No parser configured, output with fields
			output, err := json.Marshal(event)
			if err != nil {
				fmt.Println(event.Message)
			} else {
				fmt.Println(string(output))
			}
		}
	}
}
