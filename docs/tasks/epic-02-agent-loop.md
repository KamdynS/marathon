## Epic 02: Agent Loop (Full)

### Purpose
Ship a complete durable agent loop with tools, streaming, retries, and clear stop conditions. This is a first-class feature before external adapters.

### Scope
- Default Agent implementation in `agent` package using existing interfaces:
  - Iterative loop: plan → act (tool/function) → observe/reflect → repeat.
  - Tool execution via `tools.Registry`; JSON schema/function calling shape.
  - Configurable max iterations and stop criteria; `SystemPrompt` support.
  - Middleware hooks around LLM and tool calls.
  - Memory processing hook to prune/summarize history.
- Streaming:
  - `RunStream(ctx, input, out chan<- Message)` emits incremental assistant/tool messages.
  - SSE-friendly: emit events per iteration and token-chunks (when available).
- Durability:
  - Each iteration is a workflow activity; retries with backoff; idempotent by `iteration_id`.
  - Resume after crash continues from last completed iteration.

### Acceptance Criteria
- Loop executes with registered tools and LLM to reach terminal output or stop condition.
- Streaming API produces incremental messages; consumers can render progressively.
- Cancellation via workflow cancel stops the loop promptly (respect context).
- Retries: transient LLM/tool failures are retried per policy without duplicating effects.
- Idempotency: re-running the same iteration id does not duplicate tool side-effects.
- Tests cover: tool error/retry; LLM error; cancel mid-iteration; resume after crash; max-iteration stop.

### Tech Notes
- Model each iteration as an `activity` with inputs: history slice, tool choice, iteration_id, config.
- Store conversation history in workflow state; only append on successful iteration completion.
- Provide simple JSON schema for tool calls (name, arguments) compatible with LLM router.


