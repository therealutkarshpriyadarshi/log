package dlq

import (
	"errors"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

func TestNewDeadLetterQueue(t *testing.T) {
	dir := t.TempDir()

	config := DLQConfig{
		Dir:     dir,
		MaxSize: 100,
		MaxAge:  1 * time.Hour,
	}

	dlq, err := NewDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("NewDeadLetterQueue() error = %v", err)
	}
	defer dlq.Close()

	if dlq.Size() != 0 {
		t.Errorf("initial size = %d, want 0", dlq.Size())
	}
}

func TestDLQ_EnqueueDequeue(t *testing.T) {
	dir := t.TempDir()

	config := DLQConfig{
		Dir:     dir,
		MaxSize: 100,
	}

	dlq, err := NewDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("NewDeadLetterQueue() error = %v", err)
	}
	defer dlq.Close()

	// Enqueue an event
	event := &types.LogEvent{
		Message: "test message",
		Source:  "test",
	}

	testErr := errors.New("test error")
	metadata := map[string]string{"key": "value"}

	if err := dlq.Enqueue(event, testErr, metadata); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if dlq.Size() != 1 {
		t.Errorf("size = %d, want 1", dlq.Size())
	}

	// Dequeue the event
	entry, err := dlq.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}

	if entry.Event.Message != event.Message {
		t.Errorf("message = %s, want %s", entry.Event.Message, event.Message)
	}

	if entry.Error != testErr.Error() {
		t.Errorf("error = %s, want %s", entry.Error, testErr.Error())
	}

	if dlq.Size() != 0 {
		t.Errorf("size after dequeue = %d, want 0", dlq.Size())
	}
}

func TestDLQ_MaxSize(t *testing.T) {
	dir := t.TempDir()

	config := DLQConfig{
		Dir:     dir,
		MaxSize: 5,
	}

	dlq, err := NewDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("NewDeadLetterQueue() error = %v", err)
	}
	defer dlq.Close()

	// Fill the DLQ
	for i := 0; i < 5; i++ {
		event := &types.LogEvent{Message: "test"}
		if err := dlq.Enqueue(event, errors.New("error"), nil); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	// Try to enqueue one more - should fail
	event := &types.LogEvent{Message: "test"}
	err = dlq.Enqueue(event, errors.New("error"), nil)
	if err != ErrDLQFull {
		t.Errorf("expected ErrDLQFull, got %v", err)
	}

	metrics := dlq.Metrics()
	if metrics.Dropped == 0 {
		t.Errorf("expected dropped count > 0")
	}
}

func TestDLQ_Peek(t *testing.T) {
	dir := t.TempDir()

	config := DLQConfig{
		Dir:     dir,
		MaxSize: 100,
	}

	dlq, err := NewDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("NewDeadLetterQueue() error = %v", err)
	}
	defer dlq.Close()

	// Enqueue an event
	event := &types.LogEvent{Message: "test"}
	if err := dlq.Enqueue(event, errors.New("error"), nil); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Peek should not remove the entry
	entry, err := dlq.Peek()
	if err != nil {
		t.Fatalf("Peek() error = %v", err)
	}

	if entry.Event.Message != event.Message {
		t.Errorf("message = %s, want %s", entry.Event.Message, event.Message)
	}

	if dlq.Size() != 1 {
		t.Errorf("size after peek = %d, want 1", dlq.Size())
	}
}

func TestDLQ_GetAll(t *testing.T) {
	dir := t.TempDir()

	config := DLQConfig{
		Dir:     dir,
		MaxSize: 100,
	}

	dlq, err := NewDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("NewDeadLetterQueue() error = %v", err)
	}
	defer dlq.Close()

	// Enqueue multiple events
	for i := 0; i < 5; i++ {
		event := &types.LogEvent{Message: "test"}
		if err := dlq.Enqueue(event, errors.New("error"), nil); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	entries, err := dlq.GetAll()
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}

	if len(entries) != 5 {
		t.Errorf("got %d entries, want 5", len(entries))
	}
}

