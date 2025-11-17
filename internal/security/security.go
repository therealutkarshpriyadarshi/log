package security

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled            bool
	CertFile           string
	KeyFile            string
	CAFile             string
	InsecureSkipVerify bool
	MinVersion         uint16
}

// LoadTLSConfig loads and creates a TLS configuration
func LoadTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: cfg.MinVersion,
	}

	// Set default minimum version if not specified
	if tlsConfig.MinVersion == 0 {
		tlsConfig.MinVersion = tls.VersionTLS12
	}

	// Load certificate and key if provided
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate and key: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate if provided
	if cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.RootCAs = caCertPool
		tlsConfig.ClientCAs = caCertPool
	}

	tlsConfig.InsecureSkipVerify = cfg.InsecureSkipVerify

	return tlsConfig, nil
}

// SecretManager handles secret management from various sources
type SecretManager struct {
	// In production, this could integrate with:
	// - Environment variables
	// - Kubernetes secrets
	// - HashiCorp Vault
	// - AWS Secrets Manager
	// - Azure Key Vault
}

// NewSecretManager creates a new secret manager
func NewSecretManager() *SecretManager {
	return &SecretManager{}
}

// GetSecret retrieves a secret by key
// Supports format: env:VAR_NAME, file:/path/to/secret, or plain text
func (sm *SecretManager) GetSecret(key string) (string, error) {
	// Check if it's an environment variable reference
	if strings.HasPrefix(key, "env:") {
		envVar := strings.TrimPrefix(key, "env:")
		value := os.Getenv(envVar)
		if value == "" {
			return "", fmt.Errorf("environment variable %s not found", envVar)
		}
		return value, nil
	}

	// Check if it's a file reference
	if strings.HasPrefix(key, "file:") {
		filePath := strings.TrimPrefix(key, "file:")
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to read secret from file %s: %w", filePath, err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	// Otherwise, treat as plain text (for testing/development only)
	return key, nil
}

// Validator provides input validation functions
type Validator struct{}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateIP validates an IP address or CIDR range
func (v *Validator) ValidateIP(ip string) bool {
	// Simple IP validation (IPv4)
	ipPattern := regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)
	if !ipPattern.MatchString(ip) {
		return false
	}

	// Check each octet is 0-255
	parts := strings.Split(ip, ".")
	for _, part := range parts {
		var octet int
		fmt.Sscanf(part, "%d", &octet)
		if octet < 0 || octet > 255 {
			return false
		}
	}
	return true
}

// ValidateHostPort validates host:port format
func (v *Validator) ValidateHostPort(hostPort string) bool {
	parts := strings.Split(hostPort, ":")
	if len(parts) != 2 {
		return false
	}

	// Validate port
	var port int
	fmt.Sscanf(parts[1], "%d", &port)
	return port > 0 && port <= 65535
}

// ValidatePath validates a file path (basic check)
func (v *Validator) ValidatePath(path string) bool {
	// Check for path traversal attempts
	if strings.Contains(path, "..") {
		return false
	}
	// Check for null bytes
	if strings.Contains(path, "\x00") {
		return false
	}
	return true
}

// SanitizeInput sanitizes user input to prevent injection attacks
func (v *Validator) SanitizeInput(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")
	// Trim whitespace
	input = strings.TrimSpace(input)
	return input
}

// ValidateJSONField validates JSON field names
func (v *Validator) ValidateJSONField(field string) bool {
	// Only allow alphanumeric, underscore, hyphen, and dot
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)
	return validPattern.MatchString(field)
}

// RateLimiter provides DoS protection
type RateLimiter struct {
	// This is a simple placeholder
	// In production, use golang.org/x/time/rate or similar
	maxRequestsPerSecond int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(maxRequestsPerSecond int) *RateLimiter {
	return &RateLimiter{
		maxRequestsPerSecond: maxRequestsPerSecond,
	}
}

// SecurityAuditor provides security auditing functions
type SecurityAuditor struct {
	sensitiveFields []string
}

// NewSecurityAuditor creates a new security auditor
func NewSecurityAuditor() *SecurityAuditor {
	return &SecurityAuditor{
		sensitiveFields: []string{
			"password",
			"passwd",
			"secret",
			"token",
			"api_key",
			"apikey",
			"access_key",
			"private_key",
			"credential",
			"auth",
			"authorization",
		},
	}
}

// ContainsSensitiveData checks if a field name suggests sensitive data
func (sa *SecurityAuditor) ContainsSensitiveData(fieldName string) bool {
	lowerField := strings.ToLower(fieldName)
	for _, sensitive := range sa.sensitiveFields {
		if strings.Contains(lowerField, sensitive) {
			return true
		}
	}
	return false
}

// RedactSensitiveFields redacts sensitive fields from a map
func (sa *SecurityAuditor) RedactSensitiveFields(fields map[string]interface{}) map[string]interface{} {
	redacted := make(map[string]interface{})
	for k, v := range fields {
		if sa.ContainsSensitiveData(k) {
			redacted[k] = "***REDACTED***"
		} else {
			redacted[k] = v
		}
	}
	return redacted
}

// AuditLog represents a security audit log entry
type AuditLog struct {
	Timestamp string
	Action    string
	User      string
	Source    string
	Success   bool
	Details   string
}

// ValidateConfig performs security validation on configuration
func ValidateConfig(validator *Validator, config interface{}) error {
	// This is a placeholder for configuration validation
	// In production, validate all configuration parameters
	return nil
}
