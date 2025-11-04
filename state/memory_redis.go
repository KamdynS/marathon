//go:build redis
// +build redis

package state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/KamdynS/marathon/llm"
	"github.com/redis/go-redis/v9"
)

// RedisChatMemory stores conversation messages per session in Redis LISTs.
type RedisChatMemory struct {
	rdb *redis.Client
	ns  string
	// MaxMessages bounds history length per session; 0 disables trimming
	MaxMessages int
}

type ChatMemoryConfig struct {
	Addr        string
	Username    string
	Password    string
	DB          int
	Namespace   string
	MaxMessages int
}

func NewRedisChatMemory(cfg ChatMemoryConfig) (*RedisChatMemory, error) {
	rdb := redis.NewClient(&redis.Options{Addr: cfg.Addr, Username: cfg.Username, Password: cfg.Password, DB: cfg.DB})
	return &RedisChatMemory{rdb: rdb, ns: cfg.Namespace, MaxMessages: cfg.MaxMessages}, nil
}

func (m *RedisChatMemory) key(sessionID string) string {
	return fmt.Sprintf("%s:chat:%s", m.ns, sessionID)
}

// Append appends a message to a session's history.
func (m *RedisChatMemory) Append(ctx context.Context, sessionID string, msg llm.Message) error {
	wrapper := struct {
		llm.Message
		Timestamp time.Time `json:"timestamp"`
	}{Message: msg, Timestamp: time.Now().UTC()}
	b, err := json.Marshal(wrapper)
	if err != nil {
		return err
	}
	if err := m.rdb.RPush(ctx, m.key(sessionID), string(b)).Err(); err != nil {
		return err
	}
	if m.MaxMessages > 0 {
		_ = m.rdb.LTrim(ctx, m.key(sessionID), -int64(m.MaxMessages), -1).Err()
	}
	return nil
}

// Get returns the last N messages (or all if n<=0) for a session.
func (m *RedisChatMemory) Get(ctx context.Context, sessionID string, n int) ([]llm.Message, error) {
	start := int64(0)
	end := int64(-1)
	if n > 0 {
		start = -int64(n)
	}
	vals, err := m.rdb.LRange(ctx, m.key(sessionID), start, end).Result()
	if err != nil {
		return nil, err
	}
	out := make([]llm.Message, 0, len(vals))
	for _, v := range vals {
		var wrapper struct {
			llm.Message
			Timestamp time.Time `json:"timestamp"`
		}
		if json.Unmarshal([]byte(v), &wrapper) == nil {
			out = append(out, wrapper.Message)
		}
	}
	return out, nil
}
