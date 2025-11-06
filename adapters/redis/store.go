package redisstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/KamdynS/marathon/state"
)

// Ensure Store implements state.Store
var _ state.Store = (*Store)(nil)

// ---------- Key helpers ----------

func (s *Store) wfStateKey(id string) string  { return fmt.Sprintf("%s:wf:%s:state", s.prefix, id) }
func (s *Store) wfEventsKey(id string) string { return fmt.Sprintf("%s:wf:%s:events", s.prefix, id) }
func (s *Store) wfSeqKey(id string) string    { return fmt.Sprintf("%s:wf:%s:seq", s.prefix, id) }
func (s *Store) actStateKey(id string) string { return fmt.Sprintf("%s:act:%s:state", s.prefix, id) }
func (s *Store) statusIdxKey(st state.WorkflowStatus) string {
	return fmt.Sprintf("%s:idx:status:%s", s.prefix, string(st))
}
func (s *Store) idemStartKey(key string) string {
	return fmt.Sprintf("%s:idem:start:%s", s.prefix, key)
}
func (s *Store) idemActKey(id string) string { return fmt.Sprintf("%s:idem:act:%s", s.prefix, id) }
func (s *Store) timersDueKey() string        { return fmt.Sprintf("%s:timers:due", s.prefix) }
func (s *Store) timerRecKey(workflowID, timerID string) string {
	return fmt.Sprintf("%s:timer:%s:%s", s.prefix, workflowID, timerID)
}

// ---------- Workflow State ----------

func (s *Store) SaveWorkflowState(ctx context.Context, st *state.WorkflowState) error {
	// Fetch existing to detect prior status for index maintenance
	var oldStatus *state.WorkflowStatus
	oldJSON, err := s.rdb.Get(ctx, s.wfStateKey(st.WorkflowID)).Bytes()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("redis get workflow state: %w", err)
	}
	if len(oldJSON) > 0 {
		var prev state.WorkflowState
		if uerr := json.Unmarshal(oldJSON, &prev); uerr == nil {
			os := prev.Status
			oldStatus = &os
		}
	}

	b, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal workflow state: %w", err)
	}

	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, s.wfStateKey(st.WorkflowID), b, 0)
	// maintain status sets
	if oldStatus != nil && *oldStatus != st.Status {
		pipe.SRem(ctx, s.statusIdxKey(*oldStatus), st.WorkflowID)
	}
	pipe.SAdd(ctx, s.statusIdxKey(st.Status), st.WorkflowID)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline save workflow state: %w", err)
	}
	return nil
}

func (s *Store) GetWorkflowState(ctx context.Context, workflowID string) (*state.WorkflowState, error) {
	v, err := s.rdb.Get(ctx, s.wfStateKey(workflowID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("workflow %s not found", workflowID)
		}
		return nil, fmt.Errorf("redis get workflow state: %w", err)
	}
	var st state.WorkflowState
	if err := json.Unmarshal(v, &st); err != nil {
		return nil, fmt.Errorf("unmarshal workflow state: %w", err)
	}
	return &st, nil
}

// ---------- Activity State ----------

func (s *Store) SaveActivityState(ctx context.Context, st *state.ActivityState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal activity state: %w", err)
	}
	if err := s.rdb.Set(ctx, s.actStateKey(st.ActivityID), b, 0).Err(); err != nil {
		return fmt.Errorf("redis set activity state: %w", err)
	}
	return nil
}

func (s *Store) GetActivityState(ctx context.Context, activityID string) (*state.ActivityState, error) {
	v, err := s.rdb.Get(ctx, s.actStateKey(activityID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("activity %s not found", activityID)
		}
		return nil, fmt.Errorf("redis get activity state: %w", err)
	}
	var st state.ActivityState
	if err := json.Unmarshal(v, &st); err != nil {
		return nil, fmt.Errorf("unmarshal activity state: %w", err)
	}
	return &st, nil
}

// ---------- Events ----------

