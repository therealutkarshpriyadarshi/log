package shutdown

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/logging"
)

// Manager handles graceful shutdown of the application
type Manager struct {
	logger         *logging.Logger
	timeout        time.Duration
	shutdownFuncs  []ShutdownFunc
	mu             sync.Mutex
	shutdownCh     chan struct{}
	shutdownOnce   sync.Once
	gracefulDone   chan struct{}
}

// ShutdownFunc is a function that performs cleanup during shutdown
type ShutdownFunc func(context.Context) error

// Config holds shutdown manager configuration
type Config struct {
	Timeout time.Duration
	Logger  *logging.Logger
}

// New creates a new shutdown manager
func New(cfg Config) *Manager {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	return &Manager{
		logger:       cfg.Logger,
		timeout:      cfg.Timeout,
		shutdownCh:   make(chan struct{}),
		gracefulDone: make(chan struct{}),
	}
}

// RegisterFunc registers a shutdown function to be called during shutdown
func (m *Manager) RegisterFunc(name string, fn ShutdownFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info().Str("component", name).Msg("Registered shutdown function")
	m.shutdownFuncs = append(m.shutdownFuncs, fn)
}

// WaitForSignal blocks until a shutdown signal is received
func (m *Manager) WaitForSignal(signals ...os.Signal) {
	if len(signals) == 0 {
		signals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, signals...)

	select {
	case sig := <-sigCh:
		m.logger.Info().
			Str("signal", sig.String()).
			Msg("Shutdown signal received")
		m.Shutdown()
	case <-m.shutdownCh:
		// Already shutting down
	}
}

// Shutdown initiates graceful shutdown
func (m *Manager) Shutdown() {
	m.shutdownOnce.Do(func() {
		close(m.shutdownCh)
		m.performShutdown()
	})
}

// performShutdown executes all registered shutdown functions
func (m *Manager) performShutdown() {
	m.logger.Info().
		Dur("timeout", m.timeout).
		Int("functions", len(m.shutdownFuncs)).
		Msg("Starting graceful shutdown")

	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	var wg sync.WaitGroup
	errors := make(chan error, len(m.shutdownFuncs))

	// Execute all shutdown functions in parallel
	for i, fn := range m.shutdownFuncs {
		wg.Add(1)
		go func(index int, shutdownFn ShutdownFunc) {
			defer wg.Done()

			m.logger.Debug().
				Int("index", index).
				Msg("Executing shutdown function")

			if err := shutdownFn(ctx); err != nil {
				m.logger.Error().
					Err(err).
					Int("index", index).
					Msg("Shutdown function failed")
				errors <- err
			} else {
				m.logger.Debug().
					Int("index", index).
					Msg("Shutdown function completed")
			}
		}(i, fn)
	}

	// Wait for all functions to complete or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
		close(errors)
	}()

	select {
	case <-done:
		// Check for errors
		var errorCount int
		for err := range errors {
			if err != nil {
				errorCount++
			}
		}
		if errorCount > 0 {
			m.logger.Warn().
				Int("errors", errorCount).
				Msg("Graceful shutdown completed with errors")
		} else {
			m.logger.Info().Msg("Graceful shutdown completed successfully")
		}
	case <-ctx.Done():
		m.logger.Warn().
			Dur("timeout", m.timeout).
			Msg("Graceful shutdown timed out, forcing exit")
	}

	close(m.gracefulDone)
}

// Done returns a channel that is closed when shutdown is complete
func (m *Manager) Done() <-chan struct{} {
	return m.gracefulDone
}

// ShutdownChannel returns a channel that is closed when shutdown is initiated
func (m *Manager) ShutdownChannel() <-chan struct{} {
	return m.shutdownCh
}

// Component represents a component that can be gracefully shut down
type Component interface {
	Stop(context.Context) error
	Name() string
}

// RegisterComponent registers a component for graceful shutdown
func (m *Manager) RegisterComponent(component Component) {
	m.RegisterFunc(component.Name(), component.Stop)
}

// HandlePanic recovers from panics and initiates shutdown
func (m *Manager) HandlePanic() {
	if r := recover(); r != nil {
		m.logger.Error().
			Interface("panic", r).
			Msg("Panic recovered, initiating shutdown")
		m.Shutdown()
		// Re-panic to maintain normal panic behavior
		panic(r)
	}
}

// WaitWithTimeout waits for shutdown to complete with a timeout
func (m *Manager) WaitWithTimeout(timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-m.Done():
		return nil
	case <-timer.C:
		return fmt.Errorf("shutdown did not complete within %v", timeout)
	}
}
