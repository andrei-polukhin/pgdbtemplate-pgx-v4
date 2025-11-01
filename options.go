package pgdbtemplatepgxv4

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// ConnectionOption configures ConnectionProvider.
type ConnectionOption func(*ConnectionProvider)

// WithPoolConfig sets custom pool configuration.
func WithPoolConfig(config pgxpool.Config) ConnectionOption {
	return func(p *ConnectionProvider) {
		p.poolConfig = config
	}
}

// WithMaxConns sets the maximum number of connections in the pool.
func WithMaxConns(maxConns int32) ConnectionOption {
	return func(p *ConnectionProvider) {
		p.poolConfig.MaxConns = maxConns
	}
}

// WithMinConns sets the minimum number of connections in the pool.
func WithMinConns(minConns int32) ConnectionOption {
	return func(p *ConnectionProvider) {
		p.poolConfig.MinConns = minConns
	}
}

// WithMaxConnLifetime sets the maximum time a connection may be reused.
func WithMaxConnLifetime(d time.Duration) ConnectionOption {
	return func(p *ConnectionProvider) {
		p.poolConfig.MaxConnLifetime = d
	}
}

// WithMaxConnIdleTime sets the maximum time a connection may be idle.
func WithMaxConnIdleTime(d time.Duration) ConnectionOption {
	return func(p *ConnectionProvider) {
		p.poolConfig.MaxConnIdleTime = d
	}
}

// WithAfterConnect sets a function to be called
// after a new connection is established.
func WithAfterConnect(afterConnect func(context.Context, *pgx.Conn) error) ConnectionOption {
	return func(p *ConnectionProvider) {
		p.poolConfig.AfterConnect = afterConnect
	}
}
