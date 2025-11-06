package agenthttp

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"
    "time"

    "github.com/KamdynS/marathon/state"
)

// EventsSinceGetter fetches workflow events since a given sequence.
type EventsSinceGetter func(ctx context.Context, workflowID string, since int64) ([]*state.Event, error)

// StreamEvents streams workflow events over SSE using the provided getter.
// - Respects Last-Event-ID for resume (pass empty string if none)
// - Sends heartbeat comments ": ping" at the provided heartbeat interval (default 15s)
// - Polls for new events at pollInterval (default 500ms)
// - Emits "event: done" and closes on terminal workflow events
func StreamEvents(
    ctx context.Context,
    w http.ResponseWriter,
    lastEventID string,
    getSince EventsSinceGetter,
    workflowID string,
    pollInterval time.Duration,
    heartbeatInterval time.Duration,
) error {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    flusher, ok := w.(http.Flusher)
    if !ok { return fmt.Errorf("stream unsupported") }

    // Parse Last-Event-ID if provided
    var since int64 = 0
    if lastEventID != "" {
        if v, err := strconv.ParseInt(lastEventID, 10, 64); err == nil {
            since = v
        }
    }

    if pollInterval <= 0 {
        pollInterval = 500 * time.Millisecond
    }
    tick := time.NewTicker(pollInterval)
    defer tick.Stop()
    if heartbeatInterval <= 0 { heartbeatInterval = 15 * time.Second }
    hb := time.NewTicker(heartbeatInterval)
    defer hb.Stop()

    for {
        select {
        case <-ctx.Done():
            return nil
        case <-tick.C:
            evs, err := getSince(ctx, workflowID, since)
            if err != nil { return err }
            for _, ev := range evs {
                if ev.SequenceNum > since { since = ev.SequenceNum }
                b, _ := json.Marshal(ev)
                fmt.Fprintf(w, "id: %d\n", ev.SequenceNum)
                fmt.Fprintf(w, "event: %s\n", string(ev.Type))
                fmt.Fprintf(w, "data: %s\n\n", string(b))
                flusher.Flush()
                if ev.Type == state.EventWorkflowCompleted ||
                    ev.Type == state.EventWorkflowFailed ||
                    ev.Type == state.EventWorkflowCanceled {
                    fmt.Fprintf(w, "event: done\ndata: {}\n\n")
                    flusher.Flush()
                    return nil
                }
            }
        case <-hb.C:
            fmt.Fprintf(w, ": ping\n\n")
            flusher.Flush()
        }
    }
}

