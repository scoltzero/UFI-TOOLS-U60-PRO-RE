// SPDX-License-Identifier: AGPL-3.0-or-later
package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/internal/plugins"
)

// Catalog source, metadata and the browser's active document form one change.
// A failed update must not erase the previously installed version or user head.
func (s *Store) SavePlugin(ctx context.Context, id, source, metadata string) error {
	return s.changePlugin(ctx, id, source, metadata, false)
}
func (s *Store) RemovePlugin(ctx context.Context, id string) error {
	return s.changePlugin(ctx, id, "", "", true)
}
func (s *Store) changePlugin(ctx context.Context, id, source, metadata string, remove bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var head string
	err = tx.QueryRowContext(ctx, "SELECT value FROM kv WHERE key='custom_head'").Scan(&head)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var next string
	if remove {
		next, err = plugins.RemoveManaged(head, id)
	} else {
		next, err = plugins.UpsertManaged(head, id, source)
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(next) == "" {
		next = ""
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO kv(key,value) VALUES ('custom_head',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, next); err != nil {
		return err
	}
	if remove {
		if _, err = tx.ExecContext(ctx, "DELETE FROM kv WHERE key IN (?,?)", "plugin."+id, "pluginmeta."+id); err != nil {
			return err
		}
	} else {
		for key, value := range map[string]string{"plugin." + id: source, "pluginmeta." + id: metadata} {
			if _, err = tx.ExecContext(ctx, `INSERT INTO kv(key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
