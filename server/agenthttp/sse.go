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

// StreamEvents streams workflow events from a state.Store as Server-Sent Events.
// Supports Last-Event-ID for resume and sends heartbeat comments periodically.
func StreamEvents(ctx context.Context, w http.ResponseWriter, store state.Store, workflowID string, heartbeatInterval time.Duration) error {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    flusher, ok := w.(http.Flusher)
    if !ok { return fmt.Errorf("stream unsupported") }

    // Pull Last-Event-ID from request, if any
    var since int64
    if rid, ok := ctx.Value(lastEventIDKey{}).(string); ok && rid != "" {
        if v, err := strconv.ParseInt(rid, 10, 64); err == nil { since = v }
    }

    tick := time.NewTicker(time.Second)
    defer tick.Stop()
    if heartbeatInterval <= 0 { heartbeatInterval = 15 * time.Second }
    hb := time.NewTicker(heartbeatInterval)
    defer hb.Stop()

    for {
        select {
        case <-ctx.Done():
            return nil
        case <-tick.C:
            evs, err := store.GetEventsSince(ctx, workflowID, since)
            if err != nil { return err }
            for _, ev := range evs {
                if ev.SequenceNum > since { since = ev.SequenceNum }
                b, _ := json.Marshal(ev)
                fmt.Fprintf(w, "id: %d\n", ev.SequenceNum)
                fmt.Fprintf(w, "event: %s\n", string(ev.Type))
                fmt.Fprintf(w, "data: %s\n\n", string(b))
            }
            flusher.Flush()
        case <-hb.C:
            fmt.Fprintf(w, ": keep-alive\n\n")
            flusher.Flush()
        }
    }
}

// lastEventIDKey allows passing Last-Event-ID from the HTTP layer into context.
type lastEventIDKey struct{}

// WithLastEventID attaches Last-Event-ID to a context for StreamEvents.
func WithLastEventID(ctx context.Context, id string) context.Context { return context.WithValue(ctx, lastEventIDKey{}, id) }


