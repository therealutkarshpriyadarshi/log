# Deployment Guide

This guide covers deploying logaggregator in various environments.

## Table of Contents

- [Docker Deployment](#docker-deployment)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Helm Deployment](#helm-deployment)
- [Binary Deployment](#binary-deployment)
- [Configuration](#configuration)
- [High Availability](#high-availability)

## Docker Deployment

### Build Image

```bash
# Build from source
docker build -t logaggregator:latest .

# Or pull from registry
docker pull logaggregator:latest
```

### Run Container

```bash
docker run -d \
  --name logaggregator \
  -p 8080:8080 \
  -p 514:514/udp \
  -p 9090:9090 \
  -p 8081:8081 \
  -v /var/log:/var/log:ro \
  -v ./config.yaml:/etc/logaggregator/config.yaml:ro \
  -v logaggregator-data:/var/lib/logaggregator \
  logaggregator:latest
```

### Docker Compose

```yaml
version: '3.8'

services:
  logaggregator:
    image: logaggregator:latest
    ports:
      - "8080:8080"
      - "514:514/udp"
      - "9090:9090"
      - "8081:8081"
    volumes:
      - /var/log:/var/log:ro
      - ./config.yaml:/etc/logaggregator/config.yaml:ro
      - logaggregator-checkpoints:/var/lib/logaggregator/checkpoints
      - logaggregator-wal:/var/lib/logaggregator/wal
    environment:
      - LOG_LEVEL=info
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8081/health"]
      interval: 30s
      timeout: 5s
      retries: 3

volumes:
  logaggregator-checkpoints:
  logaggregator-wal:
```

## Kubernetes Deployment

### Prerequisites

- Kubernetes 1.19+
- kubectl configured
- Sufficient cluster resources

### Quick Start

```bash
# Create namespace
kubectl create namespace logging

# Apply manifests
kubectl apply -f deploy/kubernetes/namespace.yaml
kubectl apply -f deploy/kubernetes/serviceaccount.yaml
kubectl apply -f deploy/kubernetes/configmap.yaml
kubectl apply -f deploy/kubernetes/deployment.yaml
kubectl apply -f deploy/kubernetes/service.yaml

# Check status
kubectl get pods -n logging
kubectl logs -n logging deployment/logaggregator
```

### Deployment Types

#### 1. Centralized Deployment

Use for centralized log collection (HTTP, Syslog inputs):

```bash
kubectl apply -f deploy/kubernetes/deployment.yaml
```

Features:
- Multiple replicas for high availability
- Load-balanced service
- Persistent storage for checkpoints
- Suitable for receiving logs from external sources

#### 2. DaemonSet Deployment

Use for node-level log collection:

```bash
kubectl apply -f deploy/kubernetes/daemonset.yaml
```

Features:
- One pod per node
- Direct access to node logs
- Suitable for container log collection
- Lower network overhead

#### 3. Hybrid Deployment

Use both for comprehensive coverage:

```bash
kubectl apply -f deploy/kubernetes/deployment.yaml
kubectl apply -f deploy/kubernetes/daemonset.yaml
```

### Configuration

Edit the ConfigMap:

```bash
kubectl edit configmap logaggregator-config -n logging
```

Or create from file:

```bash
kubectl create configmap logaggregator-config \
  --from-file=config.yaml=./my-config.yaml \
  -n logging \
  --dry-run=client -o yaml | kubectl apply -f -
```

### Scaling

```bash
# Scale deployment
kubectl scale deployment logaggregator -n logging --replicas=5

# Auto-scaling
kubectl autoscale deployment logaggregator -n logging \
  --min=3 --max=10 --cpu-percent=80
```

### Monitoring

```bash
# Check metrics
kubectl port-forward -n logging service/logaggregator 9090:9090
curl http://localhost:9090/metrics

# Check health
kubectl port-forward -n logging service/logaggregator 8081:8081
curl http://localhost:8081/health
```

## Helm Deployment

### Install Helm Chart

```bash
# Install with default values
helm install logaggregator ./deploy/helm/logaggregator \
  --namespace logging \
  --create-namespace

# Install with custom values
helm install logaggregator ./deploy/helm/logaggregator \
  --namespace logging \
  --create-namespace \
  --values my-values.yaml

# Install as DaemonSet
helm install logaggregator ./deploy/helm/logaggregator \
  --namespace logging \
  --create-namespace \
  --set deploymentType=daemonset \
  --set daemonset.enabled=true
```

### Custom Values Example

Create `my-values.yaml`:

```yaml
replicaCount: 5

resources:
  requests:
    cpu: 1000m
    memory: 1Gi
  limits:
    cpu: 4000m
    memory: 4Gi

persistence:
  checkpoints:
    enabled: true
    size: 10Gi
  wal:
    enabled: true
    size: 50Gi

config:
  output:
    type: kafka
    kafka:
      brokers:
        - kafka-1:9092
        - kafka-2:9092
      topic: logs
      compression_codec: snappy

metrics:
  enabled: true
  serviceMonitor:
    enabled: true
```

Install:

```bash
helm install logaggregator ./deploy/helm/logaggregator \
  --namespace logging \
  --create-namespace \
  -f my-values.yaml
```

### Upgrade

```bash
# Upgrade with new values
helm upgrade logaggregator ./deploy/helm/logaggregator \
  --namespace logging \
  -f my-values.yaml

# Rollback
helm rollback logaggregator 1 --namespace logging
```

### Uninstall

```bash
helm uninstall logaggregator --namespace logging
```

## Binary Deployment

### Build from Source

```bash
# Clone repository
git clone https://github.com/therealutkarshpriyadarshi/log.git
cd log

# Build binary
make build

# Binary will be in ./bin/logaggregator
```

### Install

```bash
# Copy binary
sudo cp bin/logaggregator /usr/local/bin/

# Create directories
sudo mkdir -p /var/lib/logaggregator/{checkpoints,wal,dlq}
sudo mkdir -p /etc/logaggregator

# Create user
sudo useradd -r -s /bin/false logaggregator
sudo chown -R logaggregator:logaggregator /var/lib/logaggregator

# Copy config
sudo cp config.yaml.example /etc/logaggregator/config.yaml
sudo chown logaggregator:logaggregator /etc/logaggregator/config.yaml
```

### Systemd Service

Create `/etc/systemd/system/logaggregator.service`:

```ini
[Unit]
Description=Log Aggregator
After=network.target

[Service]
Type=simple
User=logaggregator
Group=logaggregator
ExecStart=/usr/local/bin/logaggregator -config /etc/logaggregator/config.yaml
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=logaggregator

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/logaggregator

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable logaggregator
sudo systemctl start logaggregator
sudo systemctl status logaggregator
```

View logs:

```bash
sudo journalctl -u logaggregator -f
```

## Configuration

### Minimal Configuration

```yaml
inputs:
  files:
    - paths:
        - /var/log/app/*.log

logging:
  level: info
  format: json

output:
  type: stdout
```

### Production Configuration

```yaml
inputs:
  files:
    - paths:
        - /var/log/app/*.log
      checkpoint_path: /var/lib/logaggregator/checkpoints
      checkpoint_interval: 5s
      parser:
        type: json

buffer:
  type: memory
  size: 524288
  backpressure_strategy: drop

worker_pool:
  num_workers: 8

wal:
  enabled: true
  dir: /var/lib/logaggregator/wal

dead_letter:
  enabled: true
  dir: /var/lib/logaggregator/dlq

metrics:
  enabled: true
  address: "0.0.0.0:9090"

health:
  enabled: true
  address: "0.0.0.0:8081"

output:
  type: kafka
  kafka:
    brokers:
      - kafka-1:9092
      - kafka-2:9092
    topic: logs
    compression_codec: snappy
    enable_tls: true
```

### Environment Variables

Use environment variables in config:

```yaml
output:
  type: kafka
  kafka:
    brokers:
      - ${KAFKA_BROKER_1}
      - ${KAFKA_BROKER_2}
    topic: ${KAFKA_TOPIC}
    sasl_username: ${KAFKA_USERNAME}
    sasl_password: ${KAFKA_PASSWORD}
```

Set variables:

```bash
export KAFKA_BROKER_1=kafka-1:9092
export KAFKA_BROKER_2=kafka-2:9092
export KAFKA_TOPIC=logs
export KAFKA_USERNAME=user
export KAFKA_PASSWORD=password
```

## High Availability

### Load Balancing

Use Kubernetes Service:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: logaggregator
spec:
  type: LoadBalancer
  selector:
    app: logaggregator
  ports:
    - port: 8080
      name: http
    - port: 514
      name: syslog
```

### Persistence

Use PersistentVolumeClaims:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: logaggregator-checkpoints
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
```

### Monitoring

Use Prometheus and Grafana:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: logaggregator
spec:
  selector:
    matchLabels:
      app: logaggregator
  endpoints:
    - port: metrics
      interval: 30s
```

### Backup and Recovery

```bash
# Backup checkpoints
kubectl cp logging/logaggregator-pod:/var/lib/logaggregator/checkpoints ./backup/

# Restore checkpoints
kubectl cp ./backup/ logging/logaggregator-pod:/var/lib/logaggregator/checkpoints
```

## Troubleshooting

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common issues and solutions.

## Next Steps

- Configure outputs: [Configuration Reference](CONFIGURATION.md)
- Monitor performance: [Performance Tuning](PERFORMANCE_TUNING.md)
- Set up alerts: [Monitoring Guide](MONITORING.md)
