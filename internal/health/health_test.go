package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewChecker(t *testing.T) {
	c := NewChecker(5 * time.Second)
	if c == nil {
		t.Fatal("NewChecker returned nil")
	}

	if c.timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", c.timeout)
	}

	// Test default timeout
	c2 := NewChecker(0)
	if c2.timeout != 5*time.Second {
		t.Errorf("Expected default timeout 5s, got %v", c2.timeout)
	}
}

func TestRegisterUnregister(t *testing.T) {
	c := NewChecker(5 * time.Second)

	check := AlwaysHealthy()
	c.Register("test", check)

	if len(c.components) != 1 {
		t.Errorf("Expected 1 component, got %d", len(c.components))
	}

	c.Unregister("test")

	if len(c.components) != 0 {
		t.Errorf("Expected 0 components, got %d", len(c.components))
	}
}

func TestCheck(t *testing.T) {
	c := NewChecker(5 * time.Second)

	c.Register("component1", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "Component 1 is healthy",
		}
	})

	c.Register("component2", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusDegraded,
			Message: "Component 2 is degraded",
		}
	})

	ctx := context.Background()
	results := c.Check(ctx)

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	if results["component1"].Status != StatusHealthy {
		t.Errorf("Expected component1 to be healthy, got %s", results["component1"].Status)
	}

	if results["component2"].Status != StatusDegraded {
		t.Errorf("Expected component2 to be degraded, got %s", results["component2"].Status)
	}

	// Verify last checked time is set
	if results["component1"].LastChecked.IsZero() {
		t.Error("LastChecked should be set")
	}
}

func TestCheckComponent(t *testing.T) {
	c := NewChecker(5 * time.Second)

	c.Register("test", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "Test component is healthy",
		}
	})

	ctx := context.Background()
	result, exists := c.CheckComponent(ctx, "test")

	if !exists {
		t.Fatal("Component should exist")
	}

	if result.Status != StatusHealthy {
		t.Errorf("Expected healthy status, got %s", result.Status)
	}

	_, exists = c.CheckComponent(ctx, "nonexistent")
	if exists {
		t.Error("Nonexistent component should not exist")
	}
}

func TestGetLastStatus(t *testing.T) {
	c := NewChecker(5 * time.Second)

	c.Register("test", AlwaysHealthy())

	ctx := context.Background()
	c.Check(ctx)

	lastStatus := c.GetLastStatus()

	if len(lastStatus) != 1 {
		t.Fatalf("Expected 1 last status, got %d", len(lastStatus))
	}

	if lastStatus["test"].Status != StatusHealthy {
		t.Errorf("Expected healthy status, got %s", lastStatus["test"].Status)
	}
}

func TestOverallStatus(t *testing.T) {
	tests := []struct {
		name     string
		checks   map[string]HealthCheck
		expected Status
	}{
		{
			name:     "no checks",
			checks:   map[string]HealthCheck{},
			expected: StatusHealthy,
		},
		{
			name: "all healthy",
			checks: map[string]HealthCheck{
				"c1": func(ctx context.Context) ComponentHealth {
					return ComponentHealth{Status: StatusHealthy}
				},
				"c2": func(ctx context.Context) ComponentHealth {
					return ComponentHealth{Status: StatusHealthy}
				},
			},
			expected: StatusHealthy,
		},
		{
			name: "one degraded",
			checks: map[string]HealthCheck{
				"c1": func(ctx context.Context) ComponentHealth {
					return ComponentHealth{Status: StatusHealthy}
				},
				"c2": func(ctx context.Context) ComponentHealth {
					return ComponentHealth{Status: StatusDegraded}
				},
			},
			expected: StatusDegraded,
		},
		{
			name: "one unhealthy",
			checks: map[string]HealthCheck{
				"c1": func(ctx context.Context) ComponentHealth {
					return ComponentHealth{Status: StatusHealthy}
				},
				"c2": func(ctx context.Context) ComponentHealth {
					return ComponentHealth{Status: StatusUnhealthy}
				},
			},
			expected: StatusUnhealthy,
		},
		{
			name: "unhealthy overrides degraded",
			checks: map[string]HealthCheck{
				"c1": func(ctx context.Context) ComponentHealth {
					return ComponentHealth{Status: StatusDegraded}
				},
				"c2": func(ctx context.Context) ComponentHealth {
					return ComponentHealth{Status: StatusUnhealthy}
				},
			},
			expected: StatusUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewChecker(5 * time.Second)
			for name, check := range tt.checks {
				c.Register(name, check)
			}

			ctx := context.Background()
			status := c.OverallStatus(ctx)

			if status != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, status)
			}
		})
	}
}

