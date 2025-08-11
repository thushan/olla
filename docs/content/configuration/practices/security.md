---
title: "Olla Security Best Practices - Production Security Guide"
description: "Comprehensive security guide for Olla deployments. Network security, rate limiting, TLS configuration, access control, and security monitoring for production."
keywords: ["olla security", "security best practices", "rate limiting", "network security", "access control", "production security"]
---

# Security Best Practices

This guide covers security considerations and best practices for deploying Olla in production environments.

> :memo: **Default Security Configuration**
> ```yaml
> server:
>   rate_limits:
>     global_requests_per_minute: 1000
>     per_ip_requests_per_minute: 100
>   request_limits:
>     max_body_size: 52428800  # 50MB
>     max_header_size: 524288   # 512KB
> ```
> **Key Settings**:
> 
> - Rate limiting enabled by default
> - Request size limits prevent abuse
> - No authentication built-in (use reverse proxy)
> 
> **Environment Variables**: `OLLA_SERVER_RATE_LIMITS_*`

## Security Principles

Olla follows these security principles:

1. **Defence in Depth**: Multiple layers of security controls
2. **Least Privilege**: Minimal permissions required
3. **Fail Secure**: Safe defaults when errors occur
4. **Zero Trust**: Verify all requests

## Network Security

### Bind Address Configuration

Control network exposure carefully:

```yaml
# Development - local only
server:
  host: "localhost"  # Only local connections
  port: 40114

# Production - controlled exposure
server:
  host: "0.0.0.0"   # Accept from network
  port: 40114
```

**Recommendations**:

- Use `localhost` unless network access is required
- Deploy behind a reverse proxy for internet exposure
- Use firewall rules to restrict access

### TLS/HTTPS Configuration

Olla doesn't handle TLS directly. Use a reverse proxy:

**nginx example**:
```nginx
server {
    listen 443 ssl http2;
    server_name api.example.com;
    
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    
    location / {
        proxy_pass http://localhost:40114;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

**Caddy example**:
```caddyfile
api.example.com {
    reverse_proxy localhost:40114
}
```

### Firewall Rules

Restrict access at the network level:

```bash
# Allow only from trusted networks
iptables -A INPUT -p tcp --dport 40114 -s 10.0.0.0/8 -j ACCEPT
iptables -A INPUT -p tcp --dport 40114 -j DROP

# Or use UFW
ufw allow from 10.0.0.0/8 to any port 40114
ufw deny 40114
```

## Rate Limiting

Protect against abuse and DoS attacks:

### Global Rate Limits

Prevent system overload:

```yaml
server:
  rate_limits:
    global_requests_per_minute: 5000  # Total system capacity
    health_requests_per_minute: 1000  # Monitoring endpoints
```

### Per-Client Rate Limits

Prevent single client abuse:

```yaml
server:
  rate_limits:
    per_ip_requests_per_minute: 60  # Strict for public APIs
    burst_size: 10                  # Small burst allowance
```

### Rate Limit Strategies

| Deployment Type | Global RPM | Per-IP RPM | Burst |
|----------------|------------|------------|-------|
| Internal API | 10000 | 1000 | 100 |
| Public API | 5000 | 60 | 10 |
| Development | 1000 | 100 | 50 |
| High Security | 1000 | 20 | 5 |

## Request Validation

### Size Limits

Prevent resource exhaustion attacks:

```yaml
server:
  request_limits:
    max_body_size: 10485760  # 10MB - adjust based on needs
    max_header_size: 131072  # 128KB - usually sufficient
```

**Guidelines**:

- Set limits based on legitimate use cases
- Smaller limits for public APIs
- Monitor rejected requests

### Input Validation

Olla validates:

- Request size limits
- Header size limits
- URL format and structure
- HTTP method restrictions

## Access Control

### Internal Endpoints

Restrict access to internal endpoints:

```yaml
# These endpoints should not be public:
# /internal/health
# /internal/status
# /internal/process
# /version
```

**nginx protection**:

```nginx
location /internal/ {
    allow 10.0.0.0/8;
    deny all;
    proxy_pass http://localhost:40114;
}
```

### Backend Endpoint Security

Secure your LLM endpoints:

```yaml
discovery:
  static:
    endpoints:
      # Use internal networks
      - url: "http://10.0.1.10:11434"  # Internal IP
        name: "internal-ollama"
        type: "ollama"
        
      # Avoid public endpoints when possible
      # If required, ensure they have authentication
```

## Deployment Security

### Container Security

When using Docker:

```dockerfile
# Run as non-root user
FROM golang:1.21-alpine AS builder
# ... build steps ...

