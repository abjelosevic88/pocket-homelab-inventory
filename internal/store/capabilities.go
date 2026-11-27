package store

import (
	"context"
	"database/sql"
	"strings"
	"unicode/utf8"
)

// Queries behind the Pocket capability routes (server/capabilities.go): one
// day of the activity log, and the check that keeps a quick line from adding
// an item that is already there.

// DayChange is what happened to one item in a stretch of time: its quantity
// changes added up, and whether the item itself is new.
type DayChange struct {
	ItemID   int64
	Name     string
	Unit     string
	Location string
	New      bool     // the item was created in that time
	Net      float64  // the changes added up
	After    float64  // how many there were after the last change
	At       string   // when it last changed (when it was created, without a change)
	Notes    []string // the notes of the changes, each once, oldest first
}

// dayMovements reads one stretch of the log with each entry's item. CROSS
// JOIN keeps the log first, so the stretch is found by time
// (movements_created) however many items and entries there are.
const dayMovements = `SELECT i.id, i.name, i.unit, COALESCE(l.name, ''), i.created_at, m.delta, m.quantity_after, m.note, m.created_at
	FROM movements m CROSS JOIN items i ON i.id = m.item_id LEFT JOIN locations l ON l.id = i.location_id
	WHERE m.created_at >= ? AND m.created_at < ? AND ` + visibleTo + ` ORDER BY m.created_at, m.id`

// DayChanges returns, per item uid can see, the movements with from <=
// created_at < to (stored UTC timestamps, compared as text) and the items
// created in that time, in the order things happened. Only that stretch of
// the log is read.
func (s *Store) DayChanges(ctx context.Context, uid int64, from, to string) ([]DayChange, error) {
	rows, err := s.DB.QueryContext(ctx, dayMovements, from, to, uid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DayChange{}
	at := map[int64]int{} // item id → its place in out
	for rows.Next() {
		var c DayChange
		var created, note string
		var delta float64
		if err := rows.Scan(&c.ItemID, &c.Name, &c.Unit, &c.Location, &created, &delta, &c.After, &note, &c.At); err != nil {
			return nil, err
		}
		i, ok := at[c.ItemID]
		if !ok {
			c.New = created >= from && created < to
			out = append(out, c)
			i = len(out) - 1
			at[c.ItemID] = i
		}
		d := &out[i]
		d.Net += delta
		d.After, d.At = c.After, c.At
		if note = strings.TrimSpace(note); note != "" && !containsString(d.Notes, note) {
			d.Notes = append(d.Notes, note)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	// an item added with none on hand has no movement yet
	fresh, err := s.DB.QueryContext(ctx, `SELECT i.id, i.name, i.unit, COALESCE(l.name, ''), i.created_at
		FROM items i LEFT JOIN locations l ON l.id = i.location_id
		WHERE i.created_at >= ? AND i.created_at < ? AND `+visibleTo+` ORDER BY i.id`, from, to, uid, uid)
	if err != nil {
		return nil, err
	}
	defer fresh.Close()
	for fresh.Next() {
		c := DayChange{New: true}
		if err := fresh.Scan(&c.ItemID, &c.Name, &c.Unit, &c.Location, &c.At); err != nil {
			return nil, err
		}
		if _, ok := at[c.ItemID]; !ok {
			out = append(out, c)
		}
	}
	return out, fresh.Err()
}

func containsString(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// ItemNamed returns the name of an item uid can see (their own or shared
// with them, not archived) that is called name, capitals aside; "" when there
// is none. except leaves one item out (the one just added). It reads through
// q, so it can run inside the transaction that adds the item.
func ItemNamed(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, uid int64, name string, except int64) (string, error) {
	name = strings.TrimSpace(name)
	// SQLite folds only A–Z; names of the same length are compared here instead
	rows, err := q.QueryContext(ctx, `SELECT i.name FROM items i WHERE i.archived = 0 AND i.id <> ? AND length(i.name) = ? AND `+visibleTo+` ORDER BY i.id`,
		except, utf8.RuneCountInString(name), uid, uid)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var have string
		if err := rows.Scan(&have); err != nil {
			return "", err
		}
		if strings.EqualFold(have, name) {
			return have, nil
		}
	}
	return "", rows.Err()
}
