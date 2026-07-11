// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	store := &Store{db: db}
	if err := store.configure(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) configure(ctx context.Context) error {
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS config (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS kv (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS scheduler (id TEXT PRIMARY KEY, data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS sms_forward (id INTEGER PRIMARY KEY CHECK(id=1), data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS cellular_recent (ts INTEGER PRIMARY KEY, total INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS cellular_daily (date TEXT PRIMARY KEY, total INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS uploads (name TEXT PRIMARY KEY, data BLOB NOT NULL, size INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT INTO schema_meta(key, value) VALUES ('version', '1') ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("run migration: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) Get(ctx context.Context, table, key string) (string, error) {
	if table != "config" && table != "kv" {
		return "", errors.New("unsupported key/value table")
	}
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM "+table+" WHERE key = ?", key).Scan(&value)
	return value, err
}

func (s *Store) Set(ctx context.Context, table, key, value string) error {
	if table != "config" && table != "kv" {
		return errors.New("unsupported key/value table")
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO "+table+"(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
		key, value,
	)
	return err
}

func (s *Store) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return s.Ping(ctx)
}
