package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretManager_GetSecret(t *testing.T) {
	sm := NewSecretManager()

	// Test environment variable
	os.Setenv("TEST_SECRET", "test-value")
	defer os.Unsetenv("TEST_SECRET")

	secret, err := sm.GetSecret("env:TEST_SECRET")
	if err != nil {
		t.Fatalf("Failed to get env secret: %v", err)
	}
	if secret != "test-value" {
		t.Errorf("Expected 'test-value', got %s", secret)
	}

	// Test plain text (for development)
	secret, err = sm.GetSecret("plain-secret")
	if err != nil {
		t.Fatalf("Failed to get plain secret: %v", err)
	}
	if secret != "plain-secret" {
		t.Errorf("Expected 'plain-secret', got %s", secret)
	}

	// Test file secret
	tmpDir := t.TempDir()
	secretFile := filepath.Join(tmpDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("file-secret\n"), 0600); err != nil {
		t.Fatalf("Failed to create secret file: %v", err)
	}

	secret, err = sm.GetSecret("file:" + secretFile)
	if err != nil {
		t.Fatalf("Failed to get file secret: %v", err)
	}
	if secret != "file-secret" {
		t.Errorf("Expected 'file-secret', got %s", secret)
	}

	// Test missing environment variable
	_, err = sm.GetSecret("env:NONEXISTENT_VAR")
	if err == nil {
		t.Error("Expected error for missing env var, got nil")
	}
}

func TestValidator_ValidateIP(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		ip    string
		valid bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"255.255.255.255", true},
		{"0.0.0.0", true},
		{"256.1.1.1", false},
		{"192.168.1", false},
		{"192.168.1.1.1", false},
		{"not-an-ip", false},
		{"", false},
	}

	for _, tt := range tests {
		result := v.ValidateIP(tt.ip)
		if result != tt.valid {
			t.Errorf("ValidateIP(%s) = %v, want %v", tt.ip, result, tt.valid)
		}
	}
}

func TestValidator_ValidateHostPort(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		hostPort string
		valid    bool
	}{
		{"localhost:8080", true},
		{"example.com:443", true},
		{"192.168.1.1:9000", true},
		{"0.0.0.0:80", true},
		{"localhost", false},
		{"localhost:0", false},
		{"localhost:65536", false},
		{"localhost:-1", false},
		{"localhost:abc", false},
		{"", false},
	}

	for _, tt := range tests {
		result := v.ValidateHostPort(tt.hostPort)
		if result != tt.valid {
			t.Errorf("ValidateHostPort(%s) = %v, want %v", tt.hostPort, result, tt.valid)
		}
	}
}

func TestValidator_ValidatePath(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		path  string
		valid bool
	}{
		{"/var/log/app.log", true},
		{"/tmp/test.txt", true},
		{"./local.log", true},
		{"../etc/passwd", false},
		{"/var/log/../../../etc/passwd", false},
		{"/var/log/\x00test", false},
		{"", true},
	}

	for _, tt := range tests {
		result := v.ValidatePath(tt.path)
		if result != tt.valid {
			t.Errorf("ValidatePath(%s) = %v, want %v", tt.path, result, tt.valid)
		}
	}
}

func TestValidator_SanitizeInput(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		input    string
		expected string
	}{
		{"  test  ", "test"},
		{"test\x00input", "testinput"},
		{"  test\x00input  ", "testinput"},
		{"normal-input", "normal-input"},
		{"", ""},
	}

	for _, tt := range tests {
		result := v.SanitizeInput(tt.input)
		if result != tt.expected {
			t.Errorf("SanitizeInput(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestValidator_ValidateJSONField(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		field string
		valid bool
	}{
		{"timestamp", true},
		{"log_level", true},
		{"user-id", true},
		{"field.name", true},
		{"field123", true},
		{"field!name", false},
		{"field@name", false},
		{"field name", false},
		{"field/name", false},
		{"", false},
	}

	for _, tt := range tests {
		result := v.ValidateJSONField(tt.field)
		if result != tt.valid {
			t.Errorf("ValidateJSONField(%s) = %v, want %v", tt.field, result, tt.valid)
		}
	}
}

func TestSecurityAuditor_ContainsSensitiveData(t *testing.T) {
	sa := NewSecurityAuditor()

	tests := []struct {
		field     string
		sensitive bool
	}{
		{"password", true},
		{"user_password", true},
		{"PASSWORD", true},
		{"api_key", true},
		{"secret_token", true},
		{"username", false},
		{"email", false},
		{"id", false},
		{"timestamp", false},
	}

	for _, tt := range tests {
		result := sa.ContainsSensitiveData(tt.field)
		if result != tt.sensitive {
			t.Errorf("ContainsSensitiveData(%s) = %v, want %v", tt.field, result, tt.sensitive)
		}
	}
}

func TestSecurityAuditor_RedactSensitiveFields(t *testing.T) {
	sa := NewSecurityAuditor()

	fields := map[string]interface{}{
		"username": "john",
		"password": "secret123",
		"email":    "john@example.com",
		"api_key":  "key-123",
		"level":    "info",
	}

	redacted := sa.RedactSensitiveFields(fields)

	// Check sensitive fields are redacted
	if redacted["password"] != "***REDACTED***" {
		t.Errorf("password not redacted: %v", redacted["password"])
	}
	if redacted["api_key"] != "***REDACTED***" {
		t.Errorf("api_key not redacted: %v", redacted["api_key"])
	}

	// Check non-sensitive fields are preserved
	if redacted["username"] != "john" {
		t.Errorf("username should not be redacted: %v", redacted["username"])
	}
	if redacted["email"] != "john@example.com" {
		t.Errorf("email should not be redacted: %v", redacted["email"])
	}
	if redacted["level"] != "info" {
		t.Errorf("level should not be redacted: %v", redacted["level"])
	}
}

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(100)
	if rl == nil {
		t.Error("NewRateLimiter returned nil")
	}
	if rl.maxRequestsPerSecond != 100 {
		t.Errorf("Expected maxRequestsPerSecond=100, got %d", rl.maxRequestsPerSecond)
	}
}