func TestDLQ_Clear(t *testing.T) {
	dir := t.TempDir()

	config := DLQConfig{
		Dir:     dir,
		MaxSize: 100,
	}

	dlq, err := NewDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("NewDeadLetterQueue() error = %v", err)
	}
	defer dlq.Close()

	// Enqueue some events
	for i := 0; i < 3; i++ {
		event := &types.LogEvent{Message: "test"}
		if err := dlq.Enqueue(event, errors.New("error"), nil); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	if err := dlq.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	if dlq.Size() != 0 {
		t.Errorf("size after clear = %d, want 0", dlq.Size())
	}
}

func TestDLQ_Persistence(t *testing.T) {
	dir := t.TempDir()

	config := DLQConfig{
		Dir:           dir,
		MaxSize:       100,
		FlushInterval: 100 * time.Millisecond,
	}

	// Create DLQ and enqueue events
	dlq1, err := NewDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("NewDeadLetterQueue() error = %v", err)
	}

	for i := 0; i < 3; i++ {
		event := &types.LogEvent{Message: "persistent"}
		if err := dlq1.Enqueue(event, errors.New("error"), nil); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	if err := dlq1.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	dlq1.Close()

	// Create new DLQ with same directory
	dlq2, err := NewDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("NewDeadLetterQueue() error = %v", err)
	}
	defer dlq2.Close()

	// Should have loaded the entries
	if dlq2.Size() != 3 {
		t.Errorf("size after reload = %d, want 3", dlq2.Size())
	}
}

func TestDLQ_Retry(t *testing.T) {
	dir := t.TempDir()

	config := DLQConfig{
		Dir:     dir,
		MaxSize: 100,
	}

	dlq, err := NewDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("NewDeadLetterQueue() error = %v", err)
	}
	defer dlq.Close()

	// Enqueue and dequeue
	event := &types.LogEvent{Message: "test"}
	if err := dlq.Enqueue(event, errors.New("error"), nil); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	entry, _ := dlq.Dequeue()
	if entry.Retries != 0 {
		t.Errorf("initial retries = %d, want 0", entry.Retries)
	}

	// Retry
	if err := dlq.Retry(entry); err != nil {
		t.Fatalf("Retry() error = %v", err)
	}

	// Dequeue again
	retried, _ := dlq.Dequeue()
	if retried.Retries != 1 {
		t.Errorf("retries after retry = %d, want 1", retried.Retries)
	}
}

func TestDLQ_Metrics(t *testing.T) {
	dir := t.TempDir()

	config := DLQConfig{
		Dir:     dir,
		MaxSize: 100,
	}

	dlq, err := NewDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("NewDeadLetterQueue() error = %v", err)
	}
	defer dlq.Close()

	// Enqueue some events
	for i := 0; i < 5; i++ {
		event := &types.LogEvent{Message: "test"}
		if err := dlq.Enqueue(event, errors.New("error"), nil); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	metrics := dlq.Metrics()

	if metrics.Enqueued != 5 {
		t.Errorf("Enqueued = %d, want 5", metrics.Enqueued)
	}

	if metrics.CurrentSize != 5 {
		t.Errorf("CurrentSize = %d, want 5", metrics.CurrentSize)
	}

	utilization := metrics.Utilization()
	if utilization != 5.0 { // 5/100 * 100
		t.Errorf("Utilization = %f, want 5.0", utilization)
	}

	// Dequeue some
	for i := 0; i < 2; i++ {
		_, _ = dlq.Dequeue()
	}

	metrics = dlq.Metrics()

	if metrics.Dequeued != 2 {
		t.Errorf("Dequeued = %d, want 2", metrics.Dequeued)
	}

	if metrics.CurrentSize != 3 {
		t.Errorf("CurrentSize = %d, want 3", metrics.CurrentSize)
	}
}

func TestDLQ_Close(t *testing.T) {
	dir := t.TempDir()

	config := DLQConfig{
		Dir:     dir,
		MaxSize: 100,
	}

	dlq, err := NewDeadLetterQueue(config)
	if err != nil {
		t.Fatalf("NewDeadLetterQueue() error = %v", err)
	}

	// Close the DLQ
	if err := dlq.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Try to enqueue after close
	event := &types.LogEvent{Message: "test"}
	err = dlq.Enqueue(event, errors.New("error"), nil)
	if err != ErrDLQClosed {
		t.Errorf("expected ErrDLQClosed, got %v", err)
	}

	// Try to close again
	err = dlq.Close()
	if err != ErrDLQClosed {
		t.Errorf("expected ErrDLQClosed on second close, got %v", err)
	}
}
