package repo

import (
	"database/sql"
	"fmt"
)

type migration struct {
	name string
	up   func(*sql.DB) error
}

var sqliteMigrations = []migration{
	{
		name: "create core tables",
		up: func(db *sql.DB) error {
			const qNodes = `
CREATE TABLE IF NOT EXISTS nodes (
  name TEXT PRIMARY KEY,
  ip TEXT NOT NULL,
  port INTEGER NOT NULL,
  source TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`
			const qStatus = `
CREATE TABLE IF NOT EXISTS node_status (
  name TEXT PRIMARY KEY,
  state TEXT NOT NULL,
  success_streak INTEGER NOT NULL DEFAULT 0,
  failure_streak INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  last_heartbeat_at TEXT,
  updated_at TEXT NOT NULL
);
`
			const qResources = `
CREATE TABLE IF NOT EXISTS node_resources (
  name TEXT PRIMARY KEY,
  capacity_cpu_milli INTEGER NOT NULL DEFAULT 0,
  capacity_memory_bytes INTEGER NOT NULL DEFAULT 0,
  allocatable_cpu_milli INTEGER NOT NULL DEFAULT 0,
  allocatable_memory_bytes INTEGER NOT NULL DEFAULT 0,
  used_cpu_milli INTEGER NOT NULL DEFAULT 0,
  used_memory_bytes INTEGER NOT NULL DEFAULT 0,
  available_cpu_milli INTEGER NOT NULL DEFAULT 0,
  available_memory_bytes INTEGER NOT NULL DEFAULT 0,
  max_alloc_percent INTEGER NOT NULL DEFAULT 0,
  external TEXT NOT NULL DEFAULT '(none)',
  resource_updated_at TEXT,
  updated_at TEXT NOT NULL
);
`
			const qSandboxes = `
CREATE TABLE IF NOT EXISTS sandboxes (
  id TEXT PRIMARY KEY,
  spec_json TEXT NOT NULL,
  status_json TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`
			const qSandboxPorts = `
CREATE TABLE IF NOT EXISTS sandbox_ports (
  sandbox_id TEXT NOT NULL,
  node_name TEXT NOT NULL,
  host_port INTEGER NOT NULL,
  container_port INTEGER NOT NULL,
  protocol TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (node_name, host_port),
  FOREIGN KEY (sandbox_id) REFERENCES sandboxes(id) ON DELETE CASCADE
);
`
			const qNodeExternals = `
CREATE TABLE IF NOT EXISTS node_externals (
  external_id TEXT PRIMARY KEY,
  node_id TEXT NOT NULL UNIQUE,
  external TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (node_id) REFERENCES nodes(name) ON DELETE CASCADE
);
`

			for _, q := range []string{qNodes, qStatus, qResources, qSandboxes, qSandboxPorts, qNodeExternals} {
				if _, err := db.Exec(q); err != nil {
					return err
				}
			}

			return nil
		},
	},
	{
		name: "add nodes.unschedulable",
		up: func(db *sql.DB) error {
			ok, err := sqliteColumnExists(db, "nodes", "unschedulable")
			if err != nil {
				return err
			}
			if ok {
				return nil
			}

			_, err = db.Exec(`ALTER TABLE nodes ADD COLUMN unschedulable INTEGER NOT NULL DEFAULT 0`)
			return err
		},
	},
}

func migrate(db *sql.DB) error {
	for _, m := range sqliteMigrations {
		if err := m.up(db); err != nil {
			return fmt.Errorf("migration %q: %w", m.name, err)
		}
	}

	return nil
}

func sqliteColumnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `);`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			typ       string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)

		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}

	return false, rows.Err()
}
