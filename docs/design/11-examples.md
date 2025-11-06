## Example Projects (full microservices)

### Structure
```
examples/react-agent/
  cmd/api/main.go          # server mode
  cmd/worker/main.go       # worker mode
  internal/workflows/
  internal/activities/
  internal/agent/
  Dockerfile
  k8s/
    deployment-api.yaml
    job-worker.yaml (template)
    rbac.yaml
```

### ReAct Agent Example
- Agent plans → executes tool → reflects.
- Workflow orchestrates iterations with timeout and max-steps.

### Multi-Agent Example
- Researcher, Analyst, Writer in parallel; synthesize step.


