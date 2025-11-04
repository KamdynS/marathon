# Marathon Docker Deployment

Deploy the Marathon durable workflow system using Docker and Docker Compose.

## Quick Start

```bash
# Build and start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

## Services

The Docker Compose setup includes:

- **Redis**: State store for workflow persistence
- **API Server**: HTTP API for workflow management (port 8080)
- **Workers**: Pool of 3 workers processing activities

## Accessing the API

Once running, the API is available at `http://localhost:8080`:

```bash
# Health check
curl http://localhost:8080/health

# Start a workflow
curl -X POST http://localhost:8080/workflows \
  -H "Content-Type: application/json" \
  -d '{"workflow_name": "greeting-workflow", "input": null}'

# Get workflow status
curl http://localhost:8080/workflows/{workflow-id}
```

## Scaling Workers

To scale the number of workers:

```bash
docker-compose up -d --scale worker=5
```

## Production Considerations

For production deployments:

1. **Persistent Storage**: The Redis data is persisted in a Docker volume
2. **Environment Variables**: Configure via `.env` file or environment
3. **Monitoring**: Add Prometheus/Grafana for observability
4. **Secrets**: Use Docker secrets or external secret management
5. **Networking**: Configure reverse proxy (nginx/traefik) for SSL/TLS

## Customization

### Using Your Own Application

Replace the Dockerfile to build your custom application:

```dockerfile
# Build your app instead of the example
RUN CGO_ENABLED=0 GOOS=linux go build -o /workflow-app ./cmd/your-app
```

### Environment Variables

Common environment variables:

- `REDIS_ADDR`: Redis connection address (default: redis:6379)
- `QUEUE_NAME`: Task queue name (default: default)
- `MAX_CONCURRENT`: Max concurrent activities per worker (default: 5)
- `PORT`: API server port (default: 8080)

## Troubleshooting

### Check service status
```bash
docker-compose ps
```

### View logs for a specific service
```bash
docker-compose logs -f api-server
docker-compose logs -f worker
docker-compose logs -f redis
```

### Restart a service
```bash
docker-compose restart worker
```

### Clean up everything
```bash
docker-compose down -v  # -v removes volumes
```

