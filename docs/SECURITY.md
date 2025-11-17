# Security Guide

Security best practices and guidelines for logaggregator deployment.

## Table of Contents

- [Security Features](#security-features)
- [Authentication](#authentication)
- [Encryption](#encryption)
- [Access Control](#access-control)
- [Secret Management](#secret-management)
- [Network Security](#network-security)
- [Container Security](#container-security)
- [Kubernetes Security](#kubernetes-security)
- [Audit Logging](#audit-logging)
- [Compliance](#compliance)

## Security Features

### Built-in Security

1. **TLS/SSL Support**: All network communication can be encrypted
2. **Authentication**: API keys, SASL, basic auth
3. **Authorization**: RBAC for Kubernetes
4. **Input Validation**: Prevent injection attacks
5. **Rate Limiting**: DoS protection
6. **Sensitive Data Redaction**: Automatic PII filtering
7. **Secret Management**: Multiple secret sources
8. **Non-root Execution**: Runs as unprivileged user

## Authentication

### HTTP Input Authentication

Use API keys for HTTP input:

```yaml
inputs:
  http:
    - name: api
      address: "0.0.0.0:8080"
      api_keys:
        - "${HTTP_API_KEY_1}"
        - "${HTTP_API_KEY_2}"
```

Send requests with API key:

```bash
curl -X POST http://localhost:8080/log \
  -H "X-API-Key: your-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"message": "Test"}'
```

### Kafka Authentication

SASL authentication:

```yaml
output:
  kafka:
    brokers: ["kafka:9092"]
    topic: logs
    sasl_enabled: true
    sasl_mechanism: SCRAM-SHA-256
    sasl_username: "${KAFKA_USERNAME}"
    sasl_password: "${KAFKA_PASSWORD}"
    enable_tls: true
```

Supported SASL mechanisms:
- PLAIN
- SCRAM-SHA-256
- SCRAM-SHA-512

### Elasticsearch Authentication

Basic authentication:

```yaml
output:
  elasticsearch:
    addresses: ["https://es:9200"]
    username: "${ES_USERNAME}"
    password: "${ES_PASSWORD}"
```

API key authentication:

```yaml
output:
  elasticsearch:
    addresses: ["https://es:9200"]
    api_key: "${ES_API_KEY}"
```

### AWS Authentication

Use IAM roles:

```yaml
output:
  s3:
    bucket: logs
    region: us-east-1
    # IAM role attached to pod
```

Or credentials:

```bash
export AWS_ACCESS_KEY_ID=xxx
export AWS_SECRET_ACCESS_KEY=xxx
```

## Encryption

### TLS Configuration

Enable TLS for all network communication:

#### Syslog TLS

```yaml
inputs:
  syslog:
    - name: secure-syslog
      protocol: tcp
      address: "0.0.0.0:6514"
      tls_enabled: true
      tls_cert: /etc/certs/server.crt
      tls_key: /etc/certs/server.key
```

#### HTTP TLS

```yaml
inputs:
  http:
    - name: secure-http
      address: "0.0.0.0:8443"
      tls_enabled: true
      tls_cert: /etc/certs/server.crt
      tls_key: /etc/certs/server.key
```

#### Kafka TLS

```yaml
output:
  kafka:
    brokers: ["kafka:9093"]
    enable_tls: true
```

#### Elasticsearch TLS

```yaml
output:
  elasticsearch:
    addresses: ["https://es:9200"]
    # TLS automatically enabled with https
```

### Certificate Management

#### Self-signed Certificates

Generate for development:

```bash
# Generate CA
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 365 -key ca.key -out ca.crt

# Generate server certificate
openssl genrsa -out server.key 4096
openssl req -new -key server.key -out server.csr
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -set_serial 01 -out server.crt
```

#### Production Certificates

Use proper CA (Let's Encrypt, cert-manager):

```yaml
# With cert-manager
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: logaggregator-tls
spec:
  secretName: logaggregator-tls
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
    - logaggregator.example.com
```

## Access Control

### Kubernetes RBAC

Minimal permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: logaggregator
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list"]
```

### Network Policies

Restrict network access:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: logaggregator
spec:
  podSelector:
    matchLabels:
      app: logaggregator
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: logging
      ports:
        - protocol: TCP
          port: 8080
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              name: kafka
      ports:
        - protocol: TCP
          port: 9092
```

### Pod Security

Pod Security Standards:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: logaggregator
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    fsGroup: 1000
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: logaggregator
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop:
            - ALL
        readOnlyRootFilesystem: true
```

## Secret Management

### Environment Variables

Store secrets in environment variables:

```bash
export KAFKA_PASSWORD="secret"
export ES_API_KEY="secret"
```

Reference in config:

```yaml
output:
  kafka:
    sasl_password: "${KAFKA_PASSWORD}"
```

### Kubernetes Secrets

Create secret:

```bash
kubectl create secret generic logaggregator-secrets \
  --from-literal=kafka-password=secret \
  --from-literal=es-api-key=secret \
  -n logging
```

Mount in pod:

```yaml
env:
  - name: KAFKA_PASSWORD
    valueFrom:
      secretKeyRef:
        name: logaggregator-secrets
        key: kafka-password
```

### File-based Secrets

Store secrets in files:

```bash
echo "secret" > /run/secrets/kafka-password
chmod 600 /run/secrets/kafka-password
```

Reference in config:

```yaml
output:
  kafka:
    sasl_password: "file:/run/secrets/kafka-password"
```

### HashiCorp Vault

Integration example:

```yaml
env:
  - name: KAFKA_PASSWORD
    valueFrom:
      secretKeyRef:
        name: vault-secrets
        key: kafka-password
```

### AWS Secrets Manager

Use AWS Secrets Manager:

```yaml
# With external-secrets operator
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: aws-secrets
spec:
  provider:
    aws:
      service: SecretsManager
      region: us-east-1
```

## Network Security

### Firewall Rules

Allow only necessary ports:

```bash
# Allow syslog
iptables -A INPUT -p udp --dport 514 -j ACCEPT
iptables -A INPUT -p tcp --dport 514 -j ACCEPT

# Allow HTTP input
iptables -A INPUT -p tcp --dport 8080 -j ACCEPT

# Allow metrics
iptables -A INPUT -p tcp --dport 9090 -j ACCEPT

# Drop others
iptables -A INPUT -j DROP
```

### Rate Limiting

Configure rate limits:

```yaml
inputs:
  http:
    - name: api
      rate_limit: 1000  # requests per second

  syslog:
    - name: syslog
      rate_limit: 10000  # messages per second
```

### IP Whitelisting

Use network policies or firewall rules:

```bash
# Allow from specific subnet
iptables -A INPUT -p tcp --dport 8080 -s 10.0.0.0/8 -j ACCEPT
iptables -A INPUT -p tcp --dport 8080 -j DROP
```

## Container Security

### Image Security

Scan images for vulnerabilities:

```bash
# Scan with Trivy
trivy image logaggregator:latest

# Scan with Snyk
snyk container test logaggregator:latest
```

### Non-root User

Run as non-root user:

```dockerfile
# In Dockerfile
USER 1000:1000
```

```yaml
# In Kubernetes
securityContext:
  runAsUser: 1000
  runAsNonRoot: true
```

### Read-only Filesystem

Use read-only root filesystem:

```yaml
securityContext:
  readOnlyRootFilesystem: true
```

Mount writable volumes for data:

```yaml
volumeMounts:
  - name: data
    mountPath: /var/lib/logaggregator
  - name: tmp
    mountPath: /tmp
```

### Resource Limits

Prevent resource exhaustion:

```yaml
resources:
  limits:
    cpu: 2000m
    memory: 2Gi
  requests:
    cpu: 500m
    memory: 512Mi
```

## Kubernetes Security

### Service Account

Use dedicated service account:

```yaml
serviceAccountName: logaggregator
automountServiceAccountToken: true
```

### Pod Security Policies

Apply Pod Security Standards:

```yaml
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: logaggregator
spec:
  privileged: false
  allowPrivilegeEscalation: false
  runAsUser:
    rule: MustRunAsNonRoot
  fsGroup:
    rule: RunAsAny
  volumes:
    - configMap
    - secret
    - emptyDir
    - persistentVolumeClaim
```

## Audit Logging

### Security Events

Log security-relevant events:

```go
logger.Warn().
    Str("ip", clientIP).
    Str("action", "authentication_failed").
    Msg("Failed authentication attempt")
```

### Audit Configuration

Enable audit logging:

```yaml
logging:
  level: info
  format: json
  audit:
    enabled: true
    path: /var/log/audit.log
```

## Compliance

### PCI DSS

For PCI DSS compliance:
- Encrypt all network communication (TLS)
- Use strong authentication
- Redact sensitive fields (credit card numbers)
- Enable audit logging
- Implement access controls

### HIPAA

For HIPAA compliance:
- Encrypt data in transit and at rest
- Implement access controls
- Enable audit logging
- Redact PHI
- Use secure secret management

### GDPR

For GDPR compliance:
- Redact PII
- Implement data retention policies
- Enable audit logging
- Provide data deletion capability
- Implement access controls

## Security Checklist

- [ ] TLS enabled for all network communication
- [ ] Strong authentication configured
- [ ] Secrets stored securely (not in config files)
- [ ] Running as non-root user
- [ ] Resource limits configured
- [ ] Network policies applied
- [ ] RBAC configured with minimal permissions
- [ ] Read-only root filesystem
- [ ] Sensitive fields redacted
- [ ] Rate limiting enabled
- [ ] Audit logging enabled
- [ ] Container images scanned for vulnerabilities
- [ ] Pod security policies applied
- [ ] Regular security updates

## Security Reporting

Report security vulnerabilities to:
security@example.com

Use PGP key: [KEY_ID]
