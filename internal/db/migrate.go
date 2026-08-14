package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate applies all pending migrations in order.
func Migrate(db *sql.DB) error {
	if err := ensureMigrationsTable(db); err != nil {
		return err
	}

	applied, err := getAppliedVersions(db)
	if err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return err
	}

	// Collect migration files and sort by name
	type migrationFile struct {
		name    string
		version string
	}
	var files []migrationFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".sql") {
			version := strings.TrimSuffix(name, ".sql")
			files = append(files, migrationFile{name: name, version: version})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].name < files[j].name
	})

	for _, f := range files {
		if _, ok := applied[f.version]; ok {
			continue
		}
		if err := applyMigration(db, f.version, f.name); err != nil {
			return fmt.Errorf("migration %s: %w", f.version, err)
		}
	}
	return nil
}

func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL
		)
	`)
	return err
}

func getAppliedVersions(db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := make(map[string]struct{})
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = struct{}{}
	}
	return applied, rows.Err()
}

func applyMigration(db *sql.DB, version, filename string) error {
	data, err := migrationFS.ReadFile("migrations/" + filename)
	if err != nil {
		return err
	}
	sqlText := string(data)

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	// Execute statements split by semicolon
	stmts := strings.Split(sqlText, ";")
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			tx.Rollback()
			return err
		}
	}
	// Record version
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, version, time.Now().UTC()); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
