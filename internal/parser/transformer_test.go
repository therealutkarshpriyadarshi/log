package parser

import (
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

func TestFilterTransformer(t *testing.T) {
	tests := []struct {
		name       string
		config     *TransformConfig
		event      *types.LogEvent
		wantFields map[string]string
	}{
		{
			name: "include fields",
			config: &TransformConfig{
				Type:          "filter",
				IncludeFields: []string{"user", "request_id"},
			},
			event: &types.LogEvent{
				Fields: map[string]string{
					"user":       "alice",
					"request_id": "123",
					"password":   "secret",
					"token":      "xyz",
				},
			},
			wantFields: map[string]string{
				"user":       "alice",
				"request_id": "123",
			},
		},
		{
			name: "exclude fields",
			config: &TransformConfig{
				Type:          "filter",
				ExcludeFields: []string{"password", "token"},
			},
			event: &types.LogEvent{
				Fields: map[string]string{
					"user":     "alice",
					"password": "secret",
					"token":    "xyz",
				},
			},
			wantFields: map[string]string{
				"user": "alice",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := NewFilterTransformer(tt.config)
			if err != nil {
				t.Fatalf("Failed to create transformer: %v", err)
			}

			result, err := transformer.Transform(tt.event)
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}

			if len(result.Fields) != len(tt.wantFields) {
				t.Errorf("Fields count = %d, want %d", len(result.Fields), len(tt.wantFields))
			}

			for key, wantValue := range tt.wantFields {
				if gotValue, ok := result.Fields[key]; !ok {
					t.Errorf("Field %s not found", key)
				} else if gotValue != wantValue {
					t.Errorf("Field %s = %v, want %v", key, gotValue, wantValue)
				}
			}
		})
	}
}

func TestRenameTransformer(t *testing.T) {
	config := &TransformConfig{
		Type: "rename",
		Rename: map[string]string{
			"old_name": "new_name",
			"user":     "username",
		},
	}

	event := &types.LogEvent{
		Fields: map[string]string{
			"old_name": "value1",
			"user":     "alice",
			"other":    "value2",
		},
	}

	transformer, err := NewRenameTransformer(config)
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	result, err := transformer.Transform(event)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	// Check renamed fields exist
	if _, ok := result.Fields["new_name"]; !ok {
		t.Error("Field 'new_name' not found after rename")
	}

	if _, ok := result.Fields["username"]; !ok {
		t.Error("Field 'username' not found after rename")
	}

	// Check old fields are removed
	if _, ok := result.Fields["old_name"]; ok {
		t.Error("Field 'old_name' should be removed after rename")
	}

	if _, ok := result.Fields["user"]; ok {
		t.Error("Field 'user' should be removed after rename")
	}

	// Check other fields are preserved
	if result.Fields["other"] != "value2" {
		t.Error("Other fields should be preserved")
	}
}

func TestAddFieldsTransformer(t *testing.T) {
	config := &TransformConfig{
		Type: "add",
		Add: map[string]string{
			"environment": "production",
			"app":         "myapp",
			"version":     "1.0.0",
		},
	}

	event := &types.LogEvent{
		Fields: map[string]string{
			"user": "alice",
		},
	}

	transformer, err := NewAddFieldsTransformer(config)
	if err != nil {
		t.Fatalf("Failed to create transformer: %v", err)
	}

	result, err := transformer.Transform(event)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	// Check added fields
	expectedFields := map[string]string{
		"user":        "alice",
		"environment": "production",
		"app":         "myapp",
		"version":     "1.0.0",
	}

	for key, wantValue := range expectedFields {
		if gotValue, ok := result.Fields[key]; !ok {
			t.Errorf("Field %s not found", key)
		} else if gotValue != wantValue {
			t.Errorf("Field %s = %v, want %v", key, gotValue, wantValue)
		}
	}
}

func TestKVExtractor(t *testing.T) {
	tests := []struct {
		name       string
		config     *TransformConfig
		event      *types.LogEvent
		wantFields map[string]string
	}{
		{
			name: "simple key-value extraction",
			config: &TransformConfig{
				Type:       "kv",
				FieldSplit: " ",
				ValueSplit: "=",
			},
			event: &types.LogEvent{
				Message: "user=alice status=success code=200",
			},
			wantFields: map[string]string{
				"user":   "alice",
				"status": "success",
				"code":   "200",
			},
		},
		{
			name: "key-value with prefix",
			config: &TransformConfig{
				Type:       "kv",
				FieldSplit: " ",
				ValueSplit: "=",
				Prefix:     "kv_",
			},
			event: &types.LogEvent{
				Message: "user=alice status=success",
			},
			wantFields: map[string]string{
				"kv_user":   "alice",
				"kv_status": "success",
			},
		},
		{
			name: "key-value with custom separators",
			config: &TransformConfig{
				Type:       "kv",
				FieldSplit: ",",
				ValueSplit: ":",
			},
			event: &types.LogEvent{
				Message: "user:bob,role:admin,active:true",
			},
			wantFields: map[string]string{
				"user":   "bob",
				"role":   "admin",
				"active": "true",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer, err := NewKVExtractor(tt.config)
			if err != nil {
				t.Fatalf("Failed to create transformer: %v", err)
			}

			if tt.event.Fields == nil {
				tt.event.Fields = make(map[string]string)
			}

			result, err := transformer.Transform(tt.event)
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}

			for key, wantValue := range tt.wantFields {
				if gotValue, ok := result.Fields[key]; !ok {
					t.Errorf("Field %s not found", key)
				} else if gotValue != wantValue {
					t.Errorf("Field %s = %v, want %v", key, gotValue, wantValue)
				}
			}
		})
	}
}

func TestTransformPipeline(t *testing.T) {
	configs := []TransformConfig{
		{
			Type:       "kv",
			FieldSplit: " ",
			ValueSplit: "=",
		},
		{
			Type: "add",
			Add: map[string]string{
				"environment": "production",
			},
		},
		{
			Type: "rename",
			Rename: map[string]string{
				"user": "username",
			},
		},
		{
			Type:          "filter",
			ExcludeFields: []string{"password"},
		},
	}

	pipeline, err := NewTransformPipeline(configs)
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	event := &types.LogEvent{
		Timestamp: time.Now(),
		Message:   "user=alice password=secret status=active",
		Fields:    make(map[string]string),
	}

	result, err := pipeline.Transform(event)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	// Check KV extraction
	if _, ok := result.Fields["username"]; !ok {
		t.Error("Expected 'username' field after KV extraction and rename")
	}

	// Check password is filtered out
	if _, ok := result.Fields["password"]; ok {
		t.Error("Password field should be filtered out")
	}

	// Check added field
	if result.Fields["environment"] != "production" {
		t.Error("Expected environment field to be added")
	}

	// Check status preserved
	if result.Fields["status"] != "active" {
		t.Error("Status field should be preserved")
	}
}

func TestNewTransformer_UnknownType(t *testing.T) {
	config := &TransformConfig{
		Type: "unknown",
	}

	_, err := NewTransformer(config)
	if err == nil {
		t.Error("Expected error for unknown transformer type")
	}
}