func TestHTTPHandler(t *testing.T) {
	c := NewChecker(5 * time.Second)

	c.Register("component1", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "Component 1 is healthy",
		}
	})

	c.Register("component2", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusDegraded,
			Message: "Component 2 is degraded",
		}
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler := c.HTTPHandler()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != StatusDegraded {
		t.Errorf("Expected degraded status, got %s", response.Status)
	}

	if len(response.Components) != 2 {
		t.Errorf("Expected 2 components, got %d", len(response.Components))
	}
}

func TestHTTPHandlerUnhealthy(t *testing.T) {
	c := NewChecker(5 * time.Second)

	c.Register("component1", func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusUnhealthy,
			Message: "Component 1 is unhealthy",
		}
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler := c.HTTPHandler()
	handler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != StatusUnhealthy {
		t.Errorf("Expected unhealthy status, got %s", response.Status)
	}
}

func TestLivenessHandler(t *testing.T) {
	c := NewChecker(5 * time.Second)

	req := httptest.NewRequest("GET", "/health/live", nil)
	w := httptest.NewRecorder()

	handler := c.LivenessHandler()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "alive" {
		t.Errorf("Expected status 'alive', got %s", response["status"])
	}
}

func TestReadinessHandler(t *testing.T) {
	tests := []struct {
		name       string
		checks     map[string]HealthCheck
		statusCode int
		status     Status
	}{
		{
			name: "ready",
			checks: map[string]HealthCheck{
				"c1": AlwaysHealthy(),
			},
			statusCode: http.StatusOK,
			status:     StatusHealthy,
		},
		{
			name: "not ready",
			checks: map[string]HealthCheck{
				"c1": func(ctx context.Context) ComponentHealth {
					return ComponentHealth{Status: StatusUnhealthy}
				},
			},
			statusCode: http.StatusServiceUnavailable,
			status:     StatusUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewChecker(5 * time.Second)
			for name, check := range tt.checks {
				c.Register(name, check)
			}

			req := httptest.NewRequest("GET", "/health/ready", nil)
			w := httptest.NewRecorder()

			handler := c.ReadinessHandler()
			handler(w, req)

			if w.Code != tt.statusCode {
				t.Errorf("Expected status %d, got %d", tt.statusCode, w.Code)
			}

			var response map[string]interface{}
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if Status(response["status"].(string)) != tt.status {
				t.Errorf("Expected status %s, got %s", tt.status, response["status"])
			}
		})
	}
}

func TestCheckFunc(t *testing.T) {
	check := CheckFunc(func() (bool, string) {
		return true, "All good"
	})

	result := check(context.Background())

	if result.Status != StatusHealthy {
		t.Errorf("Expected healthy status, got %s", result.Status)
	}

	if result.Message != "All good" {
		t.Errorf("Expected message 'All good', got %s", result.Message)
	}

	check2 := CheckFunc(func() (bool, string) {
		return false, "Something wrong"
	})

	result2 := check2(context.Background())

	if result2.Status != StatusUnhealthy {
		t.Errorf("Expected unhealthy status, got %s", result2.Status)
	}
}

func TestCheckWithMetadata(t *testing.T) {
	check := CheckWithMetadata(func() (Status, string, map[string]interface{}) {
		return StatusDegraded, "High latency", map[string]interface{}{
			"latency_ms": 500,
			"threshold":  100,
		}
	})

	result := check(context.Background())

	if result.Status != StatusDegraded {
		t.Errorf("Expected degraded status, got %s", result.Status)
	}

	if result.Message != "High latency" {
		t.Errorf("Expected message 'High latency', got %s", result.Message)
	}

	if result.Metadata["latency_ms"].(int) != 500 {
		t.Errorf("Expected latency_ms 500, got %v", result.Metadata["latency_ms"])
	}
}

func TestCheckTimeout(t *testing.T) {
	c := NewChecker(100 * time.Millisecond)

	c.Register("slow", func(ctx context.Context) ComponentHealth {
		select {
		case <-time.After(1 * time.Second):
			return ComponentHealth{Status: StatusHealthy}
		case <-ctx.Done():
			return ComponentHealth{
				Status:  StatusUnhealthy,
				Message: "Check timed out",
			}
		}
	})

	ctx := context.Background()
	results := c.Check(ctx)

	// The check should have timed out
	if results["slow"].Status == StatusHealthy {
		t.Error("Expected check to timeout")
	}
}
