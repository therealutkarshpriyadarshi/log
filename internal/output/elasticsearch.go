package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// ElasticsearchConfig contains Elasticsearch-specific configuration
type ElasticsearchConfig struct {
	BaseConfig `yaml:",inline"`

	// Addresses is the list of Elasticsearch node URLs
	Addresses []string `yaml:"addresses"`

	// Index is the default index name or pattern (supports time-based patterns)
	Index string `yaml:"index"`

	// IndexRotation specifies how often to rotate indices (daily, weekly, monthly, none)
	IndexRotation string `yaml:"index_rotation,omitempty"`

	// IndexTimestampField is the field to use for index timestamp
	IndexTimestampField string `yaml:"index_timestamp_field,omitempty"`

	// Pipeline is the ingest pipeline to use
	Pipeline string `yaml:"pipeline,omitempty"`

	// Username for authentication
	Username string `yaml:"username,omitempty"`

	// Password for authentication
	Password string `yaml:"password,omitempty"`

	// CloudID for Elastic Cloud
	CloudID string `yaml:"cloud_id,omitempty"`

	// APIKey for authentication
	APIKey string `yaml:"api_key,omitempty"`

	// EnableTLS enables TLS for connections
	EnableTLS bool `yaml:"enable_tls,omitempty"`

	// BulkWorkers is the number of concurrent bulk workers
	BulkWorkers int `yaml:"bulk_workers,omitempty"`

	// MaxRetries for failed requests
	MaxRetries int `yaml:"max_retries,omitempty"`
}

// DefaultElasticsearchConfig returns default Elasticsearch configuration
func DefaultElasticsearchConfig() ElasticsearchConfig {
	return ElasticsearchConfig{
		BaseConfig:          DefaultBaseConfig(),
		Addresses:           []string{"http://localhost:9200"},
		Index:               "logs",
		IndexRotation:       "daily",
		IndexTimestampField: "timestamp",
		BulkWorkers:         1,
		MaxRetries:          3,
	}
}

// ElasticsearchOutput sends events to Elasticsearch
type ElasticsearchOutput struct {
	config  ElasticsearchConfig
	client  *elasticsearch.Client
	batcher *Batcher
	metrics *OutputMetrics
	mu      sync.RWMutex
	closed  atomic.Bool
}

// NewElasticsearchOutput creates a new Elasticsearch output
func NewElasticsearchOutput(config ElasticsearchConfig) (*ElasticsearchOutput, error) {
	if len(config.Addresses) == 0 && config.CloudID == "" {
		return nil, fmt.Errorf("no addresses or cloud ID specified")
	}

	if config.Index == "" {
		return nil, fmt.Errorf("no index specified")
	}

	// Create Elasticsearch config
	esConfig := elasticsearch.Config{
		Addresses: config.Addresses,
		CloudID:   config.CloudID,
		Username:  config.Username,
		Password:  config.Password,
		APIKey:    config.APIKey,
	}

	// Create client
	client, err := elasticsearch.NewClient(esConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}

	// Test connection
	res, err := client.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Elasticsearch: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch returned error: %s", res.Status())
	}

	output := &ElasticsearchOutput{
		config:  config,
		client:  client,
		metrics: &OutputMetrics{},
	}

	// Create batcher
	if config.BatchSize > 1 {
		output.batcher = NewBatcher(BatcherConfig{
			MaxBatchSize:  config.BatchSize,
			MaxBatchBytes: 10 * 1024 * 1024, // 10MB default bulk size
			FlushInterval: config.FlushInterval,
		}, output.sendBatchInternal)
	}

	return output, nil
}

// Send sends a single event to Elasticsearch
func (e *ElasticsearchOutput) Send(ctx context.Context, event *types.LogEvent) error {
	if e.closed.Load() {
		return fmt.Errorf("elasticsearch output is closed")
	}

	// Use batcher if configured
	if e.batcher != nil {
		return e.batcher.Add(ctx, event)
	}

	return e.sendSingle(ctx, event)
}

