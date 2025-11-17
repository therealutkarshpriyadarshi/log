package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// Transformer applies transformations to log events
type Transformer interface {
	Transform(event *types.LogEvent) (*types.LogEvent, error)
	Name() string
}

// TransformConfig holds transformation configuration
type TransformConfig struct {
	Type         string            `yaml:"type"`
	Fields       []string          `yaml:"fields,omitempty"`        // Fields to operate on
	IncludeFields []string         `yaml:"include_fields,omitempty"` // Fields to keep
	ExcludeFields []string         `yaml:"exclude_fields,omitempty"` // Fields to remove
	Rename       map[string]string `yaml:"rename,omitempty"`        // Field renaming map
	Add          map[string]string `yaml:"add,omitempty"`           // Fields to add
	Patterns     []string          `yaml:"patterns,omitempty"`      // KV extraction patterns
	FieldSplit   string            `yaml:"field_split,omitempty"`   // Field separator for KV
	ValueSplit   string            `yaml:"value_split,omitempty"`   // Value separator for KV
	Prefix       string            `yaml:"prefix,omitempty"`        // Prefix for extracted fields
}

// TransformPipeline is a series of transformers
type TransformPipeline struct {
	transformers []Transformer
}

// NewTransformPipeline creates a new transformation pipeline
func NewTransformPipeline(configs []TransformConfig) (*TransformPipeline, error) {
	transformers := make([]Transformer, 0, len(configs))

	for _, cfg := range configs {
		transformer, err := NewTransformer(&cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create transformer: %w", err)
		}
		transformers = append(transformers, transformer)
	}

	return &TransformPipeline{
		transformers: transformers,
	}, nil
}

// Transform applies all transformers in the pipeline
func (p *TransformPipeline) Transform(event *types.LogEvent) (*types.LogEvent, error) {
	var err error
	for _, transformer := range p.transformers {
		event, err = transformer.Transform(event)
		if err != nil {
			return event, err
		}
	}
	return event, nil
}

// NewTransformer creates a new transformer based on configuration
func NewTransformer(cfg *TransformConfig) (Transformer, error) {
	switch cfg.Type {
	case "filter":
		return NewFilterTransformer(cfg)
	case "rename":
		return NewRenameTransformer(cfg)
	case "add":
		return NewAddFieldsTransformer(cfg)
	case "kv":
		return NewKVExtractor(cfg)
	case "convert":
		return NewTypeConverter(cfg)
	default:
		return nil, fmt.Errorf("unknown transformer type: %s", cfg.Type)
	}
}

// FilterTransformer filters fields from events
type FilterTransformer struct {
	includeFields map[string]bool
	excludeFields map[string]bool
}

// NewFilterTransformer creates a new filter transformer
func NewFilterTransformer(cfg *TransformConfig) (*FilterTransformer, error) {
	include := make(map[string]bool)
	for _, field := range cfg.IncludeFields {
		include[field] = true
	}

	exclude := make(map[string]bool)
	for _, field := range cfg.ExcludeFields {
		exclude[field] = true
	}

	return &FilterTransformer{
		includeFields: include,
		excludeFields: exclude,
	}, nil
}

// Transform applies field filtering
func (t *FilterTransformer) Transform(event *types.LogEvent) (*types.LogEvent, error) {
	if event.Fields == nil {
		return event, nil
	}

	newFields := make(map[string]string)

	for key, value := range event.Fields {
		// If include list is specified, only keep those fields
		if len(t.includeFields) > 0 {
			if t.includeFields[key] {
				newFields[key] = value
			}
			continue
		}

		// Otherwise, exclude specified fields
		if !t.excludeFields[key] {
			newFields[key] = value
		}
	}

	event.Fields = newFields
	return event, nil
}

// Name returns the transformer name
func (t *FilterTransformer) Name() string {
	return "filter"
}

// RenameTransformer renames fields
type RenameTransformer struct {
	renameMap map[string]string
}

// NewRenameTransformer creates a new rename transformer
func NewRenameTransformer(cfg *TransformConfig) (*RenameTransformer, error) {
	return &RenameTransformer{
		renameMap: cfg.Rename,
	}, nil
}

// Transform renames fields
func (t *RenameTransformer) Transform(event *types.LogEvent) (*types.LogEvent, error) {
	if event.Fields == nil {
		return event, nil
	}

	for oldName, newName := range t.renameMap {
		if value, ok := event.Fields[oldName]; ok {
			event.Fields[newName] = value
			delete(event.Fields, oldName)
		}
	}

	return event, nil
}

