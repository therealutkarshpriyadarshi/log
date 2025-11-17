package input

import (
	"net"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/logging"
)

func TestSyslogInput(t *testing.T) {
	logger := logging.New(logging.Config{
		Level:  "info",
		Format: "json",
	})

	t.Run("NewSyslogInput", func(t *testing.T) {
		config := &SyslogConfig{
			Protocol:   "udp",
			Address:    "localhost:5140",
			Format:     "3164",
			BufferSize: 100,
		}

		input, err := NewSyslogInput("test-syslog", config, logger)
		if err != nil {
			t.Fatalf("failed to create syslog input: %v", err)
		}

		if input.Name() != "test-syslog" {
			t.Errorf("expected name 'test-syslog', got '%s'", input.Name())
		}

		if input.Type() != "syslog" {
			t.Errorf("expected type 'syslog', got '%s'", input.Type())
		}
	})

	t.Run("ReceiveUDP", func(t *testing.T) {
		config := &SyslogConfig{
			Protocol:   "udp",
			Address:    "localhost:5141",
			Format:     "3164",
			BufferSize: 100,
		}

		input, err := NewSyslogInput("test-syslog", config, logger)
		if err != nil {
			t.Fatalf("failed to create syslog input: %v", err)
		}

		if err := input.Start(); err != nil {
			t.Fatalf("failed to start syslog input: %v", err)
		}
		defer input.Stop()

		// Give the server time to start
		time.Sleep(100 * time.Millisecond)

		// Send a syslog message via UDP
		conn, err := net.Dial("udp", config.Address)
		if err != nil {
			t.Fatalf("failed to connect to syslog server: %v", err)
		}
		defer conn.Close()

		message := "<34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8\n"
		_, err = conn.Write([]byte(message))
		if err != nil {
			t.Fatalf("failed to send syslog message: %v", err)
		}

		// Check if event was received
		select {
		case event := <-input.Events():
			if event.Message == "" {
				t.Error("expected non-empty message")
			}
			if event.Source != "test-syslog" {
				t.Errorf("expected source 'test-syslog', got '%s'", event.Source)
			}
		case <-time.After(1 * time.Second):
			t.Error("timeout waiting for event")
		}
	})

	t.Run("ReceiveTCP", func(t *testing.T) {
		config := &SyslogConfig{
			Protocol:   "tcp",
			Address:    "localhost:5142",
			Format:     "3164",
			BufferSize: 100,
		}

		input, err := NewSyslogInput("test-syslog", config, logger)
		if err != nil {
			t.Fatalf("failed to create syslog input: %v", err)
		}

		if err := input.Start(); err != nil {
			t.Fatalf("failed to start syslog input: %v", err)
		}
		defer input.Stop()

		// Give the server time to start
		time.Sleep(100 * time.Millisecond)

		// Send a syslog message via TCP
		conn, err := net.Dial("tcp", config.Address)
		if err != nil {
			t.Fatalf("failed to connect to syslog server: %v", err)
		}
		defer conn.Close()

		message := "<34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8\n"
		_, err = conn.Write([]byte(message))
		if err != nil {
			t.Fatalf("failed to send syslog message: %v", err)
		}

		// Check if event was received
		select {
		case event := <-input.Events():
			if event.Message == "" {
				t.Error("expected non-empty message")
			}
		case <-time.After(1 * time.Second):
			t.Error("timeout waiting for event")
		}
	})

	t.Run("RateLimiting", func(t *testing.T) {
		config := &SyslogConfig{
			Protocol:   "udp",
			Address:    "localhost:5143",
			Format:     "3164",
			RateLimit:  2, // 2 messages per second
			BufferSize: 100,
		}

		input, err := NewSyslogInput("test-syslog", config, logger)
		if err != nil {
			t.Fatalf("failed to create syslog input: %v", err)
		}

		if err := input.Start(); err != nil {
			t.Fatalf("failed to start syslog input: %v", err)
		}
		defer input.Stop()

		// Give the server time to start
		time.Sleep(100 * time.Millisecond)

		// Send multiple messages rapidly
		conn, err := net.Dial("udp", config.Address)
		if err != nil {
			t.Fatalf("failed to connect to syslog server: %v", err)
		}
		defer conn.Close()

		message := "<34>Oct 11 22:14:15 mymachine test message\n"
		for i := 0; i < 10; i++ {
			conn.Write([]byte(message))
		}

		// Count received events
		receivedCount := 0
		timeout := time.After(2 * time.Second)

	loop:
		for {
			select {
			case <-input.Events():
				receivedCount++
			case <-timeout:
				break loop
			}
		}

		// Should have received fewer than 10 messages due to rate limiting
		if receivedCount >= 10 {
			t.Errorf("expected fewer than 10 events due to rate limiting, got %d", receivedCount)
		}
	})

	t.Run("Health", func(t *testing.T) {
		config := &SyslogConfig{
			Protocol:   "udp",
			Address:    "localhost:5144",
			BufferSize: 100,
		}

		input, err := NewSyslogInput("test-syslog", config, logger)
		if err != nil {
			t.Fatalf("failed to create syslog input: %v", err)
		}

		health := input.Health()

		if health.Status != HealthStatusHealthy {
			t.Errorf("expected status %s, got %s", HealthStatusHealthy, health.Status)
		}

		if health.Details["protocol"] != config.Protocol {
			t.Errorf("expected protocol '%s', got '%s'", config.Protocol, health.Details["protocol"])
		}
	})

	t.Run("ParseRFC3164", func(t *testing.T) {
		message := "<34>Oct 11 22:14:15 mymachine su: 'su root' failed"
		fields, err := parseRFC3164(message)

		if err != nil {
			t.Fatalf("failed to parse RFC 3164 message: %v", err)
		}

		if fields["format"] != "rfc3164" {
			t.Errorf("expected format 'rfc3164', got '%s'", fields["format"])
		}

		if fields["raw_message"] != message {
			t.Error("expected raw_message to match input")
		}
	})

	t.Run("ParseRFC5424", func(t *testing.T) {
		message := "<34>1 2003-10-11T22:14:15.003Z mymachine.example.com su - ID47 - 'su root' failed"
		fields, err := parseRFC5424(message)

		if err != nil {
			t.Fatalf("failed to parse RFC 5424 message: %v", err)
		}

		if fields["format"] != "rfc5424" {
			t.Errorf("expected format 'rfc5424', got '%s'", fields["format"])
		}

		if fields["raw_message"] != message {
			t.Error("expected raw_message to match input")
		}
	})
}
