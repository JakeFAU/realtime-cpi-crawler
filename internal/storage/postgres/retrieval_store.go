// Package postgres provides Postgres-backed persistence implementations.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

var validTableName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// RetrievalStoreConfig controls the Postgres connection pool used for retrieval rows.
type RetrievalStoreConfig struct {
	DSN             string
	Table           string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
}

type execCloser interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Close()
}

// RetrievalStore writes retrieval rows into Postgres.
type RetrievalStore struct {
	pool  execCloser
	table string
}

// NewRetrievalStore creates a Postgres-backed RetrievalStore using the provided config.
func NewRetrievalStore(ctx context.Context, cfg RetrievalStoreConfig) (*RetrievalStore, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("database.dsn is required")
	}
	table := cfg.Table
	if table == "" {
		table = "retrievals"
	}
	if !validTableName.MatchString(table) {
		return nil, fmt.Errorf("invalid table name %q", table)
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	}
	if cfg.MaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return &RetrievalStore{
		pool:  pool,
		table: table,
	}, nil
}

// NewRetrievalStoreWithPool constructs a store from an existing pool (primarily for testing).
func NewRetrievalStoreWithPool(pool execCloser, table string) (*RetrievalStore, error) {
	if pool == nil {
		return nil, fmt.Errorf("pool is required")
	}
	if table == "" {
		table = "retrievals"
	}
	if !validTableName.MatchString(table) {
		return nil, fmt.Errorf("invalid table name %q", table)
	}
	return &RetrievalStore{pool: pool, table: table}, nil
}

// Close releases the underlying pool resources.
func (s *RetrievalStore) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}

// StoreRetrieval inserts a retrieval row into Postgres.
func (s *RetrievalStore) StoreRetrieval(ctx context.Context, record crawler.RetrievalRecord) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("retrieval store is not configured")
	}
	if record.ID == "" {
		return fmt.Errorf("record id is required")
	}
	headersJSON, err := json.Marshal(normalizeHeaders(record.Headers))
	if err != nil {
		return fmt.Errorf("marshal headers: %w", err)
	}
	query := fmt.Sprintf(`
INSERT INTO %s (
	id,
	job_uuid,
	partition_ts,
	retrieval_timestamp,
	retrieval_url,
	retrieval_hashcode,
	retrieval_blob_location,
	retrieval_headers,
	retrieval_status_code,
	retrieval_content_type,
	parent_id,
	parent_ts
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12
)`, s.table)

	args := []any{
		record.ID,
		record.JobID,
		record.PartitionTS,
		record.RetrievedAt,
		record.URL,
		record.Hash,
		record.BlobURI,
		headersJSON,
		record.StatusCode,
		record.ContentType,
		record.ParentID,
		record.ParentTimestamp,
	}
	if _, err := s.pool.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert retrieval: %w", err)
	}
	return nil
}

func normalizeHeaders(h http.Header) map[string][]string {
	if len(h) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(h))
	for k, values := range h {
		out[k] = append([]string(nil), values...)
	}
	return out
}
