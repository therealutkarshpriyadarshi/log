package input

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/logging"
	"github.com/therealutkarshpriyadarshi/log/pkg/types"
	"golang.org/x/time/rate"
)

// HTTPConfig holds configuration for HTTP input
type HTTPConfig struct {
	// Address to bind to (e.g., "0.0.0.0:8080")
	Address string
	// Path for single event endpoint (default: "/log")
	Path string
	// Batch path for multiple events (default: "/logs")
	BatchPath string
	// API keys for authentication
	APIKeys []string
	// Rate limit per IP (requests per second)
	RateLimit int
	// Max request body size (bytes)
	MaxBodySize int64
	// TLS configuration
	TLSEnabled bool
	TLSCert    string
	TLSKey     string
	// Buffer size for events channel
	BufferSize int
	// Read timeout
	ReadTimeout time.Duration
	// Write timeout
	WriteTimeout time.Duration
}

// HTTPInput receives logs via HTTP API
type HTTPInput struct {
	*BaseInput
	config   *HTTPConfig
	logger   *logging.Logger
	server   *http.Server
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	stats    *httpStats
}

// httpStats tracks HTTP input statistics
type httpStats struct {
	requestsTotal   uint64
	eventsTotal     uint64
	errorsTotal     uint64
	authFailures    uint64
	rateLimitHits   uint64
}

// NewHTTPInput creates a new HTTP input
func NewHTTPInput(name string, config *HTTPConfig, logger *logging.Logger) (*HTTPInput, error) {
	if config.BufferSize == 0 {
		config.BufferSize = 10000
	}
	if config.Path == "" {
		config.Path = "/log"
	}
	if config.BatchPath == "" {
		config.BatchPath = "/logs"
	}
	if config.MaxBodySize == 0 {
		config.MaxBodySize = 10 * 1024 * 1024 // 10MB default
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 30 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 30 * time.Second
	}

	input := &HTTPInput{
		BaseInput: NewBaseInput(name, "http", config.BufferSize),
		config:    config,
		logger:    logger.WithComponent("input-http"),
		limiters:  make(map[string]*rate.Limiter),
		stats:     &httpStats{},
	}

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc(config.Path, input.handleSingleEvent)
	mux.HandleFunc(config.BatchPath, input.handleBatchEvents)
	mux.HandleFunc("/health", input.handleHealth)
	mux.HandleFunc("/metrics", input.handleMetrics)

	input.server = &http.Server{
		Addr:         config.Address,
		Handler:      input.authMiddleware(input.rateLimitMiddleware(mux)),
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	}

	return input, nil
}

// Start starts the HTTP receiver
func (h *HTTPInput) Start() error {
	h.logger.Info().
		Str("address", h.config.Address).
		Str("path", h.config.Path).
		Str("batch_path", h.config.BatchPath).
		Msg("HTTP receiver starting")

	go func() {
		var err error
		if h.config.TLSEnabled {
			err = h.server.ListenAndServeTLS(h.config.TLSCert, h.config.TLSKey)
		} else {
			err = h.server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			h.logger.Error().Err(err).Msg("HTTP server error")
		}
	}()

	return nil
}

// Stop stops the HTTP receiver
func (h *HTTPInput) Stop() error {
	h.logger.Info().Msg("Stopping HTTP receiver")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := h.server.Shutdown(ctx); err != nil {
		h.logger.Error().Err(err).Msg("Error shutting down HTTP server")
		return err
	}

	h.Cancel()
	h.Close()

	return nil
}

// Health returns the health status
func (h *HTTPInput) Health() Health {
	details := make(map[string]interface{})
	details["address"] = h.config.Address
	details["requests_total"] = atomic.LoadUint64(&h.stats.requestsTotal)
	details["events_total"] = atomic.LoadUint64(&h.stats.eventsTotal)
	details["errors_total"] = atomic.LoadUint64(&h.stats.errorsTotal)

	return Health{
		Status:  HealthStatusHealthy,
		Message: "HTTP receiver is running",
		Details: details,
	}
}

