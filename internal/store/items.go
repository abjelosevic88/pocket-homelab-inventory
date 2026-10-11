package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ItemFilter narrows ListItems.
type ItemFilter struct {
	Query      string
	LocationID int64 // 0 = any, -1 = no location
	CategoryID int64 // 0 = any, -1 = no category
	UserID     int64 // 0 = any, -1 = household (no owner), >0 = linked to that user
	Tag        string
	Barcode    string
	LowOnly    bool
	Archived   bool // include archived items (default: hide)
	Expiring   int  // >0: expiry within N days (or already expired)
	Sort       string
	Limit      int
}

const itemCols = `i.id, i.name, i.notes, i.category_id, i.location_id, i.quantity, i.unit, i.min_quantity,
	i.barcode, i.brand, i.model, i.serial, i.purchase_date, i.price, i.expiry_date, i.tags, i.url, i.photo,
	i.archived, i.created_at, i.updated_at, COALESCE(c.name, ''), COALESCE(l.name, '')`

const itemFrom = ` FROM items i LEFT JOIN categories c ON c.id = i.category_id LEFT JOIN locations l ON l.id = i.location_id `

func scanItem(row interface{ Scan(...any) error }) (*Item, error) {
	var it Item
	var cat, loc sql.NullInt64
	var archived int
	if err := row.Scan(&it.ID, &it.Name, &it.Notes, &cat, &loc, &it.Quantity, &it.Unit, &it.MinQuantity,
		&it.Barcode, &it.Brand, &it.Model, &it.Serial, &it.PurchaseDate, &it.Price, &it.ExpiryDate, &it.Tags, &it.URL, &it.Photo,
		&archived, &it.CreatedAt, &it.UpdatedAt, &it.CategoryName, &it.LocationName); err != nil {
		return nil, notFound(err)
	}
	if cat.Valid {
		v := cat.Int64
		it.CategoryID = &v
	}
	if loc.Valid {
		v := loc.Int64
		it.LocationID = &v
	}
	it.Archived = archived == 1
	it.Low = it.MinQuantity > 0 && it.Quantity <= it.MinQuantity
	it.UserIDs = []int64{}
	it.Users = []UserRef{}
	return &it, nil
}

// attachUsers fills UserIDs / Users for the given items in one query.
func (s *Store) attachUsers(ctx context.Context, items []*Item) error {
	if len(items) == 0 {
		return nil
	}
	byID := map[int64]*Item{}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		byID[it.ID] = it
		ids = append(ids, fmt.Sprintf("%d", it.ID))
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT iu.item_id, u.id, u.name, u.email FROM item_users iu JOIN users u ON u.id = iu.user_id
		WHERE iu.item_id IN (`+strings.Join(ids, ",")+`) ORDER BY u.name COLLATE NOCASE, u.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var itemID, uid int64
		var name, email string
		if err := rows.Scan(&itemID, &uid, &name, &email); err != nil {
			return err
		}
		if name == "" {
			name = email
		}
		if it := byID[itemID]; it != nil {
			it.UserIDs = append(it.UserIDs, uid)
			it.Users = append(it.Users, UserRef{ID: uid, Name: name})
		}
	}
	return rows.Err()
}

