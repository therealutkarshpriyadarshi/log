package input

import (
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

func TestBaseInput(t *testing.T) {
	t.Run("NewBaseInput", func(t *testing.T) {
		base := NewBaseInput("test-input", "test", 100)

		if base.Name() != "test-input" {
			t.Errorf("expected name 'test-input', got '%s'", base.Name())
		}

		if base.Type() != "test" {
			t.Errorf("expected type 'test', got '%s'", base.Type())
		}

		if base.Events() == nil {
			t.Error("expected non-nil events channel")
		}
	})

	t.Run("SendEvent", func(t *testing.T) {
		base := NewBaseInput("test-input", "test", 10)
		defer base.Close()

		event := &types.LogEvent{
			Timestamp: time.Now(),
			Message:   "test message",
			Source:    "test",
		}

		sent := base.SendEvent(event)
		if !sent {
			t.Error("expected event to be sent successfully")
		}

		// Receive event
		select {
		case receivedEvent := <-base.Events():
			if receivedEvent.Message != event.Message {
				t.Errorf("expected message '%s', got '%s'", event.Message, receivedEvent.Message)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("timeout waiting for event")
		}
	})

	t.Run("SendEventAfterCancel", func(t *testing.T) {
		base := NewBaseInput("test-input", "test", 10)
		base.Cancel()
		base.Close()

		event := &types.LogEvent{
			Timestamp: time.Now(),
			Message:   "test message",
			Source:    "test",
		}

		sent := base.SendEvent(event)
		if sent {
			t.Error("expected event send to fail after cancel")
		}
	})
}

func TestHealthStatus(t *testing.T) {
	tests := []struct {
		name   string
		status HealthStatus
		valid  bool
	}{
		{"healthy", HealthStatusHealthy, true},
		{"degraded", HealthStatusDegraded, true},
		{"unhealthy", HealthStatusUnhealthy, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := Health{
				Status:  tt.status,
				Message: "test message",
			}

			if health.Status != tt.status {
				t.Errorf("expected status '%s', got '%s'", tt.status, health.Status)
			}
		})
	}
}
