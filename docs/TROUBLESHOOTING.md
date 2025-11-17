# Troubleshooting Guide

Common issues and their solutions for logaggregator.

## Table of Contents

- [Installation Issues](#installation-issues)
- [Runtime Issues](#runtime-issues)
- [Performance Issues](#performance-issues)
- [Configuration Issues](#configuration-issues)
- [Output Issues](#output-issues)
- [Kubernetes Issues](#kubernetes-issues)

## Installation Issues

### Binary Won't Start

**Symptom**: Binary exits immediately or won't start

**Possible Causes**:
1. Missing configuration file
2. Invalid configuration
3. Permission issues
4. Missing dependencies

**Solutions**:

```bash
# Check if config file exists
ls -la /etc/logaggregator/config.yaml

# Validate config
./bin/logaggregator -config /etc/logaggregator/config.yaml -validate

# Check permissions
sudo chown logaggregator:logaggregator /var/lib/logaggregator
sudo chmod 755 /var/lib/logaggregator

# Check logs
journalctl -u logaggregator -n 50
```

### Docker Container Won't Start

**Symptom**: Container exits immediately

**Solutions**:

```bash
# Check container logs
docker logs logaggregator

# Run interactively
docker run -it --rm logaggregator:latest /bin/sh

# Check volume mounts
docker inspect logaggregator | grep -A 10 Mounts

# Verify config
docker exec logaggregator cat /etc/logaggregator/config.yaml
```

## Runtime Issues

### High CPU Usage

**Symptom**: CPU usage > 100%

**Diagnosis**:

```bash
# Check profiling data
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof

# Check goroutine count
curl http://localhost:6060/debug/stats | jq '.Goroutines'

# Check metrics
curl http://localhost:9090/metrics | grep cpu
```

**Solutions**:

1. **Reduce worker count**:
```yaml
worker_pool:
  num_workers: 4  # Reduce from default 8
```

2. **Enable object pooling**:
```yaml
performance:
  enable_pooling: true
```

3. **Adjust buffer size**:
```yaml
buffer:
  size: 262144  # Reduce from 524288
```

### High Memory Usage

**Symptom**: Memory usage growing unbounded

**Diagnosis**:

```bash
# Capture memory profile
curl http://localhost:6060/debug/pprof/heap > mem.prof
go tool pprof mem.prof

# Check for goroutine leaks
curl http://localhost:6060/debug/pprof/goroutine > goroutine.prof
go tool pprof goroutine.prof
```

**Solutions**:

1. **Reduce buffer size**:
```yaml
buffer:
  size: 131072  # Smaller buffer
```

2. **Enable backpressure**:
```yaml
buffer:
  backpressure_strategy: block  # Or sample
```

3. **Adjust GC**:
```yaml
performance:
  gc_percent: 50  # More aggressive GC
```

### Events Not Being Processed

**Symptom**: Events received but not outputted

**Diagnosis**:

```bash
# Check metrics
curl http://localhost:9090/metrics | grep events

# Check buffer status
curl http://localhost:9090/metrics | grep buffer_utilization

# Check logs for errors
journalctl -u logaggregator | grep ERROR
```

**Solutions**:

1. **Check output configuration**:
```bash
# Test output connectivity
telnet kafka-broker 9092
```

2. **Check dead letter queue**:
```bash
ls -la /var/lib/logaggregator/dlq/
```

3. **Enable debug logging**:
```yaml
logging:
  level: debug
```

### File Rotation Not Detected

**Symptom**: Stops reading after file rotation

**Diagnosis**:

```bash
# Check checkpoint file
cat /var/lib/logaggregator/checkpoints/checkpoint.json

# Check inode changes
ls -li /var/log/app.log
```

**Solutions**:

1. **Restart logaggregator** to reload files

2. **Check checkpoint interval**:
```yaml
inputs:
  files:
    - checkpoint_interval: 1s  # More frequent checkpoints
```

## Performance Issues

### Low Throughput

**Symptom**: Processing < expected events/sec

**Diagnosis**:

```bash
# Check throughput metrics
curl http://localhost:9090/metrics | grep throughput

# Check latency
curl http://localhost:9090/metrics | grep latency

# Check CPU utilization
curl http://localhost:9090/metrics | grep cpu
```

**Solutions**:

1. **Increase workers**:
```yaml
worker_pool:
  num_workers: 16  # More parallelism
```

2. **Increase buffer**:
```yaml
buffer:
  size: 1048576  # Larger buffer
```

3. **Optimize parsing**:
```yaml
parser:
  type: json  # Fastest parser
```

### High Latency

**Symptom**: p99 latency > 100ms

**Diagnosis**:

```bash
# Check latency distribution
curl http://localhost:9090/metrics | grep latency_bucket

# Check blocking operations
curl http://localhost:6060/debug/pprof/block > block.prof
```

**Solutions**:

1. **Reduce batch size**:
```yaml
output:
  kafka:
    batch_size: 100  # Smaller batches
    batch_timeout: 100ms
```

2. **Adjust backpressure**:
```yaml
buffer:
  backpressure_strategy: drop  # Don't block
```

### Dropped Events

**Symptom**: Events being dropped

**Diagnosis**:

```bash
# Check drop metrics
curl http://localhost:9090/metrics | grep dropped

# Check buffer utilization
curl http://localhost:9090/metrics | grep buffer_utilization
```

**Solutions**:

1. **Increase buffer**:
```yaml
buffer:
  size: 2097152  # Double buffer size
```

2. **Change backpressure strategy**:
```yaml
buffer:
  backpressure_strategy: block  # Block instead of drop
  block_timeout: 1s
```

3. **Enable WAL**:
```yaml
wal:
  enabled: true
  dir: /var/lib/logaggregator/wal
```

## Configuration Issues

### Invalid YAML

**Symptom**: "failed to parse config" error

**Solutions**:

```bash
# Validate YAML syntax
yamllint config.yaml

# Check for tabs (use spaces)
cat -A config.yaml | grep '\^I'

# Test config
./bin/logaggregator -config config.yaml -validate
```

### Environment Variables Not Expanded

**Symptom**: Literal "${VAR}" in config

**Solutions**:

```bash
# Verify variable is set
echo $KAFKA_BROKER

# Export before running
export KAFKA_BROKER=kafka:9092
./bin/logaggregator -config config.yaml

# Check expanded config
cat config.yaml | envsubst
```

## Output Issues

### Kafka Connection Failed

**Symptom**: "failed to connect to Kafka" error

**Diagnosis**:

```bash
# Test connectivity
telnet kafka-broker 9092

# Check DNS
nslookup kafka-broker

# Check broker logs
kafka-server-logs.sh
```

**Solutions**:

1. **Verify brokers**:
```yaml
output:
  kafka:
    brokers:
      - correct-broker:9092
```

2. **Check authentication**:
```yaml
output:
  kafka:
    sasl_enabled: true
    sasl_username: ${KAFKA_USER}
    sasl_password: ${KAFKA_PASS}
```

3. **Enable TLS**:
```yaml
output:
  kafka:
    enable_tls: true
```

### Elasticsearch Bulk Failed

**Symptom**: "bulk insert failed" error

**Diagnosis**:

```bash
# Check ES health
curl http://elasticsearch:9200/_cluster/health

# Check index
curl http://elasticsearch:9200/logs-*/_search?size=1

# Check ES logs
docker logs elasticsearch
```

**Solutions**:

1. **Check credentials**:
```yaml
output:
  elasticsearch:
    username: elastic
    password: ${ES_PASSWORD}
```

2. **Reduce batch size**:
```yaml
output:
  elasticsearch:
    batch_size: 500
```

3. **Check index template**:
```bash
curl -X PUT http://elasticsearch:9200/_index_template/logs
```

### S3 Upload Failed

**Symptom**: "failed to upload to S3" error

**Diagnosis**:

```bash
# Check AWS credentials
aws sts get-caller-identity

# Check bucket exists
aws s3 ls s3://my-bucket

# Check permissions
aws s3api get-bucket-acl --bucket my-bucket
```

**Solutions**:

1. **Verify credentials**:
```bash
export AWS_ACCESS_KEY_ID=xxx
export AWS_SECRET_ACCESS_KEY=xxx
export AWS_REGION=us-east-1
```

2. **Check bucket config**:
```yaml
output:
  s3:
    bucket: correct-bucket-name
    region: us-east-1
```

## Kubernetes Issues

### Pods Not Starting

**Symptom**: Pods in CrashLoopBackOff

**Diagnosis**:

```bash
# Check pod status
kubectl get pods -n logging

# Check pod events
kubectl describe pod logaggregator-xxx -n logging

# Check logs
kubectl logs logaggregator-xxx -n logging
```

**Solutions**:

1. **Check resource limits**:
```yaml
resources:
  requests:
    memory: 512Mi  # Increase if needed
```

2. **Check config**:
```bash
kubectl get configmap logaggregator-config -n logging -o yaml
```

3. **Check secrets**:
```bash
kubectl get secrets -n logging
```

### Insufficient Permissions

**Symptom**: "forbidden" errors for Kubernetes API

**Diagnosis**:

```bash
# Check ServiceAccount
kubectl get sa logaggregator -n logging

# Check RoleBinding
kubectl get clusterrolebinding logaggregator
```

**Solutions**:

```bash
# Apply RBAC
kubectl apply -f deploy/kubernetes/serviceaccount.yaml
```

### High Pod Restart Count

**Symptom**: Pods restarting frequently

**Diagnosis**:

```bash
# Check restart reason
kubectl describe pod logaggregator-xxx -n logging

# Check resource usage
kubectl top pod logaggregator-xxx -n logging

# Check logs before restart
kubectl logs logaggregator-xxx -n logging --previous
```

**Solutions**:

1. **Increase resource limits**
2. **Fix health check configuration**
3. **Check for OOMKilled**

## Getting Help

If you can't resolve your issue:

1. **Check logs**: Increase log level to `debug`
2. **Collect metrics**: Export Prometheus metrics
3. **Capture profiles**: CPU and memory profiles
4. **Report issue**: Create GitHub issue with:
   - Version
   - Configuration
   - Logs
   - Metrics
   - Steps to reproduce

## Useful Commands

```bash
# Health check
curl http://localhost:8081/health

# Metrics
curl http://localhost:9090/metrics

# Stats
curl http://localhost:6060/debug/stats

# Config validation
./bin/logaggregator -config config.yaml -validate

# Version
./bin/logaggregator -version
```