// ListItems returns items matching the filter.
func (s *Store) ListItems(ctx context.Context, f ItemFilter) ([]Item, error) {
	var where []string
	var args []any
	if !f.Archived {
		where = append(where, "i.archived = 0")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := "%" + q + "%"
		where = append(where, `(i.name LIKE ? OR i.notes LIKE ? OR i.brand LIKE ? OR i.model LIKE ? OR i.serial LIKE ? OR i.barcode LIKE ? OR i.tags LIKE ? OR l.name LIKE ? OR c.name LIKE ?)`)
		for i := 0; i < 9; i++ {
			args = append(args, like)
		}
	}
	switch {
	case f.LocationID > 0:
		where = append(where, "i.location_id = ?")
		args = append(args, f.LocationID)
	case f.LocationID < 0:
		where = append(where, "i.location_id IS NULL")
	}
	switch {
	case f.CategoryID > 0:
		where = append(where, "i.category_id = ?")
		args = append(args, f.CategoryID)
	case f.CategoryID < 0:
		where = append(where, "i.category_id IS NULL")
	}
	switch {
	case f.UserID > 0:
		where = append(where, "EXISTS (SELECT 1 FROM item_users iu WHERE iu.item_id = i.id AND iu.user_id = ?)")
		args = append(args, f.UserID)
	case f.UserID < 0:
		where = append(where, "NOT EXISTS (SELECT 1 FROM item_users iu WHERE iu.item_id = i.id)")
	}
	if f.Tag != "" {
		where = append(where, `(',' || REPLACE(LOWER(i.tags), ', ', ',') || ',') LIKE ?`)
		args = append(args, "%,"+strings.ToLower(strings.TrimSpace(f.Tag))+",%")
	}
	if f.Barcode != "" {
		where = append(where, "i.barcode = ?")
		args = append(args, strings.TrimSpace(f.Barcode))
	}
	if f.LowOnly {
		where = append(where, "i.min_quantity > 0 AND i.quantity <= i.min_quantity")
	}
	if f.Expiring > 0 {
		where = append(where, "i.expiry_date <> '' AND i.expiry_date <= ?")
		args = append(args, time.Now().AddDate(0, 0, f.Expiring).Format("2006-01-02"))
	}
	order := "i.name COLLATE NOCASE"
	switch f.Sort {
	case "updated":
		order = "i.updated_at DESC"
	case "created":
		order = "i.created_at DESC"
	case "quantity":
		order = "i.quantity ASC, i.name COLLATE NOCASE"
	case "location":
		order = "l.sort_order, l.name COLLATE NOCASE, i.name COLLATE NOCASE"
	case "expiry":
		order = "CASE WHEN i.expiry_date = '' THEN 1 ELSE 0 END, i.expiry_date, i.name COLLATE NOCASE"
	}
	q := `SELECT ` + itemCols + itemFrom
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY " + order
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ptrs := []*Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		ptrs = append(ptrs, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if err := s.attachUsers(ctx, ptrs); err != nil {
		return nil, err
	}
	out := make([]Item, len(ptrs))
	for i, p := range ptrs {
		out[i] = *p
	}
	return out, nil
}

// GetItem returns one item.
func (s *Store) GetItem(ctx context.Context, id int64) (*Item, error) {
	it, err := scanItem(s.DB.QueryRowContext(ctx, `SELECT `+itemCols+itemFrom+` WHERE i.id = ?`, id))
	if err != nil {
		return nil, err
	}
	if err := s.attachUsers(ctx, []*Item{it}); err != nil {
		return nil, err
	}
	return it, nil
}

func setItemUsers(ctx context.Context, tx *sql.Tx, itemID int64, userIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM item_users WHERE item_id = ?`, itemID); err != nil {
		return err
	}
	seen := map[int64]bool{}
	for _, uid := range userIDs {
		if uid <= 0 || seen[uid] {
			continue
		}
		seen[uid] = true
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO item_users (item_id, user_id) SELECT ?, id FROM users WHERE id = ?`, itemID, uid); err != nil {
			return err
		}
	}
	return nil
}

func nullID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

