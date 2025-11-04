package llm

import (
    "context"
    "math"
    "time"
)

// Retrier performs bounded exponential backoff retries for a function.
type Retrier struct {
    cfg RetryConfig
}

// NewRetrier creates a new Retrier with the given config (or defaults if zero values).
func NewRetrier(cfg RetryConfig) *Retrier {
    if cfg.MaxRetries <= 0 {
        cfg.MaxRetries = DefaultRetryConfig().MaxRetries
    }
    if cfg.InitialDelay <= 0 {
        cfg.InitialDelay = DefaultRetryConfig().InitialDelay
    }
    if cfg.MaxDelay <= 0 {
        cfg.MaxDelay = DefaultRetryConfig().MaxDelay
    }
    if cfg.BackoffFactor <= 0 {
        cfg.BackoffFactor = DefaultRetryConfig().BackoffFactor
    }
    return &Retrier{cfg: cfg}
}

// Do runs fn and retries on error up to MaxRetries with exponential backoff.
func (r *Retrier) Do(ctx context.Context, fn func() error) error {
    var attempt int
    var delay = r.cfg.InitialDelay
    for {
        if err := fn(); err != nil {
            if attempt >= r.cfg.MaxRetries {
                return err
            }
            // Sleep with backoff or until context canceled
            t := time.NewTimer(delay)
            select {
            case <-ctx.Done():
                t.Stop()
                return ctx.Err()
            case <-t.C:
            }
            // Increase delay
            attempt++
            next := time.Duration(float64(delay) * r.cfg.BackoffFactor)
            if next > r.cfg.MaxDelay {
                next = r.cfg.MaxDelay
            }
            // Guard against overflow
            if next < 0 || next > time.Duration(math.MaxInt64) {
                next = r.cfg.MaxDelay
            }
            delay = next
            continue
        }
        return nil
    }
}


