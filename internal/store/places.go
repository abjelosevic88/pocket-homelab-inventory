package store

import (
	"context"
	"strings"
)

// ---------- locations ----------

const locationCols = `l.id, l.name, l.description, l.sort_order, l.created_at,
	(SELECT COUNT(1) FROM items i WHERE i.location_id = l.id AND i.archived = 0)`

func scanLocation(row interface{ Scan(...any) error }) (*Location, error) {
	var l Location
	if err := row.Scan(&l.ID, &l.Name, &l.Description, &l.SortOrder, &l.CreatedAt, &l.ItemCount); err != nil {
		return nil, notFound(err)
	}
	return &l, nil
}

// ListLocations returns all locations in display order.
func (s *Store) ListLocations(ctx context.Context) ([]Location, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+locationCols+` FROM locations l ORDER BY l.sort_order, l.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Location{}
	for rows.Next() {
		l, err := scanLocation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// GetLocation returns one location.
func (s *Store) GetLocation(ctx context.Context, id int64) (*Location, error) {
	return scanLocation(s.DB.QueryRowContext(ctx, `SELECT `+locationCols+` FROM locations l WHERE l.id = ?`, id))
}

// FindLocationByName matches case-insensitively.
func (s *Store) FindLocationByName(ctx context.Context, name string) (*Location, error) {
	return scanLocation(s.DB.QueryRowContext(ctx, `SELECT `+locationCols+` FROM locations l WHERE l.name = ? COLLATE NOCASE`, strings.TrimSpace(name)))
}

// CreateLocation inserts a location at the end of the list.
func (s *Store) CreateLocation(ctx context.Context, l *Location) error {
	var max int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) FROM locations`).Scan(&max)
	l.SortOrder = max + 1
	res, err := s.DB.ExecContext(ctx, `INSERT INTO locations (name, description, sort_order) VALUES (?, ?, ?)`, strings.TrimSpace(l.Name), l.Description, l.SortOrder)
	if err != nil {
		return conflict(err)
	}
	l.ID, _ = res.LastInsertId()
	l.CreatedAt = Now()
	return nil
}

// UpdateLocation renames / re-describes a location.
func (s *Store) UpdateLocation(ctx context.Context, l *Location) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE locations SET name = ?, description = ? WHERE id = ?`, strings.TrimSpace(l.Name), l.Description, l.ID)
	return conflict(err)
}

// DeleteLocation removes a location; its items keep existing with no location.
func (s *Store) DeleteLocation(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM locations WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderLocations sets sort_order from the given id sequence.
func (s *Store) ReorderLocations(ctx context.Context, ids []int64) error {
	return s.reorder(ctx, "locations", ids)
}

// ---------- categories ----------

const categoryCols = `c.id, c.name, c.color, c.sort_order, c.created_at,
	(SELECT COUNT(1) FROM items i WHERE i.category_id = c.id AND i.archived = 0)`

func scanCategory(row interface{ Scan(...any) error }) (*Category, error) {
	var c Category
	if err := row.Scan(&c.ID, &c.Name, &c.Color, &c.SortOrder, &c.CreatedAt, &c.ItemCount); err != nil {
		return nil, notFound(err)
	}
	return &c, nil
}

// ListCategories returns all categories in display order.
func (s *Store) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+categoryCols+` FROM categories c ORDER BY c.sort_order, c.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetCategory returns one category.
func (s *Store) GetCategory(ctx context.Context, id int64) (*Category, error) {
	return scanCategory(s.DB.QueryRowContext(ctx, `SELECT `+categoryCols+` FROM categories c WHERE c.id = ?`, id))
}

// FindCategoryByName matches case-insensitively.
func (s *Store) FindCategoryByName(ctx context.Context, name string) (*Category, error) {
	return scanCategory(s.DB.QueryRowContext(ctx, `SELECT `+categoryCols+` FROM categories c WHERE c.name = ? COLLATE NOCASE`, strings.TrimSpace(name)))
}

// CreateCategory inserts a category at the end of the list.
func (s *Store) CreateCategory(ctx context.Context, c *Category) error {
	var max int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) FROM categories`).Scan(&max)
	c.SortOrder = max + 1
	res, err := s.DB.ExecContext(ctx, `INSERT INTO categories (name, color, sort_order) VALUES (?, ?, ?)`, strings.TrimSpace(c.Name), c.Color, c.SortOrder)
	if err != nil {
		return conflict(err)
	}
	c.ID, _ = res.LastInsertId()
	c.CreatedAt = Now()
	return nil
}

// UpdateCategory renames / recolours a category.
func (s *Store) UpdateCategory(ctx context.Context, c *Category) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE categories SET name = ?, color = ? WHERE id = ?`, strings.TrimSpace(c.Name), c.Color, c.ID)
	return conflict(err)
}

// DeleteCategory removes a category; its items keep existing with no category.
func (s *Store) DeleteCategory(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM categories WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderCategories sets sort_order from the given id sequence.
func (s *Store) ReorderCategories(ctx context.Context, ids []int64) error {
	return s.reorder(ctx, "categories", ids)
}

func (s *Store) reorder(ctx context.Context, table string, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET sort_order = ? WHERE id = ?`, i+1, id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ---------- tags ----------

// ListTags returns every distinct tag used by non-archived items, sorted.
func (s *Store) ListTags(ctx context.Context) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT tags FROM items WHERE archived = 0 AND tags <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	out := []string{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		for _, tag := range SplitTags(t) {
			if !seen[strings.ToLower(tag)] {
				seen[strings.ToLower(tag)] = true
				out = append(out, tag)
			}
		}
	}
	sortStrings(out)
	return out, rows.Err()
}

// SplitTags turns "a, b ,c" into ["a","b","c"].
func SplitTags(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// NormalizeTags trims, de-duplicates and re-joins tags.
func NormalizeTags(s string) string {
	seen := map[string]bool{}
	var out []string
	for _, t := range SplitTags(s) {
		k := strings.ToLower(t)
		if !seen[k] {
			seen[k] = true
			out = append(out, t)
		}
	}
	return strings.Join(out, ", ")
}
