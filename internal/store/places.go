package store

import (
	"context"
	"database/sql"
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

// ListLocations returns uid's locations in display order.
func (s *Store) ListLocations(ctx context.Context, uid int64) ([]Location, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+locationCols+` FROM locations l WHERE l.user_id = ? ORDER BY l.sort_order, l.name COLLATE NOCASE`, uid)
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

// GetLocation returns one of uid's locations (ErrNotFound for anyone else's).
func (s *Store) GetLocation(ctx context.Context, uid, id int64) (*Location, error) {
	return scanLocation(s.DB.QueryRowContext(ctx, `SELECT `+locationCols+` FROM locations l WHERE l.id = ? AND l.user_id = ?`, id, uid))
}

// FindLocationByName matches uid's locations case-insensitively.
func (s *Store) FindLocationByName(ctx context.Context, uid int64, name string) (*Location, error) {
	return scanLocation(s.DB.QueryRowContext(ctx, `SELECT `+locationCols+` FROM locations l WHERE l.user_id = ? AND l.name = ? COLLATE NOCASE`, uid, strings.TrimSpace(name)))
}

// CreateLocation inserts a location for uid at the end of their list.
func (s *Store) CreateLocation(ctx context.Context, uid int64, l *Location) error {
	var max int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) FROM locations WHERE user_id = ?`, uid).Scan(&max)
	l.SortOrder = max + 1
	res, err := s.DB.ExecContext(ctx, `INSERT INTO locations (user_id, name, description, sort_order) VALUES (?, ?, ?, ?)`, uid, strings.TrimSpace(l.Name), l.Description, l.SortOrder)
	if err != nil {
		return conflict(err)
	}
	l.ID, _ = res.LastInsertId()
	l.CreatedAt = Now()
	return nil
}

// UpdateLocation renames / re-describes one of uid's locations.
func (s *Store) UpdateLocation(ctx context.Context, uid int64, l *Location) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE locations SET name = ?, description = ? WHERE id = ? AND user_id = ?`, strings.TrimSpace(l.Name), l.Description, l.ID, uid)
	return affected(res, conflict(err))
}

// DeleteLocation removes one of uid's locations; its items keep existing with no location.
func (s *Store) DeleteLocation(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM locations WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderLocations sets sort_order of uid's locations from the given id sequence.
func (s *Store) ReorderLocations(ctx context.Context, uid int64, ids []int64) error {
	return s.reorder(ctx, "locations", uid, ids)
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

// ListCategories returns uid's categories in display order.
func (s *Store) ListCategories(ctx context.Context, uid int64) ([]Category, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+categoryCols+` FROM categories c WHERE c.user_id = ? ORDER BY c.sort_order, c.name COLLATE NOCASE`, uid)
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

// GetCategory returns one of uid's categories (ErrNotFound for anyone else's).
func (s *Store) GetCategory(ctx context.Context, uid, id int64) (*Category, error) {
	return scanCategory(s.DB.QueryRowContext(ctx, `SELECT `+categoryCols+` FROM categories c WHERE c.id = ? AND c.user_id = ?`, id, uid))
}

// FindCategoryByName matches uid's categories case-insensitively.
func (s *Store) FindCategoryByName(ctx context.Context, uid int64, name string) (*Category, error) {
	return scanCategory(s.DB.QueryRowContext(ctx, `SELECT `+categoryCols+` FROM categories c WHERE c.user_id = ? AND c.name = ? COLLATE NOCASE`, uid, strings.TrimSpace(name)))
}

// CreateCategory inserts a category for uid at the end of their list.
func (s *Store) CreateCategory(ctx context.Context, uid int64, c *Category) error {
	var max int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) FROM categories WHERE user_id = ?`, uid).Scan(&max)
	c.SortOrder = max + 1
	res, err := s.DB.ExecContext(ctx, `INSERT INTO categories (user_id, name, color, sort_order) VALUES (?, ?, ?, ?)`, uid, strings.TrimSpace(c.Name), c.Color, c.SortOrder)
	if err != nil {
		return conflict(err)
	}
	c.ID, _ = res.LastInsertId()
	c.CreatedAt = Now()
	return nil
}

// UpdateCategory renames / recolours one of uid's categories.
func (s *Store) UpdateCategory(ctx context.Context, uid int64, c *Category) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE categories SET name = ?, color = ? WHERE id = ? AND user_id = ?`, strings.TrimSpace(c.Name), c.Color, c.ID, uid)
	return affected(res, conflict(err))
}

// DeleteCategory removes one of uid's categories; its items keep existing with no category.
func (s *Store) DeleteCategory(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM categories WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderCategories sets sort_order of uid's categories from the given id sequence.
func (s *Store) ReorderCategories(ctx context.Context, uid int64, ids []int64) error {
	return s.reorder(ctx, "categories", uid, ids)
}

// reorder only touches uid's rows; ids of anyone else's are ignored.
func (s *Store) reorder(ctx context.Context, table string, uid int64, ids []int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET sort_order = ? WHERE id = ? AND user_id = ?`, i+1, id, uid); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ---------- tags ----------

// ListTags returns every distinct tag used by non-archived items uid can see, sorted.
func (s *Store) ListTags(ctx context.Context, uid int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT i.tags FROM items i WHERE i.archived = 0 AND i.tags <> '' AND `+visibleTo, uid, uid)
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

// affected turns "no row matched" into ErrNotFound (the row is someone else's
// or does not exist; the caller cannot tell which).
func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SeedDefaultCategories gives a new person the starter categories, once.
func (s *Store) SeedDefaultCategories(ctx context.Context, uid int64) error {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM categories WHERE user_id = ?`, uid).Scan(&n); err != nil || n > 0 {
		return err
	}
	for _, name := range []string{"Tools", "Electronics", "Household", "Food & drink", "Medicine", "Documents", "Clothing", "Outdoor"} {
		if err := s.CreateCategory(ctx, uid, &Category{Name: name}); err != nil && err != ErrConflict {
			return err
		}
	}
	return nil
}