func (s *Store) AppendEvent(ctx context.Context, e *state.Event) error {
	// Event JSON may not have SequenceNum set; Lua will set it and update workflow state atomically
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	keys := []string{s.wfSeqKey(e.WorkflowID), s.wfEventsKey(e.WorkflowID), s.wfStateKey(e.WorkflowID)}
	args := []interface{}{string(b)}

	// Try EVALSHA first if we cached the SHA
	if s.appendSHA != "" {
		if _, err := s.rdb.EvalSha(ctx, s.appendSHA, keys, args...).Result(); err == nil {
			return nil
		}
		// if NOSCRIPT or other error, fall through to EVAL
	}
	if _, err := s.rdb.Eval(ctx, luaAppendEvent, keys, args...).Result(); err != nil {
		return fmt.Errorf("redis eval append event: %w", err)
	}
	return nil
}

func (s *Store) GetEvents(ctx context.Context, workflowID string) ([]*state.Event, error) {
	vals, err := s.rdb.ZRange(ctx, s.wfEventsKey(workflowID), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("redis zrange events: %w", err)
	}
	out := make([]*state.Event, 0, len(vals))
	for _, v := range vals {
		ev, uerr := state.FromJSON([]byte(v))
		if uerr != nil {
			continue
		}
		out = append(out, ev)
	}
	return out, nil
}

func (s *Store) GetEventsSince(ctx context.Context, workflowID string, since int64) ([]*state.Event, error) {
	// ZRANGEBYSCORE (since, +inf] using exclusive min
	min := "(" + strconv.FormatInt(since, 10)
	opt := &redis.ZRangeBy{
		Min: min,
		Max: "+inf",
	}
	vals, err := s.rdb.ZRangeByScore(ctx, s.wfEventsKey(workflowID), opt).Result()
	if err != nil {
		return nil, fmt.Errorf("redis zrangebyscore events: %w", err)
	}
	out := make([]*state.Event, 0, len(vals))
	for _, v := range vals {
		ev, uerr := state.FromJSON([]byte(v))
		if uerr != nil {
			continue
		}
		out = append(out, ev)
	}
	return out, nil
}

// ---------- Listing / Deletion ----------

func (s *Store) ListWorkflows(ctx context.Context, st state.WorkflowStatus) ([]*state.WorkflowState, error) {
	if st == "" {
		// No global index of all workflows; best-effort: collect from all known status sets
		statuses := []state.WorkflowStatus{
			state.StatusPending, state.StatusRunning, state.StatusCompleted, state.StatusFailed, state.StatusCanceled,
		}
		idSet := make(map[string]struct{})
		for _, status := range statuses {
			members, err := s.rdb.SMembers(ctx, s.statusIdxKey(status)).Result()
			if err != nil && err != redis.Nil {
				return nil, fmt.Errorf("redis smembers status %s: %w", status, err)
			}
			for _, id := range members {
				idSet[id] = struct{}{}
			}
		}
		if len(idSet) == 0 {
			return []*state.WorkflowState{}, nil
		}
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		return s.mgetWorkflowStates(ctx, ids)
	}
	ids, err := s.rdb.SMembers(ctx, s.statusIdxKey(st)).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("redis smembers: %w", err)
	}
	if len(ids) == 0 {
		return []*state.WorkflowState{}, nil
	}
	return s.mgetWorkflowStates(ctx, ids)
}

func (s *Store) mgetWorkflowStates(ctx context.Context, ids []string) ([]*state.WorkflowState, error) {
	pipe := s.rdb.Pipeline()
	cmds := make([]*redis.StringCmd, 0, len(ids))
	for _, id := range ids {
		cmds = append(cmds, pipe.Get(ctx, s.wfStateKey(id)))
	}
	if _, err := pipe.Exec(ctx); err != nil && !strings.Contains(err.Error(), "nil") {
		// go-redis returns first error from commands; we'll proceed and filter redis.Nil below
		// ignore here; individual cmds will be checked
	}
	out := make([]*state.WorkflowState, 0, len(ids))
	for _, cmd := range cmds {
		v, err := cmd.Bytes()
		if err != nil {
			continue
		}
		var st state.WorkflowState
		if uerr := json.Unmarshal(v, &st); uerr != nil {
			continue
		}
		out = append(out, &st)
	}
	return out, nil
}

func (s *Store) DeleteWorkflow(ctx context.Context, workflowID string) error {
	// Try to fetch current state to remove from its status set
	if st, err := s.GetWorkflowState(ctx, workflowID); err == nil && st != nil {
		_ = s.rdb.SRem(ctx, s.statusIdxKey(st.Status), workflowID).Err()
	}
	pipe := s.rdb.Pipeline()
	pipe.Del(ctx, s.wfStateKey(workflowID))
	pipe.Del(ctx, s.wfEventsKey(workflowID))
	pipe.Del(ctx, s.wfSeqKey(workflowID))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline delete workflow: %w", err)
	}
	return nil
}

