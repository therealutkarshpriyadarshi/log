package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Status represents the health status
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusDegraded  Status = "degraded"
)

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
	Status      Status                 `json:"status"`
	Message     string                 `json:"message,omitempty"`
	LastChecked time.Time              `json:"last_checked"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// HealthCheck represents a health check function
type HealthCheck func(ctx context.Context) ComponentHealth

// Checker manages health checks for all components
type Checker struct {
	mu         sync.RWMutex
	components map[string]HealthCheck
	lastStatus map[string]ComponentHealth
	timeout    time.Duration
}

// NewChecker creates a new health checker
func NewChecker(timeout time.Duration) *Checker {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &Checker{
		components: make(map[string]HealthCheck),
		lastStatus: make(map[string]ComponentHealth),
		timeout:    timeout,
	}
}

// Register registers a health check for a component
func (c *Checker) Register(name string, check HealthCheck) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.components[name] = check
}

// Unregister removes a health check
func (c *Checker) Unregister(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.components, name)
	delete(c.lastStatus, name)
}

// Check runs all health checks and returns the overall status
func (c *Checker) Check(ctx context.Context) map[string]ComponentHealth {
	c.mu.RLock()
	components := make(map[string]HealthCheck)
	for k, v := range c.components {
		components[k] = v
	}
	c.mu.RUnlock()

	results := make(map[string]ComponentHealth)
	var wg sync.WaitGroup

	for name, check := range components {
		wg.Add(1)
		go func(n string, chk HealthCheck) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
			defer cancel()

			result := chk(checkCtx)
			result.LastChecked = time.Now()

			c.mu.Lock()
			c.lastStatus[n] = result
			c.mu.Unlock()

			results[n] = result
		}(name, check)
	}

	wg.Wait()
	return results
}

// CheckComponent runs a single component's health check
func (c *Checker) CheckComponent(ctx context.Context, name string) (ComponentHealth, bool) {
	c.mu.RLock()
	check, exists := c.components[name]
	c.mu.RUnlock()

	if !exists {
		return ComponentHealth{}, false
	}

	checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	result := check(checkCtx)
	result.LastChecked = time.Now()

	c.mu.Lock()
	c.lastStatus[name] = result
	c.mu.Unlock()

	return result, true
}

// GetLastStatus returns the last known status of all components
func (c *Checker) GetLastStatus() map[string]ComponentHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := make(map[string]ComponentHealth)
	for k, v := range c.lastStatus {
		status[k] = v
	}
	return status
}

// OverallStatus returns the overall health status
func (c *Checker) OverallStatus(ctx context.Context) Status {
	results := c.Check(ctx)

	if len(results) == 0 {
		return StatusHealthy
	}

	hasUnhealthy := false
	hasDegraded := false

	for _, result := range results {
		switch result.Status {
		case StatusUnhealthy:
			hasUnhealthy = true
		case StatusDegraded:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return StatusUnhealthy
	}
	if hasDegraded {
		return StatusDegraded
	}
	return StatusHealthy
}

// HealthResponse represents the HTTP response for health checks
type HealthResponse struct {
	Status     Status                     `json:"status"`
	Components map[string]ComponentHealth `json:"components"`
	Timestamp  time.Time                  `json:"timestamp"`
}

// HTTPHandler returns an HTTP handler for health checks
func (c *Checker) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		results := c.Check(ctx)
		overall := StatusHealthy

		if len(results) == 0 {
			overall = StatusHealthy
		} else {
			hasUnhealthy := false
			hasDegraded := false

			for _, result := range results {
				switch result.Status {
				case StatusUnhealthy:
					hasUnhealthy = true
				case StatusDegraded:
					hasDegraded = true
				}
			}

			if hasUnhealthy {
				overall = StatusUnhealthy
			} else if hasDegraded {
				overall = StatusDegraded
			}
		}

		response := HealthResponse{
			Status:     overall,
			Components: results,
			Timestamp:  time.Now(),
		}

		statusCode := http.StatusOK
		if overall == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		} else if overall == StatusDegraded {
			statusCode = http.StatusOK // Still 200 for degraded
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(response)
	}
}

// LivenessHandler returns a simple liveness probe handler
func (c *Checker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "alive",
		})
	}
}

// ReadinessHandler returns a readiness probe handler
func (c *Checker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		status := c.OverallStatus(ctx)

		response := map[string]interface{}{
			"status":    status,
			"timestamp": time.Now(),
		}

		statusCode := http.StatusOK
		if status == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(response)
	}
}

// Helper functions to create common health checks

// AlwaysHealthy returns a health check that always reports healthy
func AlwaysHealthy() HealthCheck {
	return func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:  StatusHealthy,
			Message: "Component is healthy",
		}
	}
}

// CheckFunc creates a health check from a simple boolean function
func CheckFunc(check func() (bool, string)) HealthCheck {
	return func(ctx context.Context) ComponentHealth {
		healthy, message := check()
		status := StatusHealthy
		if !healthy {
			status = StatusUnhealthy
		}
		return ComponentHealth{
			Status:  status,
			Message: message,
		}
	}
}

// CheckWithMetadata creates a health check with metadata
func CheckWithMetadata(check func() (Status, string, map[string]interface{})) HealthCheck {
	return func(ctx context.Context) ComponentHealth {
		status, message, metadata := check()
		return ComponentHealth{
			Status:   status,
			Message:  message,
			Metadata: metadata,
		}
	}
}
