// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Scheduler struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

type Upload struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type CellularPoint struct {
	Timestamp int64 `json:"ts"`
	Total     int64 `json:"total"`
}

type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

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

func (s *Store) Delete(ctx context.Context, table, key string) error {
	if table != "config" && table != "kv" {
		return errors.New("unsupported key/value table")
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE key = ?", key)
	return err
}

func (s *Store) ListPrefix(ctx context.Context, table, prefix string) ([]KeyValue, error) {
	if table != "config" && table != "kv" {
		return nil, errors.New("unsupported key/value table")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM "+table+" WHERE key LIKE ? ORDER BY key", prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []KeyValue
	for rows.Next() {
		var item KeyValue
		if err := rows.Scan(&item.Key, &item.Value); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return s.Ping(ctx)
}

func (s *Store) ListSchedulers(ctx context.Context) ([]Scheduler, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, data FROM scheduler ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Scheduler
	for rows.Next() {
		var item Scheduler
		if err := rows.Scan(&item.ID, &item.Data); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) SetScheduler(ctx context.Context, id, data string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO scheduler(id, data) VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET data=excluded.data`, id, data)
	return err
}

func (s *Store) GetScheduler(ctx context.Context, id string) (Scheduler, error) {
	var item Scheduler
	err := s.db.QueryRowContext(ctx, "SELECT id, data FROM scheduler WHERE id = ?", id).Scan(&item.ID, &item.Data)
	return item, err
}

func (s *Store) DeleteScheduler(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM scheduler WHERE id = ?", id)
	return err
}

func (s *Store) GetSMSForward(ctx context.Context) (string, error) {
	var data string
	err := s.db.QueryRowContext(ctx, "SELECT data FROM sms_forward WHERE id = 1").Scan(&data)
	return data, err
}

func (s *Store) SetSMSForward(ctx context.Context, data string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sms_forward(id, data) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET data=excluded.data`, data)
	return err
}

func (s *Store) PutUpload(ctx context.Context, name string, data []byte) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO uploads(name, data, size) VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET data=excluded.data, size=excluded.size`, name, data, len(data))
	return err
}

func (s *Store) GetUpload(ctx context.Context, name string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT data FROM uploads WHERE name = ?", name).Scan(&data)
	return data, err
}

func (s *Store) DeleteUpload(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM uploads WHERE name = ?", name)
	return err
}

func (s *Store) ListUploads(ctx context.Context) ([]Upload, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT name, size FROM uploads ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Upload
	for rows.Next() {
		var item Upload
		if err := rows.Scan(&item.Name, &item.Size); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) RecordCellular(ctx context.Context, timestamp, total int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO cellular_recent(ts, total) VALUES (?, ?)
		ON CONFLICT(ts) DO UPDATE SET total=excluded.total`, timestamp, total)
	return err
}

func (s *Store) RecentCellular(ctx context.Context, limit int) ([]CellularPoint, error) {
	if limit <= 0 || limit > 1000 {
		limit = 288
	}
	rows, err := s.db.QueryContext(ctx, "SELECT ts, total FROM cellular_recent ORDER BY ts DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CellularPoint
	for rows.Next() {
		var item CellularPoint
		if err := rows.Scan(&item.Timestamp, &item.Total); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) DeleteAllUploads(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM uploads")
	return err
}

func (s *Store) StableID(ctx context.Context, key string) (string, error) {
	value, err := s.Get(ctx, "config", key)
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	value = hex.EncodeToString(random)
	if err := s.Set(ctx, "config", key, value); err != nil {
		return "", err
	}
	return value, nil
}

// SeedDeviceToken provisions a freshly generated installer token without ever
// replacing an existing or user-rotated authentication token.
func (s *Store) SeedDeviceToken(ctx context.Context, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO config(key,value) VALUES('device_token',?) ON CONFLICT(key) DO NOTHING`, value)
	return err
}
