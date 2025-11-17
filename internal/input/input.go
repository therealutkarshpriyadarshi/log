package input

import (
	"context"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// Input defines the interface that all input sources must implement
type Input interface {
	// Name returns the name of the input plugin
	Name() string

	// Type returns the type of the input (file, syslog, http, kubernetes)
	Type() string

	// Start begins collecting logs and sending them to the events channel
	Start() error

	// Stop stops the input gracefully
	Stop() error

	// Events returns the channel for log events
	Events() <-chan *types.LogEvent

	// Health returns the health status of the input
	Health() Health
}

// Health represents the health status of an input
type Health struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// HealthStatus represents the status of health check
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// BaseInput provides common functionality for all inputs
type BaseInput struct {
	ctx      context.Context
	cancel   context.CancelFunc
	eventCh  chan *types.LogEvent
	name     string
	inputType string
}

// NewBaseInput creates a new BaseInput
func NewBaseInput(name, inputType string, bufferSize int) *BaseInput {
	ctx, cancel := context.WithCancel(context.Background())
	return &BaseInput{
		ctx:       ctx,
		cancel:    cancel,
		eventCh:   make(chan *types.LogEvent, bufferSize),
		name:      name,
		inputType: inputType,
	}
}

// Name returns the name of the input
func (b *BaseInput) Name() string {
	return b.name
}

// Type returns the type of the input
func (b *BaseInput) Type() string {
	return b.inputType
}

// Events returns the channel for log events
func (b *BaseInput) Events() <-chan *types.LogEvent {
	return b.eventCh
}

// Context returns the context
func (b *BaseInput) Context() context.Context {
	return b.ctx
}

// Cancel cancels the context
func (b *BaseInput) Cancel() {
	b.cancel()
}

// SendEvent sends an event to the channel
func (b *BaseInput) SendEvent(event *types.LogEvent) bool {
	select {
	case b.eventCh <- event:
		return true
	case <-b.ctx.Done():
		return false
	}
}

// Close closes the event channel
func (b *BaseInput) Close() {
	close(b.eventCh)
}
