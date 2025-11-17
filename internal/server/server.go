package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/therealutkarshpriyadarshi/log/internal/health"
	"github.com/therealutkarshpriyadarshi/log/internal/logging"
)

// Server provides HTTP endpoints for metrics and health checks
type Server struct {
	metricsServer *http.Server
	healthServer  *http.Server
	logger        *logging.Logger
}

// Config holds server configuration
type Config struct {
	MetricsAddress    string
	MetricsPath       string
	HealthAddress     string
	LivenessPath      string
	ReadinessPath     string
	MetricsRegistry   *prometheus.Registry
	HealthChecker     *health.Checker
	Logger            *logging.Logger
}

// New creates a new server
func New(cfg Config) *Server {
	s := &Server{
		logger: cfg.Logger,
	}

	// Create metrics server
	if cfg.MetricsAddress != "" && cfg.MetricsRegistry != nil {
		metricsPath := cfg.MetricsPath
		if metricsPath == "" {
			metricsPath = "/metrics"
		}

		mux := http.NewServeMux()
		mux.Handle(metricsPath, promhttp.HandlerFor(
			cfg.MetricsRegistry,
			promhttp.HandlerOpts{
				EnableOpenMetrics: true,
			},
		))

		s.metricsServer = &http.Server{
			Addr:         cfg.MetricsAddress,
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
	}

	// Create health server
	if cfg.HealthAddress != "" && cfg.HealthChecker != nil {
		livenessPath := cfg.LivenessPath
		if livenessPath == "" {
			livenessPath = "/health/live"
		}

		readinessPath := cfg.ReadinessPath
		if readinessPath == "" {
			readinessPath = "/health/ready"
		}

		mux := http.NewServeMux()
		mux.HandleFunc(livenessPath, cfg.HealthChecker.LivenessHandler())
		mux.HandleFunc(readinessPath, cfg.HealthChecker.ReadinessHandler())
		mux.HandleFunc("/health", cfg.HealthChecker.HTTPHandler())

		s.healthServer = &http.Server{
			Addr:         cfg.HealthAddress,
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
	}

	return s
}

// Start starts the servers
func (s *Server) Start() error {
	errCh := make(chan error, 2)

	// Start metrics server
	if s.metricsServer != nil {
		go func() {
			s.logger.Info().
				Str("address", s.metricsServer.Addr).
				Msg("Starting metrics server")

			if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("metrics server error: %w", err)
			}
		}()
	}

	// Start health server
	if s.healthServer != nil {
		go func() {
			s.logger.Info().
				Str("address", s.healthServer.Addr).
				Msg("Starting health server")

			if err := s.healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("health server error: %w", err)
			}
		}()
	}

	// Wait a bit to see if there are any immediate startup errors
	select {
	case err := <-errCh:
		return err
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// Stop gracefully shuts down the servers
func (s *Server) Stop(ctx context.Context) error {
	var err error

	if s.metricsServer != nil {
		s.logger.Info().Msg("Shutting down metrics server")
		if shutdownErr := s.metricsServer.Shutdown(ctx); shutdownErr != nil {
			s.logger.Error().Err(shutdownErr).Msg("Error shutting down metrics server")
			err = shutdownErr
		}
	}

	if s.healthServer != nil {
		s.logger.Info().Msg("Shutting down health server")
		if shutdownErr := s.healthServer.Shutdown(ctx); shutdownErr != nil {
			s.logger.Error().Err(shutdownErr).Msg("Error shutting down health server")
			if err == nil {
				err = shutdownErr
			}
		}
	}

	return err
}