FROM alpine:latest
RUN adduser -D -g '' appuser
USER appuser
COPY --from=builder /app/olla /usr/local/bin/
```

```yaml
# docker-compose.yml
services:
  olla:
    image: ghcr.io/thushan/olla:latest
    user: "1000:1000"  # Non-root UID/GID
    read_only: true     # Read-only filesystem
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
    cap_add:
      - NET_BIND_SERVICE  # Only if needed for low ports
```

### Kubernetes Security

Security policies for Kubernetes:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: olla
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    fsGroup: 1000
  containers:
  - name: olla
    image: ghcr.io/thushan/olla:latest
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop:
        - ALL
    resources:
      limits:
        memory: "512Mi"
        cpu: "1000m"
      requests:
        memory: "256Mi"
        cpu: "100m"
```

### Process Isolation

Run Olla with minimal privileges:

```bash
# systemd service
[Service]
User=olla
Group=olla
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
NoNewPrivileges=yes
ReadWritePaths=/var/log/olla
```

## Logging and Auditing

### Security Logging

Configure appropriate logging:

```yaml
server:
  request_logging: true  # Log all requests

logging:
  level: "info"
  format: "json"  # Structured logs for analysis
  output: "stdout"
```

### Log Sensitive Data

**Never log**:

- API keys or tokens
- Request/response bodies with sensitive data
- User credentials
- Internal IP addresses in public logs

### Audit Trail

Monitor these security events:

- Rate limit violations
- Request size limit violations
- Circuit breaker trips
- Failed health checks
- Configuration changes

## Secrets Management

### Configuration Files

Protect configuration files:

```bash
# Restrict file permissions
chmod 600 config.yaml
chown olla:olla config.yaml
```

### Environment Variables

Use environment variables for sensitive data:

```bash
# Instead of in config.yaml
export OLLA_SERVER_PORT=40114
```

## Security Monitoring

### Key Metrics

Monitor for security issues:

1. **Rate Limit Hits**: Potential abuse
2. **Error Rates**: Potential attacks
3. **Request Patterns**: Unusual activity
4. **Circuit Breaker**: Endpoint failures
5. **Response Times**: DoS indicators

### Alerting Thresholds

Set alerts for:

- Rate limit violations > 10/minute
- Error rate > 5%
- Circuit breaker trips > 3/hour
- Response time > 10s
- Memory usage > 80%

## Common Security Issues

### 1. Exposed Internal Endpoints

**Risk**: Information disclosure

**Mitigation**:
```nginx
location /internal/ {
    return 403;  # Or restrict by IP
}
```

### 2. No Rate Limiting

**Risk**: DoS attacks

**Mitigation**:
```yaml
server:
  rate_limits:
    global_requests_per_minute: 1000
    per_ip_requests_per_minute: 60
```

### 3. Large Request Acceptance

**Risk**: Resource exhaustion

**Mitigation**:
```yaml
server:
  request_limits:
    max_body_size: 5242880  # 5MB
```

### 4. Public Bind Address

**Risk**: Unintended exposure

**Mitigation**:
```yaml
server:
  host: "localhost"  # Local only
```

## Security Checklist

Production deployment checklist:

- [ ] Configure rate limiting
- [ ] Set request size limits
- [ ] Use reverse proxy with TLS
- [ ] Restrict bind address appropriately
- [ ] Configure firewall rules
- [ ] Run as non-root user
- [ ] Protect configuration files
- [ ] Enable request logging
- [ ] Monitor security metrics
- [ ] Regular security updates
- [ ] Implement log rotation
- [ ] Set up alerting
- [ ] Document security procedures

## Incident Response

### Rate Limit Violations

When detecting abuse:

1. Check logs for source IPs
2. Verify legitimate vs malicious traffic
3. Adjust rate limits if needed
4. Block malicious IPs at firewall
5. Document the incident

### Circuit Breaker Trips

When endpoints fail:

1. Check endpoint health directly
2. Review error logs
3. Verify network connectivity
4. Check for attacks on backends
5. Implement additional monitoring

## Compliance Considerations

### Data Protection

- Olla doesn't store request/response data
- Logs may contain metadata
- Configure log retention appropriately
- Implement log encryption if required

### Network Segmentation

- Deploy in appropriate network zones
- Use private networks for backend communication
- Implement network policies
- Regular security assessments

## Next Steps

- [Performance Best Practices](performance.md) - Optimise safely
- [Monitoring Guide](monitoring.md) - Security monitoring
- [Configuration Reference](../reference.md) - Security settings