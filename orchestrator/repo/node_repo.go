package repo

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"sandboxd-o/orchestrator/types"

	_ "modernc.org/sqlite"
)

type NodeRepo interface {
	Close() error
	UpsertNode(ctx context.Context, name, ip string, port int, source string) error
	DeleteNode(ctx context.Context, name string) error
	GetNode(ctx context.Context, name string) (*types.Node, error)
	ListNodes(ctx context.Context) ([]types.Node, error)
	UpdateHeartbeat(ctx context.Context, name string, state types.NodeState, successStreak, failureStreak int, lastError string, beatAt *time.Time) error
}

type SQLiteNodeRepo struct {
	db *sql.DB
}

func NewSQLite(path string) (*SQLiteNodeRepo, error) {
	if path != "" && path != ":memory:" {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				if dir := filepath.Dir(path); dir != "" && dir != "." {
					if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
						return nil, fmt.Errorf("create sqlite dir: %w", mkErr)
					}
				}

				f, createErr := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
				if createErr != nil {
					return nil, fmt.Errorf("create sqlite db file: %w", createErr)
				}
				_ = f.Close()
				slog.Warn("sqlite db file not found; created new file", slog.String("path", path))
			} else {
				return nil, fmt.Errorf("stat sqlite db file: %w", err)
			}
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &SQLiteNodeRepo{db: db}, nil
}

func (r *SQLiteNodeRepo) Close() error {
	if r == nil || r.db == nil {
		return nil
	}

	return r.db.Close()
}

func migrate(db *sql.DB) error {
	const q = `
CREATE TABLE IF NOT EXISTS nodes (
  name TEXT PRIMARY KEY,
  ip TEXT NOT NULL,
  port INTEGER NOT NULL,
  source TEXT NOT NULL,
  state TEXT NOT NULL,
  success_streak INTEGER NOT NULL DEFAULT 0,
  failure_streak INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  last_heartbeat_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`
	_, err := db.Exec(q)
	return err
}

func (r *SQLiteNodeRepo) UpsertNode(ctx context.Context, name, ip string, port int, source string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	const q = `
INSERT INTO nodes(name, ip, port, source, state, success_streak, failure_streak, last_error, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, 0, 0, '', ?, ?)
ON CONFLICT(name) DO UPDATE SET
  ip=excluded.ip,
  port=excluded.port,
  source=excluded.source,
  updated_at=excluded.updated_at;
`
	_, err := r.db.ExecContext(ctx, q, name, ip, port, source, string(types.NodeStateUnknown), now, now)
	return err
}

func (r *SQLiteNodeRepo) DeleteNode(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM nodes WHERE name=?`, name)
	return err
}

func (r *SQLiteNodeRepo) GetNode(ctx context.Context, name string) (*types.Node, error) {
	const q = `SELECT name, ip, port, source, state, success_streak, failure_streak, last_error, last_heartbeat_at, created_at, updated_at FROM nodes WHERE name=?`
	return scanOne(r.db.QueryRowContext(ctx, q, name))
}

func (r *SQLiteNodeRepo) ListNodes(ctx context.Context) ([]types.Node, error) {
	const q = `SELECT name, ip, port, source, state, success_streak, failure_streak, last_error, last_heartbeat_at, created_at, updated_at FROM nodes ORDER BY name ASC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]types.Node, 0)
	for rows.Next() {
		n, err := scanRow(rows)
		if err != nil {
			return nil, err
		}

		n.SandboxdBaseURL = fmt.Sprintf("http://%s:%d", n.IP, n.Port)
		out = append(out, n)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *SQLiteNodeRepo) UpdateHeartbeat(ctx context.Context, name string, state types.NodeState, successStreak, failureStreak int, lastError string, beatAt *time.Time) error {
	updated := time.Now().UTC().Format(time.RFC3339Nano)
	var beat any
	if beatAt != nil {
		beat = beatAt.UTC().Format(time.RFC3339Nano)
	}

	const q = `UPDATE nodes SET state=?, success_streak=?, failure_streak=?, last_error=?, last_heartbeat_at=?, updated_at=? WHERE name=?`
	_, err := r.db.ExecContext(ctx, q, string(state), successStreak, failureStreak, lastError, beat, updated, name)
	return err
}

type scanner interface{ Scan(dest ...any) error }

func scanOne(s scanner) (*types.Node, error) {
	n, err := scanRowScanner(s)
	if err != nil {
		return nil, err
	}

	n.SandboxdBaseURL = fmt.Sprintf("http://%s:%d", n.IP, n.Port)
	return &n, nil
}

func scanRow(rows *sql.Rows) (types.Node, error) { return scanRowScanner(rows) }

func scanRowScanner(s scanner) (types.Node, error) {
	var n types.Node
	var state string
	var created, updated string
	var beat sql.NullString
	if err := s.Scan(&n.Name, &n.IP, &n.Port, &n.Source, &state, &n.SuccessStreak, &n.FailureStreak, &n.LastError, &beat, &created, &updated); err != nil {
		return n, err
	}
	n.State = types.NodeState(state)

	ct, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return n, err
	}

	ut, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return n, err
	}

	n.CreatedAt = ct
	n.UpdatedAt = ut

	if beat.Valid && beat.String != "" {
		bt, err := time.Parse(time.RFC3339Nano, beat.String)
		if err != nil {
			return n, err
		}

		n.LastHeartbeat = &bt
	}

	return n, nil
}
