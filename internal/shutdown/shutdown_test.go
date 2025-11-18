package shutdown

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/logging"
)

func TestNew(t *testing.T) {
	logger := logging.New(logging.Config{Level: "info", Format: "json"})

	cfg := Config{
		Timeout: 10 * time.Second,
		Logger:  logger,
	}

	manager := New(cfg)

	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}
	if manager.timeout != 10*time.Second {
		t.Errorf("Expected timeout 10s, got %v", manager.timeout)
	}
}

func TestRegisterFunc(t *testing.T) {
	logger := logging.New(logging.Config{Level: "info", Format: "json"})
	manager := New(Config{Logger: logger})

	fn := func(ctx context.Context) error {
		return nil
	}

	manager.RegisterFunc("test", fn)

	if len(manager.shutdownFuncs) != 1 {
		t.Errorf("Expected 1 shutdown function, got %d", len(manager.shutdownFuncs))
	}
}

func TestShutdown_Success(t *testing.T) {
	logger := logging.New(logging.Config{Level: "info", Format: "json"})
	manager := New(Config{
		Logger:  logger,
		Timeout: 5 * time.Second,
	})

	var callOrder []int
	for i := 0; i < 3; i++ {
		index := i
		manager.RegisterFunc("test", func(ctx context.Context) error {
			callOrder = append(callOrder, index)
			return nil
		})
	}

	manager.Shutdown()

	// Wait for shutdown to complete
	select {
	case <-manager.Done():
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown did not complete in time")
	}

	if len(callOrder) != 3 {
		t.Errorf("Expected 3 functions to be called, got %d", len(callOrder))
	}
}

func TestShutdown_WithError(t *testing.T) {
	logger := logging.New(logging.Config{Level: "info", Format: "json"})
	manager := New(Config{
		Logger:  logger,
		Timeout: 5 * time.Second,
	})

	manager.RegisterFunc("success", func(ctx context.Context) error {
		return nil
	})

	manager.RegisterFunc("error", func(ctx context.Context) error {
		return errors.New("test error")
	})

	manager.Shutdown()

	// Wait for shutdown to complete
	select {
	case <-manager.Done():
		// Should complete even with errors
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown did not complete in time")
	}
}

func TestShutdown_Timeout(t *testing.T) {
	logger := logging.New(logging.Config{Level: "info", Format: "json"})
	manager := New(Config{
		Logger:  logger,
		Timeout: 100 * time.Millisecond,
	})

	manager.RegisterFunc("slow", func(ctx context.Context) error {
		time.Sleep(1 * time.Second)
		return nil
	})

	start := time.Now()
	manager.Shutdown()

	// Wait for shutdown to complete
	<-manager.Done()

	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Errorf("Shutdown took too long: %v", elapsed)
	}
}

func TestShutdownChannel(t *testing.T) {
	logger := logging.New(logging.Config{Level: "info", Format: "json"})
	manager := New(Config{Logger: logger})

	select {
	case <-manager.ShutdownChannel():
		t.Error("Shutdown channel should not be closed initially")
	default:
		// Expected
	}

	manager.Shutdown()

	select {
	case <-manager.ShutdownChannel():
		// Expected
	case <-time.After(1 * time.Second):
		t.Error("Shutdown channel should be closed after Shutdown()")
	}
}

func TestWaitForSignal(t *testing.T) {
	// This test is hard to unit test properly without actually sending signals
	// We'll just test that it doesn't panic and registers the signal handler
	logger := logging.New(logging.Config{Level: "info", Format: "json"})
	manager := New(Config{Logger: logger})

	go func() {
		time.Sleep(100 * time.Millisecond)
		manager.Shutdown()
	}()

	manager.WaitForSignal(syscall.SIGTERM)
}

func TestWaitWithTimeout(t *testing.T) {
	logger := logging.New(logging.Config{Level: "info", Format: "json"})
	manager := New(Config{Logger: logger})

	manager.RegisterFunc("fast", func(ctx context.Context) error {
		return nil
	})

	go func() {
		manager.Shutdown()
	}()

	err := manager.WaitWithTimeout(5 * time.Second)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestWaitWithTimeout_Timeout(t *testing.T) {
	logger := logging.New(logging.Config{Level: "info", Format: "json"})
	manager := New(Config{
		Logger:  logger,
		Timeout: 5 * time.Second,
	})

	manager.RegisterFunc("never", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	go func() {
		manager.Shutdown()
	}()

	err := manager.WaitWithTimeout(100 * time.Millisecond)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

type mockComponent struct {
	name     string
	stopFunc func(context.Context) error
}

func (m *mockComponent) Name() string {
	return m.name
}

func (m *mockComponent) Stop(ctx context.Context) error {
	if m.stopFunc != nil {
		return m.stopFunc(ctx)
	}
	return nil
}

func TestRegisterComponent(t *testing.T) {
	logger := logging.New(logging.Config{Level: "info", Format: "json"})
	manager := New(Config{Logger: logger})

	component := &mockComponent{
		name: "test-component",
		stopFunc: func(ctx context.Context) error {
			return nil
		},
	}

	manager.RegisterComponent(component)

	if len(manager.shutdownFuncs) != 1 {
		t.Errorf("Expected 1 shutdown function, got %d", len(manager.shutdownFuncs))
	}
}
