package metrics

import (
	"fmt"
	"regexp"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricType represents the type of metric to extract
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
)

// ExtractionRule defines how to extract a metric from log fields
type ExtractionRule struct {
	Name        string            // Metric name
	Type        MetricType        // Metric type
	Field       string            // Field to extract value from
	Pattern     string            // Regex pattern to extract value (if field is a string)
	Labels      map[string]string // Static labels
	LabelFields map[string]string // Dynamic labels from fields (metric_label: field_name)
	Help        string            // Metric description
	Buckets     []float64         // Histogram buckets (optional)
}

// Extractor extracts metrics from log events
type Extractor struct {
	mu      sync.RWMutex
	rules   []ExtractionRule
	metrics map[string]interface{} // Stores prometheus metrics
	regex   map[string]*regexp.Regexp
}

// NewExtractor creates a new metrics extractor
func NewExtractor(rules []ExtractionRule) (*Extractor, error) {
	e := &Extractor{
		rules:   rules,
		metrics: make(map[string]interface{}),
		regex:   make(map[string]*regexp.Regexp),
	}

	// Compile regex patterns and create metrics
	for _, rule := range rules {
		if rule.Pattern != "" {
			re, err := regexp.Compile(rule.Pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid pattern for metric %s: %w", rule.Name, err)
			}
			e.regex[rule.Name] = re
		}

		// Create prometheus metric based on type
		if err := e.createMetric(rule); err != nil {
			return nil, err
		}
	}

	return e, nil
}

func (e *Extractor) createMetric(rule ExtractionRule) error {
	// Determine label names
	labelNames := make([]string, 0, len(rule.LabelFields))
	for labelName := range rule.LabelFields {
		labelNames = append(labelNames, labelName)
	}

	metricName := fmt.Sprintf("%s_extracted_%s", namespace, rule.Name)

	switch rule.Type {
	case MetricTypeCounter:
		if len(labelNames) > 0 {
			e.metrics[rule.Name] = prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Name: metricName,
					Help: rule.Help,
				},
				labelNames,
			)
			prometheus.MustRegister(e.metrics[rule.Name].(*prometheus.CounterVec))
		} else {
			e.metrics[rule.Name] = prometheus.NewCounter(
				prometheus.CounterOpts{
					Name: metricName,
					Help: rule.Help,
				},
			)
			prometheus.MustRegister(e.metrics[rule.Name].(prometheus.Counter))
		}

	case MetricTypeGauge:
		if len(labelNames) > 0 {
			e.metrics[rule.Name] = prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Name: metricName,
					Help: rule.Help,
				},
				labelNames,
			)
			prometheus.MustRegister(e.metrics[rule.Name].(*prometheus.GaugeVec))
		} else {
			e.metrics[rule.Name] = prometheus.NewGauge(
				prometheus.GaugeOpts{
					Name: metricName,
					Help: rule.Help,
				},
			)
			prometheus.MustRegister(e.metrics[rule.Name].(prometheus.Gauge))
		}

	case MetricTypeHistogram:
		buckets := rule.Buckets
		if buckets == nil {
			buckets = prometheus.DefBuckets
		}

		if len(labelNames) > 0 {
			e.metrics[rule.Name] = prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    metricName,
					Help:    rule.Help,
					Buckets: buckets,
				},
				labelNames,
			)
			prometheus.MustRegister(e.metrics[rule.Name].(*prometheus.HistogramVec))
		} else {
			e.metrics[rule.Name] = prometheus.NewHistogram(
				prometheus.HistogramOpts{
					Name:    metricName,
					Help:    rule.Help,
					Buckets: buckets,
				},
			)
			prometheus.MustRegister(e.metrics[rule.Name].(prometheus.Histogram))
		}

	default:
		return fmt.Errorf("unsupported metric type: %s", rule.Type)
	}

	return nil
}

// Extract processes a log event and extracts metrics
func (e *Extractor) Extract(fields map[string]interface{}) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, rule := range e.rules {
		value, labels, err := e.extractValue(rule, fields)
		if err != nil {
			continue // Skip if extraction fails
		}

		if err := e.recordMetric(rule, value, labels); err != nil {
			return err
		}
	}

	return nil
}

