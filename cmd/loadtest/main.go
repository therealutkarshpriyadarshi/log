package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/buffer"
	"github.com/therealutkarshpriyadarshi/log/internal/logging"
	"github.com/therealutkarshpriyadarshi/log/internal/parser"
	"github.com/therealutkarshpriyadarshi/log/internal/pool"
	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

var (
	targetRate  = flag.Int("rate", 100000, "Target events per second")
	duration    = flag.Int("duration", 60, "Test duration in seconds")
	workers     = flag.Int("workers", 4, "Number of worker goroutines")
	bufferSize  = flag.Int("buffer", 1048576, "Ring buffer size")
	parserType  = flag.String("parser", "json", "Parser type (json, regex, grok)")
	usePooling  = flag.Bool("pool", true, "Use object pooling")
	reportInterval = flag.Int("interval", 5, "Report interval in seconds")
)

// Stats tracks load test statistics
type Stats struct {
	eventsGenerated uint64
	eventsParsed    uint64
	eventsBuffered  uint64
	parseErrors     uint64
	bufferErrors    uint64
	startTime       time.Time
}

func (s *Stats) Report() {
	elapsed := time.Since(s.startTime).Seconds()
	generated := atomic.LoadUint64(&s.eventsGenerated)
	parsed := atomic.LoadUint64(&s.eventsParsed)
	buffered := atomic.LoadUint64(&s.eventsBuffered)
	parseErrors := atomic.LoadUint64(&s.parseErrors)
	bufferErrors := atomic.LoadUint64(&s.bufferErrors)

	fmt.Printf("\n=== Load Test Statistics ===\n")
	fmt.Printf("Duration: %.2f seconds\n", elapsed)
	fmt.Printf("Events Generated: %d (%.0f/sec)\n", generated, float64(generated)/elapsed)
	fmt.Printf("Events Parsed: %d (%.0f/sec)\n", parsed, float64(parsed)/elapsed)
	fmt.Printf("Events Buffered: %d (%.0f/sec)\n", buffered, float64(buffered)/elapsed)
	fmt.Printf("Parse Errors: %d\n", parseErrors)
	fmt.Printf("Buffer Errors: %d\n", bufferErrors)
	fmt.Printf("Success Rate: %.2f%%\n", float64(parsed)/float64(generated)*100)
	fmt.Printf("============================\n\n")
}

func main() {
	flag.Parse()

	logger := logging.New(logging.Config{
		Level:  "info",
		Format: "console",
	})

	fmt.Printf("Starting load test...\n")
	fmt.Printf("Target Rate: %d events/sec\n", *targetRate)
	fmt.Printf("Duration: %d seconds\n", *duration)
	fmt.Printf("Workers: %d\n", *workers)
	fmt.Printf("Buffer Size: %d\n", *bufferSize)
	fmt.Printf("Parser Type: %s\n", *parserType)
	fmt.Printf("Object Pooling: %t\n\n", *usePooling)

	if err := run(logger); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(logger *logging.Logger) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create parser
	var parserCfg *parser.ParserConfig
	switch *parserType {
	case "json":
		parserCfg = &parser.ParserConfig{
			Type:         parser.ParserTypeJSON,
			TimeField:    "timestamp",
			LevelField:   "level",
			MessageField: "message",
		}
	case "regex":
		parserCfg = &parser.ParserConfig{
			Type:       parser.ParserTypeRegex,
			Pattern:    `^(?P<timestamp>\S+)\s+\[(?P<level>\w+)\]\s+(?P<message>.*)$`,
			TimeField:  "timestamp",
			LevelField: "level",
		}
	default:
		return fmt.Errorf("unsupported parser type: %s", *parserType)
	}

	p, err := parser.New(parserCfg)
	if err != nil {
		return fmt.Errorf("failed to create parser: %w", err)
	}

	// Create ring buffer
	bufferCfg := buffer.RingBufferConfig{
		Size:                 *bufferSize,
		BackpressureStrategy: buffer.BackpressureDrop,
	}

	rb, err := buffer.NewRingBuffer(bufferCfg)
	if err != nil {
		return fmt.Errorf("failed to create buffer: %w", err)
	}

	// Initialize stats
	stats := &Stats{
		startTime: time.Now(),
	}

	// Start periodic reporting
	go func() {
		ticker := time.NewTicker(time.Duration(*reportInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats.Report()
			}
		}
	}()

	// Start workers
	var wg sync.WaitGroup
	eventsPerWorker := *targetRate / *workers
	sleepDuration := time.Second / time.Duration(eventsPerWorker)

	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			runWorker(ctx, workerID, p, rb, stats, sleepDuration)
		}(i)
	}

	// Wait for duration or signal
	select {
	case <-time.After(time.Duration(*duration) * time.Second):
		logger.Info().Msg("Test duration reached")
	case <-sigCh:
		logger.Info().Msg("Received shutdown signal")
	}

	// Stop workers
	cancel()
	wg.Wait()

	// Final report
	stats.Report()

	return nil
}

func runWorker(ctx context.Context, workerID int, p parser.Parser, rb *buffer.RingBuffer, stats *Stats, sleepDuration time.Duration) {
	logTemplates := []string{
		`{"timestamp":"%s","level":"info","message":"User login successful","user_id":%d,"ip":"192.168.1.%d"}`,
		`{"timestamp":"%s","level":"warn","message":"High memory usage detected","memory_mb":%d,"threshold_mb":8192}`,
		`{"timestamp":"%s","level":"error","message":"Database query timeout","query":"SELECT * FROM users","duration_ms":%d}`,
		`{"timestamp":"%s","level":"info","message":"API request processed","endpoint":"/api/users","status":%d,"duration_ms":%d}`,
		`{"timestamp":"%s","level":"debug","message":"Cache hit","key":"user:%d","ttl_seconds":%d}`,
	}

	ticker := time.NewTicker(sleepDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Generate log line
			template := logTemplates[rand.Intn(len(logTemplates))]
			var logLine string

			switch rand.Intn(5) {
			case 0:
				logLine = fmt.Sprintf(template, time.Now().Format(time.RFC3339), rand.Intn(10000), rand.Intn(255))
			case 1:
				logLine = fmt.Sprintf(template, time.Now().Format(time.RFC3339), rand.Intn(16384))
			case 2:
				logLine = fmt.Sprintf(template, time.Now().Format(time.RFC3339), rand.Intn(5000))
			case 3:
				logLine = fmt.Sprintf(template, time.Now().Format(time.RFC3339), 200+rand.Intn(300), rand.Intn(1000))
			case 4:
				logLine = fmt.Sprintf(template, time.Now().Format(time.RFC3339), rand.Intn(10000), rand.Intn(3600))
			}

			atomic.AddUint64(&stats.eventsGenerated, 1)

			// Parse
			var event *types.LogEvent
			var err error

			if *usePooling {
				event = pool.GetEvent()
			}

			event, err = p.Parse(logLine, "loadtest")
			if err != nil {
				atomic.AddUint64(&stats.parseErrors, 1)
				if *usePooling && event != nil {
					pool.PutEvent(event)
				}
				continue
			}

			atomic.AddUint64(&stats.eventsParsed, 1)

			// Buffer
			if err := rb.Enqueue(ctx, event); err != nil {
				atomic.AddUint64(&stats.bufferErrors, 1)
				if *usePooling {
					pool.PutEvent(event)
				}
				continue
			}

			atomic.AddUint64(&stats.eventsBuffered, 1)
		}
	}
}
