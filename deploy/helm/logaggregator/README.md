# Logaggregator Helm Chart

A Helm chart for deploying the logaggregator log collection and aggregation system on Kubernetes.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+

## Installing the Chart

```bash
# Add the repository (if published)
helm repo add logaggregator https://example.com/charts
helm repo update

# Install the chart
helm install my-logaggregator logaggregator/logaggregator

# Or install from local directory
helm install my-logaggregator ./deploy/helm/logaggregator
```

## Uninstalling the Chart

```bash
helm uninstall my-logaggregator
```

## Configuration

The following table lists the configurable parameters of the logaggregator chart and their default values.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `deploymentType` | Deployment type (deployment or daemonset) | `deployment` |
| `image.repository` | Image repository | `logaggregator` |
| `image.tag` | Image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `replicaCount` | Number of replicas (deployment mode) | `3` |
| `resources.requests.cpu` | CPU request | `500m` |
| `resources.requests.memory` | Memory request | `512Mi` |
| `resources.limits.cpu` | CPU limit | `2000m` |
| `resources.limits.memory` | Memory limit | `2Gi` |
| `service.type` | Service type | `ClusterIP` |
| `persistence.checkpoints.enabled` | Enable checkpoint persistence | `true` |
| `persistence.wal.enabled` | Enable WAL persistence | `true` |
| `metrics.enabled` | Enable Prometheus metrics | `true` |

See `values.yaml` for the complete list of configuration options.

## Examples

### Install as Deployment

```bash
helm install my-logaggregator ./deploy/helm/logaggregator \
  --set deploymentType=deployment \
  --set replicaCount=3
```

### Install as DaemonSet

```bash
helm install my-logaggregator ./deploy/helm/logaggregator \
  --set deploymentType=daemonset \
  --set daemonset.enabled=true
```

### Custom Configuration

Create a `custom-values.yaml`:

```yaml
replicaCount: 5

resources:
  requests:
    cpu: 1000m
    memory: 1Gi
  limits:
    cpu: 4000m
    memory: 4Gi

config:
  output:
    type: kafka
    kafka:
      brokers:
        - kafka-1:9092
        - kafka-2:9092
      topic: logs
```

Install with custom values:

```bash
helm install my-logaggregator ./deploy/helm/logaggregator -f custom-values.yaml
```

## Monitoring

The chart exposes Prometheus metrics on port 9090 by default. To enable ServiceMonitor:

```bash
helm install my-logaggregator ./deploy/helm/logaggregator \
  --set metrics.serviceMonitor.enabled=true
```
