package store

import (
	"context"
	"strconv"
)

// Box-wide settings live in `settings` (app title, setup flags); everything a
// person tunes for themselves lives in `user_settings` under their user id.
var personalKeys = map[string]bool{"currency": true, "default_unit": true, "landing": true, "expiry_warn_days": true}

// GetSettings loads the box-wide settings merged with uid's own preferences,
// with defaults applied. uid 0 returns the box-wide part with default
// preferences (used before anyone is signed in).
func (s *Store) GetSettings(ctx context.Context, uid int64) (Settings, error) {
	st := Settings{Title: "Pocket Inventory", Currency: "EUR", DefaultUnit: "pcs", Landing: "items", ExpiryWarnDays: 30}
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value, 0 FROM settings
		UNION ALL SELECT key, value, 1 FROM user_settings WHERE user_id = ?`, uid)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		var personal int
		if err := rows.Scan(&k, &v, &personal); err != nil {
			return st, err
		}
		if personalKeys[k] != (personal == 1) {
			continue // a personal key never comes from the box-wide table and vice versa
		}
		switch k {
		case "title":
			if v != "" {
				st.Title = v
			}
		case "currency":
			if v != "" {
				st.Currency = v
			}
		case "default_unit":
			st.DefaultUnit = v
		case "landing":
			if v == "items" || v == "shopping" {
				st.Landing = v
			}
		case "expiry_warn_days":
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				st.ExpiryWarnDays = n
			}
		case "setup_complete":
			st.SetupComplete = v == "1"
		case "defaults_seeded":
			st.DefaultsSeeded = v == "1"
		}
	}
	return st, rows.Err()
}

// SaveGlobalSettings persists the box-wide part (title, setup flags).
func (s *Store) SaveGlobalSettings(ctx context.Context, st Settings) error {
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	return s.upsert(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, nil, map[string]string{
		"title":           st.Title,
		"setup_complete":  b(st.SetupComplete),
		"defaults_seeded": b(st.DefaultsSeeded),
	})
}

// SaveUserSettings persists uid's own preferences.
func (s *Store) SaveUserSettings(ctx context.Context, uid int64, st Settings) error {
	return s.upsert(ctx, `INSERT INTO user_settings (user_id, key, value) VALUES (?, ?, ?) ON CONFLICT(user_id, key) DO UPDATE SET value = excluded.value`, []any{uid}, map[string]string{
		"currency":         st.Currency,
		"default_unit":     st.DefaultUnit,
		"landing":          st.Landing,
		"expiry_warn_days": strconv.Itoa(st.ExpiryWarnDays),
	})
}

func (s *Store) upsert(ctx context.Context, q string, prefix []any, kv map[string]string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for k, v := range kv {
		args := append(append([]any{}, prefix...), k, v)
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