// CreateItem inserts an item and records the opening quantity as a movement.
func (s *Store) CreateItem(ctx context.Context, it *Item, actor int64) error {
	it.Tags = NormalizeTags(it.Tags)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	res, err := tx.ExecContext(ctx, `INSERT INTO items (name, notes, category_id, location_id, quantity, unit, min_quantity, barcode, brand, model, serial,
		purchase_date, price, expiry_date, tags, url, photo, archived) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(it.Name), it.Notes, it.CategoryID, it.LocationID, it.Quantity, it.Unit, it.MinQuantity, strings.TrimSpace(it.Barcode),
		it.Brand, it.Model, it.Serial, it.PurchaseDate, it.Price, it.ExpiryDate, it.Tags, it.URL, it.Photo, boolInt(it.Archived))
	if err != nil {
		return err
	}
	it.ID, _ = res.LastInsertId()
	if err := setItemUsers(ctx, tx, it.ID, it.UserIDs); err != nil {
		return err
	}
	if it.Quantity != 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO movements (item_id, user_id, delta, quantity_after, note) VALUES (?, ?, ?, ?, 'added')`, it.ID, nullID(actor), it.Quantity, it.Quantity); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fresh, err := s.GetItem(ctx, it.ID)
	if err == nil {
		*it = *fresh
	}
	return nil
}

// UpdateItem saves every editable field. A changed quantity is logged as a movement.
func (s *Store) UpdateItem(ctx context.Context, it *Item, note string, actor int64) error {
	it.Tags = NormalizeTags(it.Tags)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var before float64
	if err := tx.QueryRowContext(ctx, `SELECT quantity FROM items WHERE id = ?`, it.ID).Scan(&before); err != nil {
		return notFound(err)
	}
	_, err = tx.ExecContext(ctx, `UPDATE items SET name = ?, notes = ?, category_id = ?, location_id = ?, quantity = ?, unit = ?, min_quantity = ?, barcode = ?,
		brand = ?, model = ?, serial = ?, purchase_date = ?, price = ?, expiry_date = ?, tags = ?, url = ?, photo = ?, archived = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(it.Name), it.Notes, it.CategoryID, it.LocationID, it.Quantity, it.Unit, it.MinQuantity, strings.TrimSpace(it.Barcode),
		it.Brand, it.Model, it.Serial, it.PurchaseDate, it.Price, it.ExpiryDate, it.Tags, it.URL, it.Photo, boolInt(it.Archived), Now(), it.ID)
	if err != nil {
		return err
	}
	if err := setItemUsers(ctx, tx, it.ID, it.UserIDs); err != nil {
		return err
	}
	if before != it.Quantity {
		if note == "" {
			note = "edited"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO movements (item_id, user_id, delta, quantity_after, note) VALUES (?, ?, ?, ?, ?)`, it.ID, nullID(actor), it.Quantity-before, it.Quantity, note); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fresh, err := s.GetItem(ctx, it.ID)
	if err == nil {
		*it = *fresh
	}
	return nil
}

