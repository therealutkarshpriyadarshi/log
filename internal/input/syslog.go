package input

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/logging"
	"github.com/therealutkarshpriyadarshi/log/pkg/types"
	"golang.org/x/time/rate"
)

// SyslogConfig holds configuration for syslog input
type SyslogConfig struct {
	// Protocol can be "tcp", "udp", or "both"
	Protocol string
	// Address to bind to (e.g., "0.0.0.0:514")
	Address string
	// RFC format: "3164" (BSD) or "5424" (new)
	Format string
	// TLS configuration for secure syslog
	TLSEnabled bool
	TLSCert    string
	TLSKey     string
	// Rate limiting per client (events per second)
	RateLimit int
	// Buffer size for events channel
	BufferSize int
}

// SyslogInput receives syslog messages over TCP/UDP
type SyslogInput struct {
	*BaseInput
	config   *SyslogConfig
	logger   *logging.Logger
	tcpLn    net.Listener
	udpConn  *net.UDPConn
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	wg       sync.WaitGroup
}

// NewSyslogInput creates a new syslog input
func NewSyslogInput(name string, config *SyslogConfig, logger *logging.Logger) (*SyslogInput, error) {
	if config.BufferSize == 0 {
		config.BufferSize = 10000
	}

	return &SyslogInput{
		BaseInput: NewBaseInput(name, "syslog", config.BufferSize),
		config:    config,
		logger:    logger.WithComponent("input-syslog"),
		limiters:  make(map[string]*rate.Limiter),
	}, nil
}

// Start starts the syslog receiver
func (s *SyslogInput) Start() error {
	protocol := s.config.Protocol
	if protocol == "" {
		protocol = "udp"
	}

	// Start TCP listener if needed
	if protocol == "tcp" || protocol == "both" {
		if err := s.startTCP(); err != nil {
			return fmt.Errorf("failed to start TCP listener: %w", err)
		}
	}

	// Start UDP listener if needed
	if protocol == "udp" || protocol == "both" {
		if err := s.startUDP(); err != nil {
			return fmt.Errorf("failed to start UDP listener: %w", err)
		}
	}

	s.logger.Info().
		Str("protocol", protocol).
		Str("address", s.config.Address).
		Msg("Syslog receiver started")

	return nil
}

// Stop stops the syslog receiver
func (s *SyslogInput) Stop() error {
	s.logger.Info().Msg("Stopping syslog receiver")

	s.Cancel()

	if s.tcpLn != nil {
		s.tcpLn.Close()
	}
	if s.udpConn != nil {
		s.udpConn.Close()
	}

	s.wg.Wait()
	s.Close()

	return nil
}

// Health returns the health status
func (s *SyslogInput) Health() Health {
	details := make(map[string]interface{})
	details["protocol"] = s.config.Protocol
	details["address"] = s.config.Address

	s.mu.RLock()
	details["active_clients"] = len(s.limiters)
	s.mu.RUnlock()

	return Health{
		Status:  HealthStatusHealthy,
		Message: "Syslog receiver is running",
		Details: details,
	}
}

// startTCP starts the TCP listener
func (s *SyslogInput) startTCP() error {
	var ln net.Listener
	var err error

	if s.config.TLSEnabled {
		cert, err := tls.LoadX509KeyPair(s.config.TLSCert, s.config.TLSKey)
		if err != nil {
			return fmt.Errorf("failed to load TLS certificate: %w", err)
		}

		config := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}

		ln, err = tls.Listen("tcp", s.config.Address, config)
		if err != nil {
			return fmt.Errorf("failed to start TLS listener: %w", err)
		}
		s.logger.Info().Msg("TLS enabled for TCP syslog")
	} else {
		ln, err = net.Listen("tcp", s.config.Address)
		if err != nil {
			return fmt.Errorf("failed to start TCP listener: %w", err)
		}
	}

	s.tcpLn = ln

	s.wg.Add(1)
	go s.acceptTCP()

	return nil
}

// acceptTCP accepts TCP connections
func (s *SyslogInput) acceptTCP() {
	defer s.wg.Done()

	for {
		conn, err := s.tcpLn.Accept()
		if err != nil {
			select {
			case <-s.Context().Done():
				return
			default:
				s.logger.Error().Err(err).Msg("Failed to accept TCP connection")
				continue
			}
		}

		s.wg.Add(1)
		go s.handleTCP(conn)
	}
}

