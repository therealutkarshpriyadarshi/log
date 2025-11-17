package input

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/logging"
)

func TestHTTPInput(t *testing.T) {
	logger := logging.New(logging.Config{
		Level:  "info",
		Format: "json",
	})

	t.Run("NewHTTPInput", func(t *testing.T) {
		config := &HTTPConfig{
			Address:    "localhost:8080",
			BufferSize: 100,
		}

		input, err := NewHTTPInput("test-http", config, logger)
		if err != nil {
			t.Fatalf("failed to create HTTP input: %v", err)
		}

		if input.Name() != "test-http" {
			t.Errorf("expected name 'test-http', got '%s'", input.Name())
		}

		if input.Type() != "http" {
			t.Errorf("expected type 'http', got '%s'", input.Type())
		}

		if config.Path != "/log" {
			t.Errorf("expected default path '/log', got '%s'", config.Path)
		}

		if config.BatchPath != "/logs" {
			t.Errorf("expected default batch path '/logs', got '%s'", config.BatchPath)
		}
	})

	t.Run("HandleSingleEvent", func(t *testing.T) {
		config := &HTTPConfig{
			Address:    "localhost:8081",
			BufferSize: 100,
		}

		input, err := NewHTTPInput("test-http", config, logger)
		if err != nil {
			t.Fatalf("failed to create HTTP input: %v", err)
		}

		// Create test request
		eventData := map[string]interface{}{
			"message": "test log message",
			"level":   "info",
		}
		body, _ := json.Marshal(eventData)

		req := httptest.NewRequest(http.MethodPost, "/log", bytes.NewReader(body))
		w := httptest.NewRecorder()

		input.handleSingleEvent(w, req)

		if w.Code != http.StatusAccepted {
			t.Errorf("expected status %d, got %d", http.StatusAccepted, w.Code)
		}

		// Check if event was received
		select {
		case event := <-input.Events():
			if event.Message != "test log message" {
				t.Errorf("expected message 'test log message', got '%s'", event.Message)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("timeout waiting for event")
		}
	})

	t.Run("HandleBatchEvents", func(t *testing.T) {
		config := &HTTPConfig{
			Address:    "localhost:8082",
			BufferSize: 100,
		}

		input, err := NewHTTPInput("test-http", config, logger)
		if err != nil {
			t.Fatalf("failed to create HTTP input: %v", err)
		}

		// Create test batch request
		events := []map[string]interface{}{
			{"message": "event 1", "level": "info"},
			{"message": "event 2", "level": "warn"},
			{"message": "event 3", "level": "error"},
		}
		body, _ := json.Marshal(events)

		req := httptest.NewRequest(http.MethodPost, "/logs", bytes.NewReader(body))
		w := httptest.NewRecorder()

		input.handleBatchEvents(w, req)

		if w.Code != http.StatusAccepted {
			t.Errorf("expected status %d, got %d", http.StatusAccepted, w.Code)
		}

		// Check if all events were received
		receivedCount := 0
		timeout := time.After(500 * time.Millisecond)
		for receivedCount < 3 {
			select {
			case <-input.Events():
				receivedCount++
			case <-timeout:
				t.Errorf("expected 3 events, got %d", receivedCount)
				return
			}
		}
	})

	t.Run("AuthMiddleware", func(t *testing.T) {
		apiKey := "test-api-key-123"
		config := &HTTPConfig{
			Address:    "localhost:8083",
			APIKeys:    []string{apiKey},
			BufferSize: 100,
		}

		input, err := NewHTTPInput("test-http", config, logger)
		if err != nil {
			t.Fatalf("failed to create HTTP input: %v", err)
		}

		// Test without API key
		req := httptest.NewRequest(http.MethodPost, "/log", nil)
		w := httptest.NewRecorder()

		handler := input.authMiddleware(http.HandlerFunc(input.handleSingleEvent))
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
		}

		// Test with valid API key
		req = httptest.NewRequest(http.MethodPost, "/log", bytes.NewReader([]byte(`{"message":"test"}`)))
		req.Header.Set("X-API-Key", apiKey)
		w = httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusAccepted {
			t.Errorf("expected status %d, got %d", http.StatusAccepted, w.Code)
		}
	})

	t.Run("RateLimitMiddleware", func(t *testing.T) {
		config := &HTTPConfig{
			Address:    "localhost:8084",
			RateLimit:  2, // 2 requests per second
			BufferSize: 100,
		}

		input, err := NewHTTPInput("test-http", config, logger)
		if err != nil {
			t.Fatalf("failed to create HTTP input: %v", err)
		}

		handler := input.rateLimitMiddleware(http.HandlerFunc(input.handleSingleEvent))

		// Send requests rapidly
		successCount := 0
		rateLimitCount := 0

		for i := 0; i < 10; i++ {
			req := httptest.NewRequest(http.MethodPost, "/log", bytes.NewReader([]byte(`{"message":"test"}`)))
			req.RemoteAddr = "192.168.1.1:12345"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code == http.StatusAccepted {
				successCount++
			} else if w.Code == http.StatusTooManyRequests {
				rateLimitCount++
			}
		}

		// Should have some rate limited requests
		if rateLimitCount == 0 {
			t.Error("expected some requests to be rate limited")
		}
	})

	t.Run("HealthEndpoint", func(t *testing.T) {
		config := &HTTPConfig{
			Address:    "localhost:8085",
			BufferSize: 100,
		}

		input, err := NewHTTPInput("test-http", config, logger)
		if err != nil {
			t.Fatalf("failed to create HTTP input: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		input.handleHealth(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		var health Health
		if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
			t.Fatalf("failed to decode health response: %v", err)
		}

		if health.Status != HealthStatusHealthy {
			t.Errorf("expected status %s, got %s", HealthStatusHealthy, health.Status)
		}
	})

	t.Run("MetricsEndpoint", func(t *testing.T) {
		config := &HTTPConfig{
			Address:    "localhost:8086",
			BufferSize: 100,
		}

		input, err := NewHTTPInput("test-http", config, logger)
		if err != nil {
			t.Fatalf("failed to create HTTP input: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		w := httptest.NewRecorder()

		input.handleMetrics(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		var metrics map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&metrics); err != nil {
			t.Fatalf("failed to decode metrics response: %v", err)
		}

		if _, ok := metrics["requests_total"]; !ok {
			t.Error("expected requests_total metric")
		}

		if _, ok := metrics["events_total"]; !ok {
			t.Error("expected events_total metric")
		}
	})
}
