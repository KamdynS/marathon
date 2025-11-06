# Marathon

> Temporal-inspired durable workflow orchestration for AI agents in Go

A focused, production-ready framework for building resilient AI workflows with full state persistence, automatic retries, and distributed execution.

## Overview

Marathon provides a durable execution engine specifically designed for AI workloads. Unlike Temporal, which handles all types of workflows, Marathon is optimized for:

- **Long-running LLM calls** that need retry logic
- **Agentic loops** with complex reasoning chains
- **Multi-agent systems** with parallel execution
- **Production AI applications** requiring observability

This library is written to be hosted entirely within your own infra, with an emphasis on enhancing traditional web server architectures. The goal is to have a way to rigth traditional backend systems in Go, without being burdened by a specific infrastructure framework. Use EC2 and Lambda, your own Kubernetes cluster, or a simple hetzner box. More details can be found in [USAGE_PATTERN](../USAGE_PATTERN.md)

## Contributing

Contributions welcome! This is a new project, so there's plenty to build.
I've written thoughts about the direction I want this project to take in [CONTRIBUTING](../docs/CONTRIBUTING.md)

### Testing

- We prefer table-driven tests and target 80%+ coverage across the repo.
- Quick start:

```bash
go test ./... -cover
go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out -o coverage.html
```

### Optional adapters and build tags

- By default, Marathon builds without external adapters to keep dependencies light.
- To enable adapters, build with tags:

```bash
# Redis store and SQS queue
go build -tags adapters_redis,adapters_sqs ./...
```

- SQS adapter requires AWS SDK v2 and is behind the `adapters_sqs` tag. To run its integration tests:

```bash
# With Localstack
LOCALSTACK_URL=http://localhost:4566 go test ./adapters/sqs -tags adapters_sqs -v

# Or with AWS (provide your queue URL/region/credentials)
SQS_QUEUE_URL="https://sqs.${AWS_REGION}.amazonaws.com/<acct>/<queue>" \
AWS_REGION="${AWS_REGION}" \
go test ./adapters/sqs -tags adapters_sqs -v
```

## License

Apache License 2.0 - see [LICENSE](../LICENSE) for details.

## Support

- Issues: [GitHub Issues](https://github.com/KamdynS/marathon/issues)
- Docs: [docs/](../docs/)