// SendBatch sends a batch of events to Elasticsearch
func (e *ElasticsearchOutput) SendBatch(ctx context.Context, events []*types.LogEvent) error {
	if e.closed.Load() {
		return fmt.Errorf("elasticsearch output is closed")
	}

	return e.sendBatchInternal(ctx, events)
}

// sendSingle sends a single event without batching
func (e *ElasticsearchOutput) sendSingle(ctx context.Context, event *types.LogEvent) error {
	index := e.getIndexName(event)

	// Serialize event
	doc, err := json.Marshal(event)
	if err != nil {
		atomic.AddInt64(&e.metrics.EventsFailed, 1)
		e.metrics.LastError = err.Error()
		e.metrics.LastErrorTime = time.Now()
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	startTime := time.Now()

	// Index document
	req := esapi.IndexRequest{
		Index:   index,
		Body:    bytes.NewReader(doc),
		Refresh: "false",
	}

	if e.config.Pipeline != "" {
		req.Pipeline = e.config.Pipeline
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		atomic.AddInt64(&e.metrics.EventsFailed, 1)
		e.metrics.LastError = err.Error()
		e.metrics.LastErrorTime = time.Now()
		return fmt.Errorf("failed to index document: %w", err)
	}
	defer res.Body.Close()

	latency := time.Since(startTime)

	if res.IsError() {
		atomic.AddInt64(&e.metrics.EventsFailed, 1)
		e.metrics.LastError = res.Status()
		e.metrics.LastErrorTime = time.Now()
		return fmt.Errorf("elasticsearch returned error: %s", res.Status())
	}

	// Update metrics
	atomic.AddInt64(&e.metrics.EventsSent, 1)
	atomic.AddInt64(&e.metrics.BytesSent, int64(len(doc)))
	e.metrics.LastSendTime = time.Now()

	// Update average latency
	e.mu.Lock()
	e.metrics.AvgLatency = (e.metrics.AvgLatency + latency) / 2
	e.mu.Unlock()

	return nil
}

// sendBatchInternal sends a batch of events using the Bulk API
func (e *ElasticsearchOutput) sendBatchInternal(ctx context.Context, events []*types.LogEvent) error {
	if len(events) == 0 {
		return nil
	}

	startTime := time.Now()

	// Build bulk request body
	var buf bytes.Buffer
	var totalBytes int64

	for _, event := range events {
		index := e.getIndexName(event)

		// Action metadata
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": index,
			},
		}
		if e.config.Pipeline != "" {
			meta["index"].(map[string]interface{})["pipeline"] = e.config.Pipeline
		}

		metaJSON, err := json.Marshal(meta)
		if err != nil {
			atomic.AddInt64(&e.metrics.EventsFailed, 1)
			continue
		}

		// Document
		docJSON, err := json.Marshal(event)
		if err != nil {
			atomic.AddInt64(&e.metrics.EventsFailed, 1)
			continue
		}

		buf.Write(metaJSON)
		buf.WriteByte('\n')
		buf.Write(docJSON)
		buf.WriteByte('\n')

		totalBytes += int64(len(docJSON))
	}

	// Send bulk request
	res, err := e.client.Bulk(bytes.NewReader(buf.Bytes()), e.client.Bulk.WithContext(ctx))
	if err != nil {
		atomic.AddInt64(&e.metrics.EventsFailed, int64(len(events)))
		e.metrics.LastError = err.Error()
		e.metrics.LastErrorTime = time.Now()
		return fmt.Errorf("bulk request failed: %w", err)
	}
	defer res.Body.Close()

	latency := time.Since(startTime)

	if res.IsError() {
		atomic.AddInt64(&e.metrics.EventsFailed, int64(len(events)))
		e.metrics.LastError = res.Status()
		e.metrics.LastErrorTime = time.Now()
		return fmt.Errorf("bulk request returned error: %s", res.Status())
	}

	// Parse bulk response
	var bulkResp struct {
		Errors bool `json:"errors"`
		Items  []map[string]struct {
			Status int    `json:"status"`
			Error  string `json:"error"`
		} `json:"items"`
	}

	if err := json.NewDecoder(res.Body).Decode(&bulkResp); err != nil {
		atomic.AddInt64(&e.metrics.EventsFailed, int64(len(events)))
		e.metrics.LastError = err.Error()
		e.metrics.LastErrorTime = time.Now()
		return fmt.Errorf("failed to parse bulk response: %w", err)
	}

	// Count successes and failures
	var failedCount int64
	if bulkResp.Errors {
		for _, item := range bulkResp.Items {
			for _, doc := range item {
				if doc.Status >= 400 {
					failedCount++
					e.metrics.LastError = doc.Error
					e.metrics.LastErrorTime = time.Now()
				}
			}
		}
	}

	successCount := int64(len(events)) - failedCount

	// Update metrics
	atomic.AddInt64(&e.metrics.EventsSent, successCount)
	atomic.AddInt64(&e.metrics.EventsFailed, failedCount)
	atomic.AddInt64(&e.metrics.BytesSent, totalBytes)
	atomic.AddInt64(&e.metrics.BatchesSent, 1)
	e.metrics.LastSendTime = time.Now()

	// Update average batch size and latency
	e.mu.Lock()
	if e.metrics.BatchesSent > 0 {
		e.metrics.AvgBatchSize = float64(e.metrics.EventsSent) / float64(e.metrics.BatchesSent)
	}
	e.metrics.AvgLatency = (e.metrics.AvgLatency + latency) / 2
	e.mu.Unlock()

	if failedCount > 0 {
		return fmt.Errorf("%d out of %d events failed to index", failedCount, len(events))
	}

	return nil
}

