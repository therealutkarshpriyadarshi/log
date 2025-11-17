package profiling

import (
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/logging"
)

func TestNew(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Address: "localhost:6060",
	}

	logger := logging.New(logging.Config{
		Level:  "info",
		Format: "console",
	})

	p, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create profiler: %v", err)
	}

	if p == nil {
		t.Fatal("Profiler is nil")
	}

	if p.config.Address != "localhost:6060" {
		t.Errorf("Expected address localhost:6060, got %s", p.config.Address)
	}

	if p.config.GoroutineThreshold != 10000 {
		t.Errorf("Expected goroutine threshold 10000, got %d", p.config.GoroutineThreshold)
	}
}

func TestStartStop(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Address:      "localhost:16060", // Use different port to avoid conflicts
		BlockProfile: true,
		MutexProfile: true,
	}

	logger := logging.New(logging.Config{
		Level:  "info",
		Format: "console",
	})

	p, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create profiler: %v", err)
	}

	// Start profiling
	if err := p.Start(); err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Verify HTTP server is running
	resp, err := http.Get("http://localhost:16060/debug/pprof/")
	if err != nil {
		t.Fatalf("Failed to connect to profiling server: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Stop profiling
	if err := p.Stop(); err != nil {
		t.Fatalf("Failed to stop profiler: %v", err)
	}

	// Give it a moment to stop
	time.Sleep(100 * time.Millisecond)

	// Verify server is stopped
	_, err = http.Get("http://localhost:16060/debug/pprof/")
	if err == nil {
		t.Error("Expected error connecting to stopped server, got nil")
	}
}

func TestDisabled(t *testing.T) {
	cfg := Config{
		Enabled: false,
	}

	logger := logging.New(logging.Config{
		Level:  "info",
		Format: "console",
	})

	p, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create profiler: %v", err)
	}

	// Start should succeed but do nothing
	if err := p.Start(); err != nil {
		t.Fatalf("Failed to start disabled profiler: %v", err)
	}

	// Stop should succeed but do nothing
	if err := p.Stop(); err != nil {
		t.Fatalf("Failed to stop disabled profiler: %v", err)
	}
}

func TestBlockAndMutexProfiling(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Address:      "",
		BlockProfile: true,
		MutexProfile: true,
	}

	logger := logging.New(logging.Config{
		Level:  "info",
		Format: "console",
	})

	p, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create profiler: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}

	// Verify block profiling is enabled
	if runtime.SetBlockProfileRate(0) != 1 {
		t.Error("Block profiling not enabled")
	}
	runtime.SetBlockProfileRate(1) // Reset

	// Verify mutex profiling is enabled
	if runtime.SetMutexProfileFraction(0) != 1 {
		t.Error("Mutex profiling not enabled")
	}
	runtime.SetMutexProfileFraction(1) // Reset

	if err := p.Stop(); err != nil {
		t.Fatalf("Failed to stop profiler: %v", err)
	}
}

func TestGetMemoryStats(t *testing.T) {
	stats := GetMemoryStats()

	if stats.Alloc == 0 {
		t.Error("Expected non-zero Alloc")
	}

	if stats.Sys == 0 {
		t.Error("Expected non-zero Sys")
	}
}

func TestGetGoroutineCount(t *testing.T) {
	count := GetGoroutineCount()

	if count < 1 {
		t.Errorf("Expected at least 1 goroutine, got %d", count)
	}
}

func TestStatsHandler(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Address: "localhost:16061",
	}

	logger := logging.New(logging.Config{
		Level:  "info",
		Format: "console",
	})

	p, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create profiler: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}
	defer p.Stop()

	time.Sleep(100 * time.Millisecond)

	// Test stats endpoint
	resp, err := http.Get("http://localhost:16061/debug/stats")
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestGCHandler(t *testing.T) {
	cfg := Config{
		Enabled: true,
		Address: "localhost:16062",
	}

	logger := logging.New(logging.Config{
		Level:  "info",
		Format: "console",
	})

	p, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create profiler: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("Failed to start profiler: %v", err)
	}
	defer p.Stop()

	time.Sleep(100 * time.Millisecond)

	// Test GC endpoint
	resp, err := http.Get("http://localhost:16062/debug/gc")
	if err != nil {
		t.Fatalf("Failed to trigger GC: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}
