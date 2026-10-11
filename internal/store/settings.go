package store

import (
	"context"
	"strconv"
)

// GetSettings loads all settings with defaults applied.
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	st := Settings{Title: "Pocket Inventory", Currency: "EUR", DefaultUnit: "pcs", Landing: "items", ExpiryWarnDays: 30}
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return st, err
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

// SaveSettings persists all settings.
func (s *Store) SaveSettings(ctx context.Context, st Settings) error {
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	kv := map[string]string{
		"title":            st.Title,
		"currency":         st.Currency,
		"default_unit":     st.DefaultUnit,
		"landing":          st.Landing,
		"expiry_warn_days": strconv.Itoa(st.ExpiryWarnDays),
		"setup_complete":   b(st.SetupComplete),
		"defaults_seeded":  b(st.DefaultsSeeded),
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for k, v := range kv {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