// getIndexName returns the index name for an event, with optional time-based rotation
func (e *ElasticsearchOutput) getIndexName(event *types.LogEvent) string {
	index := e.config.Index

	// Apply index rotation
	if e.config.IndexRotation != "none" && e.config.IndexRotation != "" {
		timestamp := event.Timestamp
		if timestamp.IsZero() {
			timestamp = time.Now()
		}

		var suffix string
		switch e.config.IndexRotation {
		case "daily":
			suffix = timestamp.Format("2006.01.02")
		case "weekly":
			year, week := timestamp.ISOWeek()
			suffix = fmt.Sprintf("%d.%02d", year, week)
		case "monthly":
			suffix = timestamp.Format("2006.01")
		case "yearly":
			suffix = timestamp.Format("2006")
		default:
			suffix = timestamp.Format("2006.01.02")
		}

		// Check if index already has a suffix pattern
		if strings.Contains(index, "%{") {
			// Replace patterns
			index = strings.ReplaceAll(index, "%{+YYYY.MM.dd}", timestamp.Format("2006.01.02"))
			index = strings.ReplaceAll(index, "%{+YYYY.MM}", timestamp.Format("2006.01"))
			index = strings.ReplaceAll(index, "%{+YYYY}", timestamp.Format("2006"))
		} else {
			index = fmt.Sprintf("%s-%s", index, suffix)
		}
	}

	return index
}

// Close closes the Elasticsearch output
func (e *ElasticsearchOutput) Close() error {
	if !e.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}

	// Stop batcher first
	if e.batcher != nil {
		if err := e.batcher.Stop(); err != nil {
			return err
		}
	}

	return nil
}

// Name returns the output name
func (e *ElasticsearchOutput) Name() string {
	if e.config.Name != "" {
		return e.config.Name
	}
	return "elasticsearch"
}

// Metrics returns the current metrics
func (e *ElasticsearchOutput) Metrics() *OutputMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Return a copy
	metricsCopy := *e.metrics
	return &metricsCopy
}
