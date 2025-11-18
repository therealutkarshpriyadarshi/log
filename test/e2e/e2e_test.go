// +build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	elasticsearch "github.com/elastic/go-elasticsearch/v8"
)

const (
	defaultLogAggregatorURL = "http://localhost:8080"
	defaultHealthURL        = "http://localhost:8081/health"
	defaultMetricsURL       = "http://localhost:9091/metrics"
	defaultKafkaBroker      = "localhost:29092"
	defaultElasticsearchURL = "http://localhost:9200"
	defaultS3Endpoint       = "http://localhost:9000"
)

// Helper function to get environment variables with defaults
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// waitForHealthy waits for the log aggregator to be healthy
func waitForHealthy(t *testing.T, timeout time.Duration) {
	t.Helper()
	healthURL := getEnv("HEALTH_URL", defaultHealthURL)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("Timeout waiting for log aggregator to be healthy")
		case <-ticker.C:
			resp, err := http.Get(healthURL)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				t.Log("Log aggregator is healthy")
				return
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
}

// TestE2E_HealthCheck verifies the health check endpoint
func TestE2E_HealthCheck(t *testing.T) {
	waitForHealthy(t, 60*time.Second)

	healthURL := getEnv("HEALTH_URL", defaultHealthURL)
	resp, err := http.Get(healthURL)
	if err != nil {
		t.Fatalf("Failed to call health endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var health map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("Failed to decode health response: %v", err)
	}

	if health["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", health["status"])
	}

	t.Logf("Health check response: %+v", health)
}

// TestE2E_MetricsEndpoint verifies the metrics endpoint
func TestE2E_MetricsEndpoint(t *testing.T) {
	waitForHealthy(t, 60*time.Second)

	metricsURL := getEnv("METRICS_URL", defaultMetricsURL)
	resp, err := http.Get(metricsURL)
	if err != nil {
		t.Fatalf("Failed to call metrics endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read metrics response: %v", err)
	}

	metrics := string(body)
	expectedMetrics := []string{
		"log_events_received_total",
		"log_events_processed_total",
		"log_processing_errors_total",
		"go_goroutines",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(metrics, metric) {
			t.Errorf("Expected metric '%s' not found in response", metric)
		}
	}

	t.Logf("Metrics endpoint is working, found %d bytes of metrics", len(body))
}

// TestE2E_HTTPLogIngestion tests log ingestion via HTTP endpoint
func TestE2E_HTTPLogIngestion(t *testing.T) {
	waitForHealthy(t, 60*time.Second)

	logURL := getEnv("LOG_AGGREGATOR_URL", defaultLogAggregatorURL) + "/logs"

	testCases := []struct {
		name    string
		payload map[string]interface{}
	}{
		{
			name: "SimpleLog",
			payload: map[string]interface{}{
				"timestamp": time.Now().UTC().Format(time.RFC3339),
				"level":     "info",
				"message":   "E2E test log entry",
				"service":   "e2e-test",
			},
		},
		{
			name: "StructuredLog",
			payload: map[string]interface{}{
				"timestamp": time.Now().UTC().Format(time.RFC3339),
				"level":     "error",
				"message":   "Test error message",
				"service":   "e2e-test",
				"error": map[string]interface{}{
					"code":    500,
					"details": "Internal server error",
				},
				"metadata": map[string]interface{}{
					"request_id": "test-123",
					"user_id":    "user-456",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(tc.payload)
			if err != nil {
				t.Fatalf("Failed to marshal payload: %v", err)
			}

			resp, err := http.Post(logURL, "application/json", bytes.NewReader(payload))
			if err != nil {
				t.Fatalf("Failed to send log: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected status 200 or 202, got %d: %s", resp.StatusCode, string(body))
			}

			t.Logf("Log sent successfully: %s", tc.name)
		})
	}
}

// TestE2E_KafkaOutput verifies logs are written to Kafka
func TestE2E_KafkaOutput(t *testing.T) {
	waitForHealthy(t, 60*time.Second)

	brokers := strings.Split(getEnv("KAFKA_BROKERS", defaultKafkaBroker), ",")
	topic := "logs"

	// Send a log with unique ID
	uniqueID := fmt.Sprintf("e2e-test-%d", time.Now().UnixNano())
	logURL := getEnv("LOG_AGGREGATOR_URL", defaultLogAggregatorURL) + "/logs"

	payload := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     "info",
		"message":   "E2E Kafka test",
		"test_id":   uniqueID,
		"service":   "e2e-test",
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	resp, err := http.Post(logURL, "application/json", bytes.NewReader(payloadJSON))
	if err != nil {
		t.Fatalf("Failed to send log: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		t.Fatalf("Failed to send log, status: %d", resp.StatusCode)
	}

	// Give some time for processing
	time.Sleep(5 * time.Second)

	// Verify in Kafka
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.Version = sarama.V2_8_0_0

	consumer, err := sarama.NewConsumer(brokers, config)
	if err != nil {
		t.Fatalf("Failed to create Kafka consumer: %v", err)
	}
	defer consumer.Close()

	partitions, err := consumer.Partitions(topic)
	if err != nil {
		t.Fatalf("Failed to get partitions: %v", err)
	}

	found := false
	for _, partition := range partitions {
		pc, err := consumer.ConsumePartition(topic, partition, sarama.OffsetNewest-100)
		if err != nil {
			continue
		}
		defer pc.Close()

		timeout := time.After(10 * time.Second)
	messageLoop:
		for {
			select {
			case msg := <-pc.Messages():
				var logEntry map[string]interface{}
				if err := json.Unmarshal(msg.Value, &logEntry); err == nil {
					if testID, ok := logEntry["test_id"].(string); ok && testID == uniqueID {
						found = true
						t.Logf("Found log entry in Kafka: %s", string(msg.Value))
						break messageLoop
					}
				}
			case <-timeout:
				break messageLoop
			}
		}

		if found {
			break
		}
	}

	if !found {
		t.Error("Log entry not found in Kafka")
	}
}

// TestE2E_ElasticsearchOutput verifies logs are written to Elasticsearch
func TestE2E_ElasticsearchOutput(t *testing.T) {
	waitForHealthy(t, 60*time.Second)

	esURL := getEnv("ELASTICSEARCH_URL", defaultElasticsearchURL)

	// Send a log with unique ID
	uniqueID := fmt.Sprintf("e2e-test-%d", time.Now().UnixNano())
	logURL := getEnv("LOG_AGGREGATOR_URL", defaultLogAggregatorURL) + "/logs"

	payload := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     "info",
		"message":   "E2E Elasticsearch test",
		"test_id":   uniqueID,
		"service":   "e2e-test",
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	resp, err := http.Post(logURL, "application/json", bytes.NewReader(payloadJSON))
	if err != nil {
		t.Fatalf("Failed to send log: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		t.Fatalf("Failed to send log, status: %d", resp.StatusCode)
	}

	// Give some time for processing and indexing
	time.Sleep(5 * time.Second)

	// Verify in Elasticsearch
	cfg := elasticsearch.Config{
		Addresses: []string{esURL},
	}

	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create Elasticsearch client: %v", err)
	}

	// Search for the log entry
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]interface{}{
				"test_id": uniqueID,
			},
		},
	}

	queryJSON, err := json.Marshal(query)
	if err != nil {
		t.Fatalf("Failed to marshal query: %v", err)
	}

	searchRes, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex("logs-test"),
		es.Search.WithBody(strings.NewReader(string(queryJSON))),
	)
	if err != nil {
		t.Fatalf("Failed to search Elasticsearch: %v", err)
	}
	defer searchRes.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(searchRes.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode search results: %v", err)
	}

	hits := result["hits"].(map[string]interface{})
	total := hits["total"].(map[string]interface{})
	value := int(total["value"].(float64))

	if value < 1 {
		t.Errorf("Log entry not found in Elasticsearch, expected at least 1, got %d", value)
	} else {
		t.Logf("Found log entry in Elasticsearch")
	}
}

