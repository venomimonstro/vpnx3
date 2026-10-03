package migrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type fileMigration struct {
	version int64
	name    string
	path    string
}

func Run(ctx context.Context, db *pgxpool.Pool, dir string) error {
	if _, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	var migrations []fileMigration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			continue
		}
		version, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid migration filename %q: %w", entry.Name(), err)
		}
		migrations = append(migrations, fileMigration{
			version: version,
			name: entry.Name(),
			path: filepath.Join(dir, entry.Name()),
		})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })

	for _, migration := range migrations {
		var exists bool
		if err := db.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)",
			migration.version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %d: %w", migration.version, err)
		}
		if exists {
			continue
		}

		sqlBytes, err := os.ReadFile(migration.path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", migration.name, err)
		}
		if _, err := db.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.name, err)
		}
		if _, err := db.Exec(ctx,
			"INSERT INTO schema_migrations(version,name) VALUES($1,$2)",
			migration.version, migration.name,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", migration.name, err)
		}
	}

	return nil
}