// AdjustQuantity adds delta (negative to take) and logs the movement. Quantity never goes below zero.
func (s *Store) AdjustQuantity(ctx context.Context, id int64, delta float64, note string, actor int64) (*Item, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck
	var before float64
	if err := tx.QueryRowContext(ctx, `SELECT quantity FROM items WHERE id = ?`, id).Scan(&before); err != nil {
		return nil, notFound(err)
	}
	after := before + delta
	if after < 0 {
		after = 0
		delta = -before
	}
	if _, err := tx.ExecContext(ctx, `UPDATE items SET quantity = ?, updated_at = ? WHERE id = ?`, after, Now(), id); err != nil {
		return nil, err
	}
	if delta != 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO movements (item_id, user_id, delta, quantity_after, note) VALUES (?, ?, ?, ?, ?)`, id, nullID(actor), delta, after, note); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetItem(ctx, id)
}

// SetQuantity sets an absolute quantity and logs the difference.
func (s *Store) SetQuantity(ctx context.Context, id int64, qty float64, note string, actor int64) (*Item, error) {
	var before float64
	if err := s.DB.QueryRowContext(ctx, `SELECT quantity FROM items WHERE id = ?`, id).Scan(&before); err != nil {
		return nil, notFound(err)
	}
	if qty < 0 {
		qty = 0
	}
	return s.AdjustQuantity(ctx, id, qty-before, note, actor)
}

// SetArchived hides or restores an item.
func (s *Store) SetArchived(ctx context.Context, id int64, archived bool) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE items SET archived = ?, updated_at = ? WHERE id = ?`, boolInt(archived), Now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPhoto stores the photo file name ("" clears it).
func (s *Store) SetPhoto(ctx context.Context, id int64, photo string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE items SET photo = ?, updated_at = ? WHERE id = ?`, photo, Now(), id)
	return err
}

// DeleteItem removes an item and its movements.
func (s *Store) DeleteItem(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const movementCols = `m.id, m.item_id, m.user_id, COALESCE(NULLIF(u.name, ''), u.email, ''), m.delta, m.quantity_after, m.note, m.created_at`
const movementFrom = ` FROM movements m LEFT JOIN users u ON u.id = m.user_id `

func scanMovement(row interface{ Scan(...any) error }, m *Movement) error {
	var uid sql.NullInt64
	if err := row.Scan(&m.ID, &m.ItemID, &uid, &m.UserName, &m.Delta, &m.QuantityAfter, &m.Note, &m.CreatedAt); err != nil {
		return err
	}
	if uid.Valid {
		v := uid.Int64
		m.UserID = &v
	}
	return nil
}

// ListMovements returns the newest movements of an item.
func (s *Store) ListMovements(ctx context.Context, itemID int64, limit int) ([]Movement, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+movementCols+movementFrom+fmt.Sprintf(` WHERE m.item_id = ? ORDER BY m.id DESC LIMIT %d`, limit), itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Movement{}
	for rows.Next() {
		var m Movement
		if err := scanMovement(rows, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ActivityRow is a movement with the item's name, for the activity feed.
type ActivityRow struct {
	Movement
	ItemName string `json:"item_name"`
	Unit     string `json:"unit"`
}

// RecentActivity returns the latest movements across all items.
func (s *Store) RecentActivity(ctx context.Context, limit int) ([]ActivityRow, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+movementCols+`, i.name, i.unit`+movementFrom+fmt.Sprintf(`JOIN items i ON i.id = m.item_id ORDER BY m.id DESC LIMIT %d`, limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ActivityRow{}
	for rows.Next() {
		var a ActivityRow
		var uid sql.NullInt64
		if err := rows.Scan(&a.ID, &a.ItemID, &uid, &a.UserName, &a.Delta, &a.QuantityAfter, &a.Note, &a.CreatedAt, &a.ItemName, &a.Unit); err != nil {
			return nil, err
		}
		if uid.Valid {
			v := uid.Int64
			a.UserID = &v
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DashboardStats computes the widget numbers.
func (s *Store) DashboardStats(ctx context.Context, currency string, warnDays int) (*Dashboard, error) {
	d := &Dashboard{Currency: currency}
	today := Today()
	soon := time.Now().AddDate(0, 0, warnDays).Format("2006-01-02")
	err := s.DB.QueryRowContext(ctx, `SELECT
		COUNT(1),
		COALESCE(SUM(quantity), 0),
		COALESCE(SUM(CASE WHEN min_quantity > 0 AND quantity <= min_quantity THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(quantity * price), 0),
		COALESCE(SUM(CASE WHEN expiry_date <> '' AND expiry_date > ? AND expiry_date <= ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN expiry_date <> '' AND expiry_date <= ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN substr(updated_at, 1, 10) = ? THEN 1 ELSE 0 END), 0)
		FROM items WHERE archived = 0`, today, soon, today, time.Now().UTC().Format("2006-01-02")).Scan(
		&d.Items, &d.QuantityTotal, &d.LowStock, &d.TotalValue, &d.Expiring, &d.Expired, &d.UpdatedToday)
	if err != nil {
		return nil, err
	}
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM items WHERE archived = 1`).Scan(&d.Archived)
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM locations`).Scan(&d.Locations)
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM categories`).Scan(&d.Categories)
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&d.Users)
	return d, nil
}

func sortStrings(ss []string) {
	sort.Slice(ss, func(i, j int) bool { return strings.ToLower(ss[i]) < strings.ToLower(ss[j]) })
}
