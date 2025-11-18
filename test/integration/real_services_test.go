// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
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

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// waitForService waits for a service to be ready
func waitForService(t *testing.T, serviceName string, checkFunc func() error, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for %s to be ready", serviceName)
		case <-ticker.C:
			if err := checkFunc(); err == nil {
				t.Logf("%s is ready", serviceName)
				return
			}
		}
	}
}

// TestKafkaIntegration tests real Kafka connection and operations
func TestKafkaIntegration(t *testing.T) {
	brokers := strings.Split(getEnvOrDefault("KAFKA_BROKERS", "localhost:29092"), ",")
	topic := "test-logs-" + fmt.Sprintf("%d", time.Now().Unix())

	// Create Kafka config
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 3
	config.Version = sarama.V2_8_0_0

	// Wait for Kafka to be ready
	waitForService(t, "Kafka", func() error {
		client, err := sarama.NewClient(brokers, config)
		if err != nil {
			return err
		}
		defer client.Close()
		return nil
	}, 60*time.Second)

	t.Run("ProducerAndConsumer", func(t *testing.T) {
		// Create producer
		producer, err := sarama.NewSyncProducer(brokers, config)
		if err != nil {
			t.Fatalf("Failed to create producer: %v", err)
		}
		defer producer.Close()

		// Produce messages
		testMessage := map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"level":     "info",
			"message":   "Integration test message",
			"service":   "test-service",
		}

		messageJSON, err := json.Marshal(testMessage)
		if err != nil {
			t.Fatalf("Failed to marshal message: %v", err)
		}

		msg := &sarama.ProducerMessage{
			Topic: topic,
			Value: sarama.ByteEncoder(messageJSON),
		}

		partition, offset, err := producer.SendMessage(msg)
		if err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}

		t.Logf("Message sent to partition %d at offset %d", partition, offset)

		// Create consumer
		consumer, err := sarama.NewConsumer(brokers, config)
		if err != nil {
			t.Fatalf("Failed to create consumer: %v", err)
		}
		defer consumer.Close()

		// Consume messages
		partitionConsumer, err := consumer.ConsumePartition(topic, partition, offset)
		if err != nil {
			t.Fatalf("Failed to create partition consumer: %v", err)
		}
		defer partitionConsumer.Close()

		// Read message with timeout
		select {
		case msg := <-partitionConsumer.Messages():
			t.Logf("Received message: %s", string(msg.Value))

			var received map[string]interface{}
			if err := json.Unmarshal(msg.Value, &received); err != nil {
				t.Fatalf("Failed to unmarshal received message: %v", err)
			}

			if received["message"] != testMessage["message"] {
				t.Errorf("Expected message %v, got %v", testMessage["message"], received["message"])
			}

		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for message")
		}
	})

	t.Run("BatchProducer", func(t *testing.T) {
		producer, err := sarama.NewSyncProducer(brokers, config)
		if err != nil {
			t.Fatalf("Failed to create producer: %v", err)
		}
		defer producer.Close()

		// Send batch of messages
		batchSize := 100
		for i := 0; i < batchSize; i++ {
			msg := &sarama.ProducerMessage{
				Topic: topic,
				Value: sarama.StringEncoder(fmt.Sprintf(`{"id":%d,"message":"batch message %d"}`, i, i)),
			}

			_, _, err := producer.SendMessage(msg)
			if err != nil {
				t.Errorf("Failed to send message %d: %v", i, err)
			}
		}

		t.Logf("Successfully sent %d messages in batch", batchSize)
	})
}

