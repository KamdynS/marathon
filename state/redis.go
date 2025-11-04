//go:build redis
// +build redis

package state

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// RedisStore implements Store using Redis HASH/LIST primitives.
type RedisStore struct {
	rdb *redis.Client
	ns  string
}

type RedisConfig struct {
	Addr      string
	Username  string
	Password  string
	DB        int
	Namespace string
}

func NewRedisStore(cfg RedisConfig) (*RedisStore, error) {
	rdb := redis.NewClient(&redis.Options{Addr: cfg.Addr, Username: cfg.Username, Password: cfg.Password, DB: cfg.DB})
	return &RedisStore{rdb: rdb, ns: cfg.Namespace}, nil
}

func (s *RedisStore) keyWorkflow(id string) string { return fmt.Sprintf("%s:wf:%s", s.ns, id) }
func (s *RedisStore) keyEvents(id string) string   { return fmt.Sprintf("%s:events:%s", s.ns, id) }
func (s *RedisStore) keyActivity(id string) string { return fmt.Sprintf("%s:act:%s", s.ns, id) }

func (s *RedisStore) SaveWorkflowState(ctx context.Context, st *WorkflowState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.rdb.HSet(ctx, s.keyWorkflow(st.WorkflowID), "state", string(b)).Err()
}

func (s *RedisStore) GetWorkflowState(ctx context.Context, workflowID string) (*WorkflowState, error) {
	v, err := s.rdb.HGet(ctx, s.keyWorkflow(workflowID), "state").Result()
	if err != nil {
		return nil, err
	}
	var st WorkflowState
	if err := json.Unmarshal([]byte(v), &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *RedisStore) AppendEvent(ctx context.Context, e *Event) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return s.rdb.RPush(ctx, s.keyEvents(e.WorkflowID), string(b)).Err()
}

func (s *RedisStore) GetEvents(ctx context.Context, workflowID string) ([]*Event, error) {
	vals, err := s.rdb.LRange(ctx, s.keyEvents(workflowID), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	out := make([]*Event, 0, len(vals))
	for _, v := range vals {
		var e Event
		if err := json.Unmarshal([]byte(v), &e); err == nil {
			out = append(out, &e)
		}
	}
	return out, nil
}

func (s *RedisStore) GetEventsSince(ctx context.Context, workflowID string, since int64) ([]*Event, error) {
	// Simplified: return all and filter client-side
	all, err := s.GetEvents(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	out := make([]*Event, 0, len(all))
	for _, e := range all {
		if e.SequenceNum > since {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *RedisStore) SaveActivityState(ctx context.Context, st *ActivityState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.rdb.HSet(ctx, s.keyActivity(st.ActivityID), "state", string(b)).Err()
}

func (s *RedisStore) GetActivityState(ctx context.Context, activityID string) (*ActivityState, error) {
	v, err := s.rdb.HGet(ctx, s.keyActivity(activityID), "state").Result()
	if err != nil {
		return nil, err
	}
	var st ActivityState
	if err := json.Unmarshal([]byte(v), &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *RedisStore) ListWorkflows(ctx context.Context, status WorkflowStatus) ([]*WorkflowState, error) {
	// Simplified: requires index; omitted for now. Return empty without error.
	return []*WorkflowState{}, nil
}

func (s *RedisStore) DeleteWorkflow(ctx context.Context, workflowID string) error {
	if err := s.rdb.Del(ctx, s.keyWorkflow(workflowID)).Err(); err != nil {
		return err
	}
	if err := s.rdb.Del(ctx, s.keyEvents(workflowID)).Err(); err != nil {
		return err
	}
	return nil
}
