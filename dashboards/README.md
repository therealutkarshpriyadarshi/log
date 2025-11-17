# Grafana Dashboards

This directory contains Grafana dashboard JSON files for monitoring the log aggregator system.

## Dashboards

### 1. Overview Dashboard (`overview.json`)
Provides a high-level view of the system health and performance:
- **Total events received and sent** - Real-time event throughput
- **Parser errors** - Track parsing failures
- **System health** - Overall health status
- **Event throughput by input/output** - Breakdown by source and destination
- **Buffer utilization** - Monitor buffer usage and potential bottlenecks
- **Parser performance** - p50/p95/p99 latencies

### 2. Performance Dashboard (`performance.json`)
Detailed performance metrics and latency analysis:
- **Output latency** - p50/p95/p99 percentiles for all outputs
- **Parser latency** - Parsing performance by parser type
- **Worker job duration** - Worker pool performance
- **WAL write latency** - Write-ahead log performance
- **Batch size distribution** - Output batching statistics
- **System goroutines** - Concurrent goroutine count
- **GC pause time** - Garbage collection impact

### 3. Errors & Resources Dashboard (`errors-and-resources.json`)
Error tracking and resource utilization:
- **Parser errors** - Parsing failures by type and reason
- **Output errors** - Output failures by destination
- **Buffer drops** - Events dropped due to buffer overflow
- **Input drops** - Events dropped at input stage
- **Dead letter queue** - Failed events tracking
- **Circuit breaker state** - Circuit breaker status monitoring
- **Memory usage** - Heap and system memory consumption
- **Worker pool stats** - Worker pool utilization
- **WAL stats** - WAL segment count and write rate
- **Component health** - Individual component health status

## Importing Dashboards

### Using Grafana UI

1. Open Grafana in your browser
2. Navigate to **Dashboards** → **Import**
3. Click **Upload JSON file** or paste the JSON content
4. Select your Prometheus data source
5. Click **Import**

### Using API

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d @overview.json \
  http://your-grafana-host/api/dashboards/db
```

### Using Provisioning

1. Copy dashboard JSON files to Grafana provisioning directory:
```bash
cp dashboards/*.json /etc/grafana/provisioning/dashboards/
```

2. Create provisioning config file `/etc/grafana/provisioning/dashboards/logaggregator.yaml`:
```yaml
apiVersion: 1

providers:
  - name: 'LogAggregator'
    orgId: 1
    folder: 'Log Aggregator'
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: /etc/grafana/provisioning/dashboards
      foldersFromFilesStructure: true
```

3. Restart Grafana

## Alerts

Several panels include pre-configured alerts:

### Overview Dashboard
- **High Buffer Utilization** - Alerts when buffer utilization > 90%

### Performance Dashboard
- **High Output Latency** - Alerts when p99 latency > 100ms

### Errors & Resources Dashboard
- **Buffer Drops Detected** - Alerts when events are being dropped
- **High Memory Usage** - Alerts when memory usage > 500MB

## Customization

### Modifying Thresholds

Edit the JSON files to adjust alert thresholds:

```json
"alert": {
  "conditions": [
    {
      "evaluator": {"params": [0.9], "type": "gt"},  // Change threshold here
      ...
    }
  ]
}
```

### Adding Panels

You can add new panels through the Grafana UI or by editing the JSON directly. Useful Prometheus queries:

```promql
# Events per second by input type
rate(logaggregator_input_events_received_total[5m])

# Output success rate
rate(logaggregator_output_events_sent_total[5m]) /
  (rate(logaggregator_output_events_sent_total[5m]) +
   rate(logaggregator_output_events_failed_total[5m]))

# Average batch size
rate(logaggregator_output_batch_size_sum[5m]) /
  rate(logaggregator_output_batch_size_count[5m])

# Buffer utilization percentage
logaggregator_buffer_utilization_ratio * 100
```

## Prometheus Configuration

Ensure Prometheus is configured to scrape the log aggregator metrics endpoint:

```yaml
scrape_configs:
  - job_name: 'logaggregator'
    static_configs:
      - targets: ['localhost:9090']  # Adjust to your metrics address
    scrape_interval: 15s
```

## Variables

Consider adding dashboard variables for dynamic filtering:

- **$datasource** - Prometheus data source selector
- **$input** - Filter by input name
- **$output** - Filter by output name
- **$interval** - Time range selector

Example variable query:
```promql
label_values(logaggregator_input_events_received_total, input_name)
```