// TestElasticsearchIntegration tests real Elasticsearch connection and operations
func TestElasticsearchIntegration(t *testing.T) {
	esURL := getEnvOrDefault("ELASTICSEARCH_URL", "http://localhost:9200")

	// Create Elasticsearch client
	cfg := elasticsearch.Config{
		Addresses: []string{esURL},
	}

	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create Elasticsearch client: %v", err)
	}

	// Wait for Elasticsearch to be ready
	waitForService(t, "Elasticsearch", func() error {
		_, err := es.Info()
		return err
	}, 60*time.Second)

	indexName := "test-logs-" + fmt.Sprintf("%d", time.Now().Unix())

	t.Run("IndexAndSearch", func(t *testing.T) {
		ctx := context.Background()

		// Index a document
		doc := map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"level":     "info",
			"message":   "Integration test log entry",
			"service":   "test-service",
			"host":      "test-host",
		}

		docJSON, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("Failed to marshal document: %v", err)
		}

		res, err := es.Index(
			indexName,
			strings.NewReader(string(docJSON)),
			es.Index.WithContext(ctx),
			es.Index.WithRefresh("true"),
		)
		if err != nil {
			t.Fatalf("Failed to index document: %v", err)
		}
		defer res.Body.Close()

		if res.IsError() {
			t.Fatalf("Error indexing document: %s", res.String())
		}

		t.Logf("Document indexed successfully")

		// Search for the document
		query := map[string]interface{}{
			"query": map[string]interface{}{
				"match": map[string]interface{}{
					"message": "Integration test",
				},
			},
		}

		queryJSON, err := json.Marshal(query)
		if err != nil {
			t.Fatalf("Failed to marshal query: %v", err)
		}

		searchRes, err := es.Search(
			es.Search.WithContext(ctx),
			es.Search.WithIndex(indexName),
			es.Search.WithBody(strings.NewReader(string(queryJSON))),
		)
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		defer searchRes.Body.Close()

		if searchRes.IsError() {
			t.Fatalf("Error searching: %s", searchRes.String())
		}

		var result map[string]interface{}
		if err := json.NewDecoder(searchRes.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode search results: %v", err)
		}

		hits := result["hits"].(map[string]interface{})
		total := hits["total"].(map[string]interface{})
		value := int(total["value"].(float64))

		if value < 1 {
			t.Errorf("Expected at least 1 search result, got %d", value)
		}

		t.Logf("Found %d documents", value)
	})

	t.Run("BulkIndex", func(t *testing.T) {
		ctx := context.Background()
		var buf strings.Builder

		// Create bulk request
		for i := 0; i < 10; i++ {
			meta := map[string]interface{}{
				"index": map[string]interface{}{
					"_index": indexName,
				},
			}
			metaJSON, _ := json.Marshal(meta)
			buf.WriteString(string(metaJSON) + "\n")

			doc := map[string]interface{}{
				"id":      i,
				"message": fmt.Sprintf("Bulk message %d", i),
			}
			docJSON, _ := json.Marshal(doc)
			buf.WriteString(string(docJSON) + "\n")
		}

		res, err := es.Bulk(
			strings.NewReader(buf.String()),
			es.Bulk.WithContext(ctx),
			es.Bulk.WithRefresh("true"),
		)
		if err != nil {
			t.Fatalf("Failed to bulk index: %v", err)
		}
		defer res.Body.Close()

		if res.IsError() {
			t.Fatalf("Error bulk indexing: %s", res.String())
		}

		t.Logf("Bulk indexed 10 documents successfully")
	})

	// Cleanup
	t.Cleanup(func() {
		_, _ = es.Indices.Delete([]string{indexName})
	})
}

// TestS3Integration tests real S3 (MinIO) connection and operations
func TestS3Integration(t *testing.T) {
	endpoint := getEnvOrDefault("S3_ENDPOINT", "http://localhost:9000")
	accessKey := getEnvOrDefault("S3_ACCESS_KEY", "minioadmin")
	secretKey := getEnvOrDefault("S3_SECRET_KEY", "minioadmin")
	bucket := getEnvOrDefault("S3_BUCKET", "test-logs")

	// Create S3 client
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		t.Fatalf("Failed to load AWS config: %v", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	// Wait for S3 to be ready
	waitForService(t, "S3/MinIO", func() error {
		_, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
		return err
	}, 60*time.Second)

	t.Run("PutAndGetObject", func(t *testing.T) {
		key := fmt.Sprintf("test/log-%d.json", time.Now().Unix())
		content := `{"timestamp":"2024-01-01T00:00:00Z","level":"info","message":"Test log entry"}`

		// Put object
		_, err := client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			Body:   strings.NewReader(content),
		})
		if err != nil {
			t.Fatalf("Failed to put object: %v", err)
		}

		t.Logf("Object uploaded: %s", key)

		// Get object
		result, err := client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			t.Fatalf("Failed to get object: %v", err)
		}
		defer result.Body.Close()

		var buf strings.Builder
		_, err = buf.ReadFrom(result.Body)
		if err != nil {
			t.Fatalf("Failed to read object body: %v", err)
		}

		if buf.String() != content {
			t.Errorf("Expected content %s, got %s", content, buf.String())
		}

		t.Logf("Object retrieved successfully")

		// Cleanup
		_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
	})

	t.Run("ListObjects", func(t *testing.T) {
		// Upload multiple objects
		prefix := fmt.Sprintf("test-list-%d/", time.Now().Unix())
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("%slog-%d.json", prefix, i)
			content := fmt.Sprintf(`{"id":%d}`, i)

			_, err := client.PutObject(ctx, &s3.PutObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(key),
				Body:   strings.NewReader(content),
			})
			if err != nil {
				t.Fatalf("Failed to put object %s: %v", key, err)
			}
		}

		// List objects
		result, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String(bucket),
			Prefix: aws.String(prefix),
		})
		if err != nil {
			t.Fatalf("Failed to list objects: %v", err)
		}

		if *result.KeyCount != 5 {
			t.Errorf("Expected 5 objects, got %d", *result.KeyCount)
		}

		t.Logf("Listed %d objects successfully", *result.KeyCount)

		// Cleanup
		for _, obj := range result.Contents {
			_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucket),
				Key:    obj.Key,
			})
		}
	})
}

// TestFullPipeline tests the complete log processing pipeline
func TestFullPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping full pipeline test in short mode")
	}

	// This test requires all services to be running
	// It will be implemented in the E2E test suite
	t.Log("Full pipeline test - see E2E tests")
}