// ---------- Idempotency ----------

func (s *Store) MapIdempotencyKeyToWorkflow(ctx context.Context, key string, workflowID string) (bool, string, error) {
	ok, err := s.rdb.SetNX(ctx, s.idemStartKey(key), workflowID, 0).Result()
	if err != nil {
		return false, "", fmt.Errorf("redis setnx idempotency key: %w", err)
	}
	if ok {
		return true, "", nil
	}
	val, err := s.rdb.Get(ctx, s.idemStartKey(key)).Result()
	if err != nil {
		if err == redis.Nil {
			return false, "", nil
		}
		return false, "", fmt.Errorf("redis get idempotency key: %w", err)
	}
	return false, val, nil
}

func (s *Store) GetWorkflowIDByIdempotencyKey(ctx context.Context, key string) (string, bool, error) {
	val, err := s.rdb.Get(ctx, s.idemStartKey(key)).Result()
	if err != nil {
		if err == redis.Nil {
			return "", false, nil
		}
		return "", false, fmt.Errorf("redis get idempotency key: %w", err)
	}
	return val, true, nil
}

// MarkActivityIfFirst provides activity-level idempotency (adapter helper).
func (s *Store) MarkActivityIfFirst(ctx context.Context, activityID string) (bool, error) {
	ok, err := s.rdb.SetNX(ctx, s.idemActKey(activityID), "1", 0).Result()
	if err != nil {
		return false, fmt.Errorf("redis setnx activity idempotency: %w", err)
	}
	return ok, nil
}

// ---------- Timers ----------

func (s *Store) ScheduleTimer(ctx context.Context, workflowID string, timerID string, fireAt time.Time) error {
	rec := state.TimerRecord{
		WorkflowID: workflowID,
		TimerID:    timerID,
		FireAt:     fireAt,
		Fired:      false,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal timer record: %w", err)
	}
	timerKey := s.timerRecKey(workflowID, timerID)
	member := workflowID + ":" + timerID

	pipe := s.rdb.Pipeline()
	pipe.SetNX(ctx, timerKey, b, 0)
	pipe.ZAddNX(ctx, s.timersDueKey(), redis.Z{Score: float64(fireAt.UnixMilli()), Member: member})
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline schedule timer: %w", err)
	}
	return nil
}

func (s *Store) ListDueTimers(ctx context.Context, now time.Time) ([]state.TimerRecord, error) {
	opt := &redis.ZRangeBy{
		Min: "-inf",
		Max: strconv.FormatInt(now.UnixMilli(), 10),
	}
	members, err := s.rdb.ZRangeByScore(ctx, s.timersDueKey(), opt).Result()
	if err != nil {
		return nil, fmt.Errorf("redis zrangebyscore timers: %w", err)
	}
	if len(members) == 0 {
		return []state.TimerRecord{}, nil
	}
	// Fetch each timer record
	pipe := s.rdb.Pipeline()
	cmds := make([]*redis.StringCmd, 0, len(members))
	for _, m := range members {
		parts := strings.SplitN(m, ":", 2)
		if len(parts) != 2 {
			continue
		}
		cmds = append(cmds, pipe.Get(ctx, s.timerRecKey(parts[0], parts[1])))
	}
	if _, err := pipe.Exec(ctx); err != nil && !strings.Contains(err.Error(), "nil") {
		// ignore; we'll check individual commands
	}
	out := make([]state.TimerRecord, 0, len(cmds))
	for _, cmd := range cmds {
		v, err := cmd.Bytes()
		if err != nil {
			continue
		}
		var rec state.TimerRecord
		if uerr := json.Unmarshal(v, &rec); uerr != nil {
			continue
		}
		if !rec.Fired {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (s *Store) MarkTimerFired(ctx context.Context, workflowID string, timerID string) (bool, error) {
	keys := []string{s.timerRecKey(workflowID, timerID), s.timersDueKey()}
	member := workflowID + ":" + timerID
	res, err := s.rdb.Eval(ctx, luaMarkTimerFired, keys, member).Int()
	if err != nil {
		return false, fmt.Errorf("redis eval mark timer fired: %w", err)
	}
	return res == 1, nil
}