// authMiddleware checks API key authentication
func (h *HTTPInput) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health and metrics endpoints
		if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		// If no API keys configured, allow all
		if len(h.config.APIKeys) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		// Check API key in header
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			// Also check Authorization header
			apiKey = r.Header.Get("Authorization")
			if apiKey != "" && len(apiKey) > 7 && apiKey[:7] == "Bearer " {
				apiKey = apiKey[7:]
			}
		}

		// Validate API key
		valid := false
		for _, key := range h.config.APIKeys {
			if subtle.ConstantTimeCompare([]byte(apiKey), []byte(key)) == 1 {
				valid = true
				break
			}
		}

		if !valid {
			atomic.AddUint64(&h.stats.authFailures, 1)
			h.logger.Warn().Str("remote_addr", r.RemoteAddr).Msg("Authentication failed")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware applies rate limiting
func (h *HTTPInput) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip rate limiting for health and metrics
		if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		if h.config.RateLimit > 0 {
			limiter := h.getRateLimiter(r.RemoteAddr)
			if !limiter.Allow() {
				atomic.AddUint64(&h.stats.rateLimitHits, 1)
				h.logger.Warn().Str("remote_addr", r.RemoteAddr).Msg("Rate limit exceeded")
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// handleSingleEvent handles single event submission
func (h *HTTPInput) handleSingleEvent(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&h.stats.requestsTotal, 1)

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, h.config.MaxBodySize)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		atomic.AddUint64(&h.stats.errorsTotal, 1)
		h.logger.Error().Err(err).Msg("Failed to read request body")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Try to parse as JSON
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		// If not JSON, treat as plain text
		data = map[string]interface{}{
			"message": string(body),
		}
	}

	// Create log event
	event := &types.LogEvent{
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("%v", data["message"]),
		Source:    h.name,
		Fields:    data,
		Raw:       string(body),
	}

	// Add metadata
	event.Fields["remote_addr"] = r.RemoteAddr
	event.Fields["user_agent"] = r.UserAgent()
	event.Fields["input_type"] = "http"

	// Send event
	if !h.SendEvent(event) {
		atomic.AddUint64(&h.stats.errorsTotal, 1)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	atomic.AddUint64(&h.stats.eventsTotal, 1)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "accepted",
	})
}

// handleBatchEvents handles batch event submission
func (h *HTTPInput) handleBatchEvents(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&h.stats.requestsTotal, 1)

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, h.config.MaxBodySize)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		atomic.AddUint64(&h.stats.errorsTotal, 1)
		h.logger.Error().Err(err).Msg("Failed to read request body")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Parse as JSON array
	var events []map[string]interface{}
	if err := json.Unmarshal(body, &events); err != nil {
		atomic.AddUint64(&h.stats.errorsTotal, 1)
		h.logger.Error().Err(err).Msg("Failed to parse batch events")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Process each event
	accepted := 0
	for _, data := range events {
		event := &types.LogEvent{
			Timestamp: time.Now(),
			Message:   fmt.Sprintf("%v", data["message"]),
			Source:    h.name,
			Fields:    data,
		}

		// Add metadata
		event.Fields["remote_addr"] = r.RemoteAddr
		event.Fields["user_agent"] = r.UserAgent()
		event.Fields["input_type"] = "http"
		event.Fields["batch"] = true

		if h.SendEvent(event) {
			accepted++
		}
	}

	atomic.AddUint64(&h.stats.eventsTotal, uint64(accepted))

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "accepted",
		"accepted": accepted,
		"total":    len(events),
	})
}

// handleHealth handles health check endpoint
func (h *HTTPInput) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := h.Health()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// handleMetrics handles metrics endpoint
func (h *HTTPInput) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]interface{}{
		"requests_total":    atomic.LoadUint64(&h.stats.requestsTotal),
		"events_total":      atomic.LoadUint64(&h.stats.eventsTotal),
		"errors_total":      atomic.LoadUint64(&h.stats.errorsTotal),
		"auth_failures":     atomic.LoadUint64(&h.stats.authFailures),
		"rate_limit_hits":   atomic.LoadUint64(&h.stats.rateLimitHits),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// getRateLimiter gets or creates a rate limiter for a client
func (h *HTTPInput) getRateLimiter(remoteAddr string) *rate.Limiter {
	h.mu.RLock()
	limiter, exists := h.limiters[remoteAddr]
	h.mu.RUnlock()

	if !exists {
		// Create new rate limiter: RateLimit requests per second
		limiter = rate.NewLimiter(rate.Limit(h.config.RateLimit), h.config.RateLimit*2)
		h.mu.Lock()
		h.limiters[remoteAddr] = limiter
		h.mu.Unlock()

		// Clean up old limiters
		go h.cleanupLimiter(remoteAddr)
	}

	return limiter
}

// cleanupLimiter removes inactive rate limiters
func (h *HTTPInput) cleanupLimiter(remoteAddr string) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	select {
	case <-ticker.C:
		h.mu.Lock()
		delete(h.limiters, remoteAddr)
		h.mu.Unlock()
	case <-h.Context().Done():
		return
	}
}