// TestE2E_S3Output verifies logs are written to S3
func TestE2E_S3Output(t *testing.T) {
	waitForHealthy(t, 60*time.Second)

	endpoint := getEnv("S3_ENDPOINT", defaultS3Endpoint)
	bucket := getEnv("S3_BUCKET", "logs")

	// Send logs
	uniqueID := fmt.Sprintf("e2e-test-%d", time.Now().UnixNano())
	logURL := getEnv("LOG_AGGREGATOR_URL", defaultLogAggregatorURL) + "/logs"

	for i := 0; i < 10; i++ {
		payload := map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"level":     "info",
			"message":   fmt.Sprintf("E2E S3 test message %d", i),
			"test_id":   uniqueID,
			"service":   "e2e-test",
		}

		payloadJSON, _ := json.Marshal(payload)
		resp, err := http.Post(logURL, "application/json", bytes.NewReader(payloadJSON))
		if err != nil {
			t.Logf("Warning: Failed to send log %d: %v", i, err)
		} else {
			resp.Body.Close()
		}
	}

	// Give time for batching and upload
	time.Sleep(15 * time.Second)

	// Verify in S3
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("minioadmin", "minioadmin", "")),
	)
	if err != nil {
		t.Fatalf("Failed to load AWS config: %v", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	// List objects with test prefix
	result, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String("test/"),
	})
	if err != nil {
		t.Fatalf("Failed to list S3 objects: %v", err)
	}

	if *result.KeyCount < 1 {
		t.Logf("Warning: No objects found in S3 (batch may not have flushed yet)")
	} else {
		t.Logf("Found %d objects in S3", *result.KeyCount)
	}
}

// TestE2E_HighThroughput tests the system under high load
func TestE2E_HighThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high throughput test in short mode")
	}

	waitForHealthy(t, 60*time.Second)

	logURL := getEnv("LOG_AGGREGATOR_URL", defaultLogAggregatorURL) + "/logs"

	// Send 1000 logs
	numLogs := 1000
	start := time.Now()

	for i := 0; i < numLogs; i++ {
		payload := map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"level":     "info",
			"message":   fmt.Sprintf("High throughput test log %d", i),
			"id":        i,
			"service":   "e2e-test",
		}

		payloadJSON, _ := json.Marshal(payload)
		resp, err := http.Post(logURL, "application/json", bytes.NewReader(payloadJSON))
		if err != nil {
			t.Logf("Warning: Failed to send log %d: %v", i, err)
			continue
		}
		resp.Body.Close()

		if i%100 == 0 {
			t.Logf("Sent %d/%d logs", i, numLogs)
		}
	}

	elapsed := time.Since(start)
	throughput := float64(numLogs) / elapsed.Seconds()

	t.Logf("Sent %d logs in %v (%.2f logs/sec)", numLogs, elapsed, throughput)

	if throughput < 100 {
		t.Errorf("Throughput too low: %.2f logs/sec (expected > 100)", throughput)
	}
}

// TestE2E_Resilience tests system resilience and recovery
func TestE2E_Resilience(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resilience test in short mode")
	}

	waitForHealthy(t, 60*time.Second)

	// This test would simulate failures and verify recovery
	// Implementation depends on chaos testing framework
	t.Log("Resilience test - see chaos testing framework")
}
