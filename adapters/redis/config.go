package redisstore

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config configures the Redis-backed Store.
type Config struct {
	Addr         string
	DB           int
	Password     string
	Prefix       string
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolSize     int
	Username     string
}

// Store is a Redis-backed implementation of state.Store.
type Store struct {
	rdb    redis.UniversalClient
	prefix string
	// cached SHA for the append event LUA script
	appendSHA string
	// ownsClient determines whether Close() should close the underlying client
	ownsClient bool
}

// New creates a new Redis Store with the provided configuration.
func New(cfg Config) (*Store, error) {
	opts := &redis.Options{
		Addr:         cfg.Addr,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolSize:     cfg.PoolSize,
	}
	rdb := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "marathon"
	}
	s := &Store{rdb: rdb, prefix: prefix, ownsClient: true}
	// load Lua script and cache SHA (best-effort; fallback to EVAL if it fails)
	if sha, err := s.rdb.ScriptLoad(ctx, luaAppendEvent).Result(); err == nil {
		s.appendSHA = sha
	}
	return s, nil
}

// Close closes the underlying Redis client.
func (s *Store) Close() error {
	if s.ownsClient {
		return s.rdb.Close()
	}
	return nil
}

// NewFromClient constructs a Store from a user-managed redis.UniversalClient.
// The Store will not Close() the client.
func NewFromClient(ctx context.Context, rdb redis.UniversalClient, prefix string) (*Store, error) {
	if prefix == "" {
		prefix = "marathon"
	}
	// Verify the connection works
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	s := &Store{rdb: rdb, prefix: prefix, ownsClient: false}
	// Best-effort load of the Lua script
	if sha, err := s.rdb.ScriptLoad(ctx, luaAppendEvent).Result(); err == nil {
		s.appendSHA = sha
	}
	return s, nil
}
