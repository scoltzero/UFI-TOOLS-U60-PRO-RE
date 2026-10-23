// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *Store) CompareSwapScheduler(ctx context.Context, id, old, next string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "UPDATE scheduler SET data=? WHERE id=? AND data=?", next, id, old)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

type DailyUsage struct {
	Date  string `json:"date"`
	Usage int64  `json:"usage"`
}

func (s *Store) RecordUsage(ctx context.Context, timestamp, total int64) error {
	if timestamp <= 0 || total < 0 {
		return errors.New("invalid cellular sample")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var before, previousTS int64
	err = tx.QueryRowContext(ctx, "SELECT ts,total FROM cellular_recent ORDER BY ts DESC LIMIT 1").Scan(&previousTS, &before)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && timestamp <= previousTS {
		return fmt.Errorf("sample timestamp must increase")
	}
	delta := int64(0)
	if err == nil {
		if total >= before {
			delta = total - before
		} else {
			delta = total
		}
	}
	// Counters resetting start a new interval. Attribute the observed delta to the
	// sample's UTC day; never invent traffic before the first baseline.
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	if _, err = tx.ExecContext(ctx, `INSERT INTO cellular_daily(date,total) VALUES (?,?) ON CONFLICT(date) DO UPDATE SET total=total+excluded.total`, date, delta); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO cellular_recent(ts,total) VALUES (?,?)", timestamp, total); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM cellular_recent WHERE ts < ?", timestamp-7*86400); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM cellular_daily WHERE date < ?", time.Unix(timestamp, 0).UTC().AddDate(-2, 0, 0).Format("2006-01-02")); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) DailyUsage(ctx context.Context, start, end string) ([]DailyUsage, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT date,total FROM cellular_daily WHERE date>=? AND date<=? ORDER BY date", start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DailyUsage{}
	for rows.Next() {
		var v DailyUsage
		if err := rows.Scan(&v.Date, &v.Usage); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