// handleTCP handles a TCP connection
func (s *SyslogInput) handleTCP(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	clientAddr := conn.RemoteAddr().String()
	s.logger.Debug().Str("client", clientAddr).Msg("New TCP connection")

	// Get or create rate limiter for this client
	limiter := s.getRateLimiter(clientAddr)

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		select {
		case <-s.Context().Done():
			return
		default:
		}

		// Apply rate limiting
		if limiter != nil && !limiter.Allow() {
			s.logger.Warn().Str("client", clientAddr).Msg("Rate limit exceeded")
			continue
		}

		line := scanner.Text()
		event := s.parseMessage(line, clientAddr)
		if event != nil {
			s.SendEvent(event)
		}
	}

	if err := scanner.Err(); err != nil {
		s.logger.Error().Err(err).Str("client", clientAddr).Msg("Error reading from TCP connection")
	}
}

// startUDP starts the UDP listener
func (s *SyslogInput) startUDP() error {
	addr, err := net.ResolveUDPAddr("udp", s.config.Address)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to start UDP listener: %w", err)
	}

	s.udpConn = conn

	s.wg.Add(1)
	go s.receiveUDP()

	return nil
}

// receiveUDP receives UDP messages
func (s *SyslogInput) receiveUDP() {
	defer s.wg.Done()

	buf := make([]byte, 65536) // Max UDP packet size

	for {
		select {
		case <-s.Context().Done():
			return
		default:
		}

		n, addr, err := s.udpConn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.Context().Done():
				return
			default:
				s.logger.Error().Err(err).Msg("Error reading from UDP")
				continue
			}
		}

		clientAddr := addr.String()

		// Apply rate limiting
		limiter := s.getRateLimiter(clientAddr)
		if limiter != nil && !limiter.Allow() {
			s.logger.Warn().Str("client", clientAddr).Msg("Rate limit exceeded")
			continue
		}

		message := string(buf[:n])
		event := s.parseMessage(message, clientAddr)
		if event != nil {
			s.SendEvent(event)
		}
	}
}

// getRateLimiter gets or creates a rate limiter for a client
func (s *SyslogInput) getRateLimiter(clientAddr string) *rate.Limiter {
	if s.config.RateLimit <= 0 {
		return nil
	}

	s.mu.RLock()
	limiter, exists := s.limiters[clientAddr]
	s.mu.RUnlock()

	if !exists {
		// Create new rate limiter: RateLimit events per second, burst of 2x
		limiter = rate.NewLimiter(rate.Limit(s.config.RateLimit), s.config.RateLimit*2)
		s.mu.Lock()
		s.limiters[clientAddr] = limiter
		s.mu.Unlock()

		// Clean up old limiters periodically
		go s.cleanupLimiter(clientAddr)
	}

	return limiter
}

// cleanupLimiter removes inactive rate limiters
func (s *SyslogInput) cleanupLimiter(clientAddr string) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	select {
	case <-ticker.C:
		s.mu.Lock()
		delete(s.limiters, clientAddr)
		s.mu.Unlock()
	case <-s.Context().Done():
		return
	}
}

// parseMessage parses a syslog message based on configured format
func (s *SyslogInput) parseMessage(message, source string) *types.LogEvent {
	format := s.config.Format
	if format == "" {
		format = "3164" // Default to BSD format
	}

	var fields map[string]interface{}
	var err error

	switch format {
	case "3164":
		fields, err = parseRFC3164(message)
	case "5424":
		fields, err = parseRFC5424(message)
	default:
		s.logger.Warn().Str("format", format).Msg("Unknown syslog format")
		fields = make(map[string]interface{})
	}

	if err != nil {
		s.logger.Debug().Err(err).Str("message", message).Msg("Failed to parse syslog message")
		// Still create event with raw message
		fields = make(map[string]interface{})
	}

	// Add metadata
	fields["source_ip"] = source
	fields["input_type"] = "syslog"

	return &types.LogEvent{
		Timestamp: time.Now(),
		Message:   message,
		Source:    s.name,
		Fields:    fields,
		Raw:       message,
	}
}

// parseRFC3164 parses BSD syslog format (RFC 3164)
// Format: <PRI>TIMESTAMP HOSTNAME TAG: MESSAGE
func parseRFC3164(message string) (map[string]interface{}, error) {
	fields := make(map[string]interface{})

	// Simple parsing - can be enhanced with proper syslog parser
	// For now, just extract the basic structure
	fields["format"] = "rfc3164"
	fields["raw_message"] = message

	// TODO: Implement proper RFC 3164 parsing with priority, timestamp, hostname, tag
	// This is a simplified version for MVP

	return fields, nil
}

// parseRFC5424 parses new syslog format (RFC 5424)
// Format: <PRI>VERSION TIMESTAMP HOSTNAME APP-NAME PROCID MSGID STRUCTURED-DATA MSG
func parseRFC5424(message string) (map[string]interface{}, error) {
	fields := make(map[string]interface{})

	// Simple parsing - can be enhanced with proper syslog parser
	fields["format"] = "rfc5424"
	fields["raw_message"] = message

	// TODO: Implement proper RFC 5424 parsing
	// This is a simplified version for MVP

	return fields, nil
}
