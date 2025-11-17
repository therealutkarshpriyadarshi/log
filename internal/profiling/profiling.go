package profiling

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	runtimepprof "runtime/pprof"
	"sync"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/logging"
)

// Config holds profiling configuration
type Config struct {
	Enabled        bool   `yaml:"enabled"`
	Address        string `yaml:"address"`         // HTTP server address for pprof
	CPUProfilePath string `yaml:"cpu_profile"`     // Path for CPU profile output
	MemProfilePath string `yaml:"mem_profile"`     // Path for memory profile output
	BlockProfile   bool   `yaml:"block_profile"`   // Enable blocking profiling
	MutexProfile   bool   `yaml:"mutex_profile"`   // Enable mutex profiling
	GoroutineThreshold int `yaml:"goroutine_threshold"` // Warn if goroutines exceed this
}

// Profiler manages performance profiling
type Profiler struct {
	config Config
	logger *logging.Logger
	server *http.Server

	cpuFile *os.File

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new profiler
func New(config Config, logger *logging.Logger) (*Profiler, error) {
	if logger == nil {
		logger = logging.GetGlobal()
	}

	if config.Address == "" {
		config.Address = "localhost:6060"
	}

	if config.GoroutineThreshold == 0 {
		config.GoroutineThreshold = 10000
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &Profiler{
		config: config,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	return p, nil
}

// Start begins profiling
func (p *Profiler) Start() error {
	if !p.config.Enabled {
		p.logger.Info().Msg("Profiling disabled")
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Enable block profiling if configured
	if p.config.BlockProfile {
		runtime.SetBlockProfileRate(1)
		p.logger.Info().Msg("Block profiling enabled")
	}

	// Enable mutex profiling if configured
	if p.config.MutexProfile {
		runtime.SetMutexProfileFraction(1)
		p.logger.Info().Msg("Mutex profiling enabled")
	}

	// Start CPU profiling if path is specified
	if p.config.CPUProfilePath != "" {
		if err := p.startCPUProfile(); err != nil {
			return fmt.Errorf("failed to start CPU profiling: %w", err)
		}
	}

	// Start HTTP server for pprof
	if p.config.Address != "" {
		mux := http.NewServeMux()

		// Register pprof handlers
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

		// Custom endpoints
		mux.HandleFunc("/debug/stats", p.statsHandler)
		mux.HandleFunc("/debug/gc", p.gcHandler)

		p.server = &http.Server{
			Addr:    p.config.Address,
			Handler: mux,
		}

		go func() {
			p.logger.Info().Str("address", p.config.Address).Msg("Starting profiling HTTP server")
			if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				p.logger.Error().Err(err).Msg("Profiling server error")
			}
		}()
	}

	// Start goroutine monitoring
	go p.monitorGoroutines()

	p.logger.Info().Msg("Profiling started")
	return nil
}

// Stop stops profiling
func (p *Profiler) Stop() error {
	if !p.config.Enabled {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Cancel background goroutines
	p.cancel()

	// Stop CPU profiling
	if p.cpuFile != nil {
		runtimepprof.StopCPUProfile()
		p.cpuFile.Close()
		p.logger.Info().Str("path", p.config.CPUProfilePath).Msg("CPU profile saved")
	}

	// Write memory profile if configured
	if p.config.MemProfilePath != "" {
		if err := p.writeMemProfile(); err != nil {
			p.logger.Error().Err(err).Msg("Failed to write memory profile")
		}
	}

	// Shutdown HTTP server
	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.server.Shutdown(ctx); err != nil {
			p.logger.Error().Err(err).Msg("Failed to shutdown profiling server")
		}
	}

	p.logger.Info().Msg("Profiling stopped")
	return nil
}

// startCPUProfile starts CPU profiling
func (p *Profiler) startCPUProfile() error {
	f, err := os.Create(p.config.CPUProfilePath)
	if err != nil {
		return err
	}

	if err := runtimepprof.StartCPUProfile(f); err != nil {
		f.Close()
		return err
	}

	p.cpuFile = f
	p.logger.Info().Str("path", p.config.CPUProfilePath).Msg("CPU profiling started")
	return nil
}

// writeMemProfile writes memory profile to file
func (p *Profiler) writeMemProfile() error {
	f, err := os.Create(p.config.MemProfilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	runtime.GC() // Get up-to-date statistics

	if err := runtimepprof.WriteHeapProfile(f); err != nil {
		return err
	}

	p.logger.Info().Str("path", p.config.MemProfilePath).Msg("Memory profile saved")
	return nil
}

// monitorGoroutines monitors goroutine count
func (p *Profiler) monitorGoroutines() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			count := runtime.NumGoroutine()
			if count > p.config.GoroutineThreshold {
				p.logger.Warn().
					Int("goroutines", count).
					Int("threshold", p.config.GoroutineThreshold).
					Msg("High goroutine count detected")
			} else {
				p.logger.Debug().Int("goroutines", count).Msg("Goroutine count")
			}
		}
	}
}

// statsHandler returns runtime statistics
func (p *Profiler) statsHandler(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	fmt.Fprintf(w, "Runtime Statistics\n")
	fmt.Fprintf(w, "==================\n\n")
	fmt.Fprintf(w, "Goroutines: %d\n", runtime.NumGoroutine())
	fmt.Fprintf(w, "CPUs: %d\n", runtime.NumCPU())
	fmt.Fprintf(w, "GOMAXPROCS: %d\n\n", runtime.GOMAXPROCS(0))

	fmt.Fprintf(w, "Memory Statistics\n")
	fmt.Fprintf(w, "=================\n\n")
	fmt.Fprintf(w, "Alloc: %d MB\n", m.Alloc/1024/1024)
	fmt.Fprintf(w, "TotalAlloc: %d MB\n", m.TotalAlloc/1024/1024)
	fmt.Fprintf(w, "Sys: %d MB\n", m.Sys/1024/1024)
	fmt.Fprintf(w, "Lookups: %d\n", m.Lookups)
	fmt.Fprintf(w, "Mallocs: %d\n", m.Mallocs)
	fmt.Fprintf(w, "Frees: %d\n", m.Frees)
	fmt.Fprintf(w, "HeapAlloc: %d MB\n", m.HeapAlloc/1024/1024)
	fmt.Fprintf(w, "HeapSys: %d MB\n", m.HeapSys/1024/1024)
	fmt.Fprintf(w, "HeapIdle: %d MB\n", m.HeapIdle/1024/1024)
	fmt.Fprintf(w, "HeapInuse: %d MB\n", m.HeapInuse/1024/1024)
	fmt.Fprintf(w, "HeapReleased: %d MB\n", m.HeapReleased/1024/1024)
	fmt.Fprintf(w, "HeapObjects: %d\n\n", m.HeapObjects)

	fmt.Fprintf(w, "GC Statistics\n")
	fmt.Fprintf(w, "=============\n\n")
	fmt.Fprintf(w, "NumGC: %d\n", m.NumGC)
	fmt.Fprintf(w, "PauseTotalNs: %d ms\n", m.PauseTotalNs/1000000)
	if m.NumGC > 0 {
		fmt.Fprintf(w, "LastGC: %s\n", time.Unix(0, int64(m.LastGC)).Format(time.RFC3339))
		fmt.Fprintf(w, "PauseNs (last): %d µs\n", m.PauseNs[(m.NumGC+255)%256]/1000)
	}
}

// gcHandler triggers garbage collection
func (p *Profiler) gcHandler(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	beforeAlloc := m.Alloc

	runtime.GC()

	runtime.ReadMemStats(&m)
	afterAlloc := m.Alloc

	fmt.Fprintf(w, "Garbage Collection Triggered\n")
	fmt.Fprintf(w, "============================\n\n")
	fmt.Fprintf(w, "Memory before GC: %d MB\n", beforeAlloc/1024/1024)
	fmt.Fprintf(w, "Memory after GC: %d MB\n", afterAlloc/1024/1024)
	if beforeAlloc > afterAlloc {
		fmt.Fprintf(w, "Memory freed: %d MB\n", (beforeAlloc-afterAlloc)/1024/1024)
	}
}

// GetMemoryStats returns current memory statistics
func GetMemoryStats() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

// GetGoroutineCount returns current goroutine count
func GetGoroutineCount() int {
	return runtime.NumGoroutine()
}