func (e *Extractor) extractValue(rule ExtractionRule, fields map[string]interface{}) (float64, prometheus.Labels, error) {
	// Extract value from field
	fieldValue, exists := fields[rule.Field]
	if !exists {
		return 0, nil, fmt.Errorf("field %s not found", rule.Field)
	}

	var value float64
	var err error

	// Handle different field types
	switch v := fieldValue.(type) {
	case float64:
		value = v
	case float32:
		value = float64(v)
	case int:
		value = float64(v)
	case int64:
		value = float64(v)
	case int32:
		value = float64(v)
	case string:
		// Try to parse as number
		value, err = strconv.ParseFloat(v, 64)
		if err != nil {
			// If pattern is specified, try to extract using regex
			if rule.Pattern != "" {
				re := e.regex[rule.Name]
				matches := re.FindStringSubmatch(v)
				if len(matches) > 1 {
					value, err = strconv.ParseFloat(matches[1], 64)
					if err != nil {
						return 0, nil, fmt.Errorf("failed to parse extracted value: %w", err)
					}
				} else {
					return 0, nil, fmt.Errorf("pattern did not match")
				}
			} else {
				return 0, nil, fmt.Errorf("failed to parse field as number: %w", err)
			}
		}
	default:
		return 0, nil, fmt.Errorf("unsupported field type: %T", v)
	}

	// Extract labels
	labels := prometheus.Labels{}
	for labelName, fieldName := range rule.LabelFields {
		if labelValue, exists := fields[fieldName]; exists {
			labels[labelName] = fmt.Sprintf("%v", labelValue)
		}
	}

	// Add static labels
	for k, v := range rule.Labels {
		labels[k] = v
	}

	return value, labels, nil
}

func (e *Extractor) recordMetric(rule ExtractionRule, value float64, labels prometheus.Labels) error {
	metric, exists := e.metrics[rule.Name]
	if !exists {
		return fmt.Errorf("metric %s not found", rule.Name)
	}

	hasLabels := len(labels) > 0

	switch rule.Type {
	case MetricTypeCounter:
		if hasLabels {
			metric.(*prometheus.CounterVec).With(labels).Add(value)
		} else {
			metric.(prometheus.Counter).Add(value)
		}

	case MetricTypeGauge:
		if hasLabels {
			metric.(*prometheus.GaugeVec).With(labels).Set(value)
		} else {
			metric.(prometheus.Gauge).Set(value)
		}

	case MetricTypeHistogram:
		if hasLabels {
			metric.(*prometheus.HistogramVec).With(labels).Observe(value)
		} else {
			metric.(prometheus.Histogram).Observe(value)
		}
	}

	return nil
}

// Example extraction rules

// ResponseTimeRule extracts HTTP response time from logs
func ResponseTimeRule() ExtractionRule {
	return ExtractionRule{
		Name:   "http_response_time",
		Type:   MetricTypeHistogram,
		Field:  "response_time_ms",
		Help:   "HTTP response time in milliseconds",
		Buckets: []float64{10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
		LabelFields: map[string]string{
			"method": "http_method",
			"status": "http_status",
			"path":   "http_path",
		},
	}
}

// RequestCountRule counts HTTP requests
func RequestCountRule() ExtractionRule {
	return ExtractionRule{
		Name: "http_requests",
		Type: MetricTypeCounter,
		Field: "request_count",
		Help: "Total number of HTTP requests",
		LabelFields: map[string]string{
			"method": "http_method",
			"status": "http_status",
		},
	}
}

// ErrorCountRule counts errors in logs
func ErrorCountRule() ExtractionRule {
	return ExtractionRule{
		Name:  "errors",
		Type:  MetricTypeCounter,
		Field: "error_count",
		Help:  "Total number of errors",
		LabelFields: map[string]string{
			"level":   "level",
			"service": "service",
		},
	}
}
