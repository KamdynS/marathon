# Marathon Kubernetes Deployment

Deploy the Marathon durable workflow system to Kubernetes.

## Prerequisites

- Kubernetes cluster (v1.20+)
- kubectl configured
- Container registry access

## Quick Start

```bash
# Apply all manifests
kubectl apply -f deployment.yaml

# Check status
kubectl get pods -n workflows

# Get API service endpoint
kubectl get svc workflow-api-service -n workflows
```

## Architecture

The deployment includes:

- **Redis**: Single-instance state store with persistent volume
- **API Server**: 2 replicas with load balancer
- **Workers**: 3-10 replicas with horizontal pod autoscaling

## Components

### Redis

- Single replica with persistent storage
- 10Gi persistent volume claim
- Service exposed internally

### API Server

- 2 replicas for high availability
- LoadBalancer service (port 80)
- Health checks configured
- Resource limits: 256Mi memory, 200m CPU

### Workers

- 3-10 replicas (auto-scales)
- HPA based on CPU utilization (70%)
- Resource limits: 512Mi memory, 500m CPU

## Customization

### Update Image

Edit `deployment.yaml` and change:
```yaml
image: your-registry/workflow-app:latest
```

Build and push your image:
```bash
docker build -t your-registry/workflow-app:latest -f deploy/docker/Dockerfile .
docker push your-registry/workflow-app:latest
```

### Scaling

#### Manual Scaling
```bash
# Scale workers
kubectl scale deployment workflow-worker -n workflows --replicas=5

# Scale API servers
kubectl scale deployment workflow-api -n workflows --replicas=3
```

#### Auto-scaling Configuration

Edit the HPA in `deployment.yaml`:
```yaml
spec:
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

### Environment Variables

Update the ConfigMap:
```bash
kubectl edit configmap workflow-config -n workflows
```

Or edit `deployment.yaml`:
```yaml
data:
  REDIS_ADDR: "redis-service:6379"
  QUEUE_NAME: "default"
  MAX_CONCURRENT: "10"
```

## Monitoring

### View Logs

```bash
# API server logs
kubectl logs -f -l app=workflow-api -n workflows

# Worker logs
kubectl logs -f -l app=workflow-worker -n workflows

# Redis logs
kubectl logs -f -l app=redis -n workflows
```

### Metrics

Add Prometheus monitoring:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: workflow-api-metrics
  namespace: workflows
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"
    prometheus.io/path: "/metrics"
spec:
  selector:
    app: workflow-api
  ports:
  - port: 8080
```

## Production Considerations

### 1. Persistent Storage

For production, use a storage class appropriate for your cloud provider:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: redis-pvc
spec:
  storageClassName: fast-ssd  # Your storage class
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 100Gi
```

### 2. Redis High Availability

For production, consider Redis Sentinel or Redis Cluster:

```bash
# Using Bitnami Helm chart
helm install redis bitnami/redis \
  --namespace workflows \
  --set sentinel.enabled=true \
  --set master.persistence.size=100Gi
```

### 3. Ingress

Add an Ingress for SSL/TLS:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: workflow-ingress
  namespace: workflows
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts:
    - workflows.your-domain.com
    secretName: workflow-tls
  rules:
  - host: workflows.your-domain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: workflow-api-service
            port:
              number: 80
```

### 4. Resource Tuning

Adjust based on your workload:

```yaml
resources:
  requests:
    memory: "512Mi"
    cpu: "500m"
  limits:
    memory: "1Gi"
    cpu: "1000m"
```

### 5. Security

- Use NetworkPolicies to restrict traffic
- Enable RBAC
- Use Pod Security Standards
- Store secrets in Kubernetes Secrets or external vault

## Troubleshooting

### Pods not starting

```bash
kubectl describe pod <pod-name> -n workflows
kubectl logs <pod-name> -n workflows
```

### Redis connection issues

```bash
# Test Redis connection from a pod
kubectl run -it --rm redis-test --image=redis:7-alpine -n workflows -- redis-cli -h redis-service -p 6379 ping
```

### HPA not scaling

```bash
# Check HPA status
kubectl get hpa -n workflows
kubectl describe hpa workflow-worker-hpa -n workflows

# Check metrics server
kubectl top nodes
kubectl top pods -n workflows
```

## Cleanup

```bash
# Delete all resources
kubectl delete namespace workflows
```

