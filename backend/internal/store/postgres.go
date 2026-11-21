package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PoolConfig struct {
	DSN             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// NewPostgresPool creates a high-concurrency pgxpool tuned for 100+ simultaneous patrons and staff.
func NewPostgresPool(ctx context.Context, pc PoolConfig) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(pc.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse_pg_dsn: %w", err)
	}
	if pc.MaxConns > 0 {
		cfg.MaxConns = pc.MaxConns
	} else {
		cfg.MaxConns = 50
	}
	if pc.MinConns > 0 {
		cfg.MinConns = pc.MinConns
	} else {
		cfg.MinConns = 10
	}
	if pc.MaxConnLifetime > 0 {
		cfg.MaxConnLifetime = pc.MaxConnLifetime
	} else {
		cfg.MaxConnLifetime = 30 * time.Minute
	}
	if pc.MaxConnIdleTime > 0 {
		cfg.MaxConnIdleTime = pc.MaxConnIdleTime
	} else {
		cfg.MaxConnIdleTime = 5 * time.Minute
	}
	cfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect_pg_pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping_pg_pool: %w", err)
	}
	return pool, nil
}