// Name returns the transformer name
func (t *RenameTransformer) Name() string {
	return "rename"
}

// AddFieldsTransformer adds static fields to events
type AddFieldsTransformer struct {
	fields map[string]string
}

// NewAddFieldsTransformer creates a new add fields transformer
func NewAddFieldsTransformer(cfg *TransformConfig) (*AddFieldsTransformer, error) {
	return &AddFieldsTransformer{
		fields: cfg.Add,
	}, nil
}

// Transform adds fields to the event
func (t *AddFieldsTransformer) Transform(event *types.LogEvent) (*types.LogEvent, error) {
	if event.Fields == nil {
		event.Fields = make(map[string]string)
	}

	for key, value := range t.fields {
		event.Fields[key] = value
	}

	return event, nil
}

// Name returns the transformer name
func (t *AddFieldsTransformer) Name() string {
	return "add_fields"
}

// KVExtractor extracts key-value pairs from log messages
type KVExtractor struct {
	patterns    []*regexp.Regexp
	fieldSplit  string
	valueSplit  string
	prefix      string
}

// NewKVExtractor creates a new key-value extractor
func NewKVExtractor(cfg *TransformConfig) (*KVExtractor, error) {
	patterns := make([]*regexp.Regexp, 0, len(cfg.Patterns))
	for _, pattern := range cfg.Patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid kv pattern: %w", err)
		}
		patterns = append(patterns, re)
	}

	fieldSplit := cfg.FieldSplit
	if fieldSplit == "" {
		fieldSplit = " "
	}

	valueSplit := cfg.ValueSplit
	if valueSplit == "" {
		valueSplit = "="
	}

	return &KVExtractor{
		patterns:   patterns,
		fieldSplit: fieldSplit,
		valueSplit: valueSplit,
		prefix:     cfg.Prefix,
	}, nil
}

// Transform extracts key-value pairs from the message
func (t *KVExtractor) Transform(event *types.LogEvent) (*types.LogEvent, error) {
	if event.Message == "" {
		return event, nil
	}

	if event.Fields == nil {
		event.Fields = make(map[string]string)
	}

	// Try pattern-based extraction first
	for _, pattern := range t.patterns {
		matches := pattern.FindStringSubmatch(event.Message)
		if matches != nil {
			for i, name := range pattern.SubexpNames() {
				if i != 0 && name != "" && i < len(matches) {
					fieldName := name
					if t.prefix != "" {
						fieldName = t.prefix + name
					}
					event.Fields[fieldName] = matches[i]
				}
			}
			return event, nil
		}
	}

	// Fallback to simple key=value parsing
	pairs := strings.Split(event.Message, t.fieldSplit)
	for _, pair := range pairs {
		kv := strings.SplitN(pair, t.valueSplit, 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			if t.prefix != "" {
				key = t.prefix + key
			}
			event.Fields[key] = value
		}
	}

	return event, nil
}

// Name returns the transformer name
func (t *KVExtractor) Name() string {
	return "kv_extractor"
}

// TypeConverter converts field types
type TypeConverter struct {
	fields []string
}

// NewTypeConverter creates a new type converter
func NewTypeConverter(cfg *TransformConfig) (*TypeConverter, error) {
	return &TypeConverter{
		fields: cfg.Fields,
	}, nil
}

// Transform converts field types (currently supports string to appropriate type inference)
func (t *TypeConverter) Transform(event *types.LogEvent) (*types.LogEvent, error) {
	if event.Fields == nil {
		return event, nil
	}

	// If specific fields are specified, only convert those
	// Otherwise, attempt conversion on all fields
	fieldsToConvert := t.fields
	if len(fieldsToConvert) == 0 {
		for key := range event.Fields {
			fieldsToConvert = append(fieldsToConvert, key)
		}
	}

	for _, field := range fieldsToConvert {
		if value, ok := event.Fields[field]; ok {
			// Try to convert to number, bool, etc.
			// For now, we'll just normalize the string representation
			// In a real implementation, you might store typed values differently
			event.Fields[field] = normalizeValue(value)
		}
	}

	return event, nil
}

// normalizeValue attempts to normalize a value
func normalizeValue(value string) string {
	// Try parsing as bool
	if value == "true" || value == "false" {
		return value
	}

	// Try parsing as number
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return value
	}

	// Return as-is
	return value
}

// Name returns the transformer name
func (t *TypeConverter) Name() string {
	return "type_converter"
}
