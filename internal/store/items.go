package store

import (
	"context"
	"database/sql"
	"fmt"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"sort"
	"strings"
	"time"
)

// visibleTo is the per-person rule for items: you see an item you own or one
// that is shared with you through item_users. Takes (uid, uid).
const visibleTo = `(i.user_id = ? OR EXISTS (SELECT 1 FROM item_users v WHERE v.item_id = i.id AND v.user_id = ?))`

// ItemFilter narrows ListItems.
type ItemFilter struct {
	OwnedOnly  bool // only items the viewer owns (exports, import matching), not ones shared with them
	Query      string
	LocationID int64 // 0 = any, -1 = no location
	CategoryID int64 // 0 = any, -1 = no category
	UserID     int64 // 0 = any, -1 = not shared with anyone, >0 = linked to that user
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
	i.archived, i.created_at, i.updated_at, COALESCE(i.user_id, 0), COALESCE(c.name, ''), COALESCE(l.name, '')`

const itemFrom = ` FROM items i LEFT JOIN categories c ON c.id = i.category_id LEFT JOIN locations l ON l.id = i.location_id `

func scanItem(row interface{ Scan(...any) error }) (*Item, error) {
	var it Item
	var cat, loc sql.NullInt64
	var archived int
	if err := row.Scan(&it.ID, &it.Name, &it.Notes, &cat, &loc, &it.Quantity, &it.Unit, &it.MinQuantity,
		&it.Barcode, &it.Brand, &it.Model, &it.Serial, &it.PurchaseDate, &it.Price, &it.ExpiryDate, &it.Tags, &it.URL, &it.Photo,
		&archived, &it.CreatedAt, &it.UpdatedAt, &it.OwnerID, &it.CategoryName, &it.LocationName); err != nil {
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
	return attachItemUsers(ctx, s.DB, items)
}
func attachItemUsers(ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, items []*Item) error {
	if len(items) == 0 {
		return nil
	}
	byID := map[int64]*Item{}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		byID[it.ID] = it
		ids = append(ids, fmt.Sprintf("%d", it.ID))
	}
	rows, err := db.QueryContext(ctx, `SELECT iu.item_id, u.id, u.name, u.email FROM item_users iu JOIN users u ON u.id = iu.user_id
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

// ListItems returns the items uid can see that match the filter.
func (s *Store) ListItems(ctx context.Context, uid int64, f ItemFilter) ([]Item, error) {
	var where []string
	var args []any
	if f.OwnedOnly {
		where = append(where, "i.user_id = ?")
		args = append(args, uid)
	} else {
		where = append(where, visibleTo)
		args = append(args, uid, uid)
	}
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
	q := `SELECT ` + itemCols + itemFrom + " WHERE " + strings.Join(where, " AND ")
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

// GetItem returns one item uid can see; anyone else's is ErrNotFound.
func (s *Store) GetItem(ctx context.Context, uid, id int64) (*Item, error) {
	it, err := scanItem(s.DB.QueryRowContext(ctx, `SELECT `+itemCols+itemFrom+` WHERE i.id = ? AND `+visibleTo, id, uid, uid))
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

// ownLocation / ownCategory keep an item's place and category inside its
// owner's own lists: an id belonging to anyone else becomes NULL.
const ownLocation = `(SELECT id FROM locations WHERE id = ? AND user_id = ?)`
const ownCategory = `(SELECT id FROM categories WHERE id = ? AND user_id = ?)`

// CreateItem inserts an item owned by actor and records the opening quantity as a movement.
func (s *Store) CreateItem(ctx context.Context, it *Item, actor int64, after ...core.CommitHook) error {
	return s.CreateItemOperation(ctx, it, actor, nil, after...)
}
func (s *Store) CreateItemOperation(ctx context.Context, it *Item, actor int64, op *core.Operation, after ...core.CommitHook) error {
	it.Name = strings.TrimSpace(it.Name)
	it.OwnerID = actor
	it.Tags = NormalizeTags(it.Tags)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var previous Item
	if ok, err := op.Replay(ctx, tx, &previous); err != nil {
		return err
	} else if ok {
		var id int64
		if err := tx.QueryRowContext(ctx, `SELECT i.id`+itemFrom+` WHERE i.id=? AND `+visibleTo, previous.ID, actor, actor).Scan(&id); err != nil {
			return notFound(err)
		}
		*it = previous
		return nil
	}

	if err := tx.QueryRowContext(ctx, `UPDATE item_sequence SET value=MAX(value,(SELECT COALESCE(MAX(id),0) FROM items))+1 WHERE singleton=1 RETURNING value`).Scan(&it.ID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO items (id, user_id, name, notes, category_id, location_id, quantity, unit, min_quantity, barcode, brand, model, serial,
		purchase_date, price, expiry_date, tags, url, photo, archived) VALUES (?, ?, ?, ?, `+ownCategory+`, `+ownLocation+`, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.ID, nullID(actor), strings.TrimSpace(it.Name), it.Notes, it.CategoryID, actor, it.LocationID, actor, it.Quantity, it.Unit, it.MinQuantity, strings.TrimSpace(it.Barcode),
		it.Brand, it.Model, it.Serial, it.PurchaseDate, it.Price, it.ExpiryDate, it.Tags, it.URL, it.Photo, boolInt(it.Archived))
	if err != nil {
		return err
	}
	if err := setItemUsers(ctx, tx, it.ID, it.UserIDs); err != nil {
		return err
	}
	if it.Quantity != 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO movements (item_id, user_id, delta, quantity_after, note) VALUES (?, ?, ?, ?, 'added')`, it.ID, nullID(actor), it.Quantity, it.Quantity); err != nil {
			return err
		}
	}
	fresh, err := scanItem(tx.QueryRowContext(ctx, `SELECT `+itemCols+itemFrom+` WHERE i.id=?`, it.ID))
	if err != nil {
		return err
	}
	if err := attachItemUsers(ctx, tx, []*Item{fresh}); err != nil {
		return err
	}
	*it = *fresh
	if err := op.Save(ctx, tx, it); err != nil {
		return err
	}

	if err := core.Commit(tx, after...); err != nil {
		return err
	}
	return nil
}

// UpdateItem saves every editable field of an item actor can see. A changed
// quantity is logged as a movement. Place, category and who it is shared with
// belong to the item's owner: when someone it is shared with saves it, those
// three stay as they were.
func (s *Store) UpdateItem(ctx context.Context, it *Item, note string, actor int64) error {
	it.Tags = NormalizeTags(it.Tags)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var before float64
	var owner int64
	var curLoc, curCat sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT i.quantity, COALESCE(i.user_id, 0), i.location_id, i.category_id FROM items i WHERE i.id = ? AND `+visibleTo, it.ID, actor, actor).
		Scan(&before, &owner, &curLoc, &curCat); err != nil {
		return notFound(err)
	}
	isOwner := owner == actor
	if !isOwner {
		it.LocationID, it.CategoryID = nil, nil
		if curLoc.Valid {
			v := curLoc.Int64
			it.LocationID = &v
		}
		if curCat.Valid {
			v := curCat.Int64
			it.CategoryID = &v
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE items SET name = ?, notes = ?, category_id = `+ownCategory+`, location_id = `+ownLocation+`, quantity = ?, unit = ?, min_quantity = ?, barcode = ?,
		brand = ?, model = ?, serial = ?, purchase_date = ?, price = ?, expiry_date = ?, tags = ?, url = ?, photo = ?, archived = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(it.Name), it.Notes, it.CategoryID, owner, it.LocationID, owner, it.Quantity, it.Unit, it.MinQuantity, strings.TrimSpace(it.Barcode),
		it.Brand, it.Model, it.Serial, it.PurchaseDate, it.Price, it.ExpiryDate, it.Tags, it.URL, it.Photo, boolInt(it.Archived), Now(), it.ID)
	if err != nil {
		return err
	}
	if isOwner {
		if err := setItemUsers(ctx, tx, it.ID, it.UserIDs); err != nil {
			return err
		}
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
	fresh, err := s.GetItem(ctx, actor, it.ID)
	if err == nil {
		*it = *fresh
	}
	return nil
}

// AdjustQuantity adds delta (negative to take) to an item actor can see and
// logs the movement. Quantity never goes below zero.
func (s *Store) AdjustQuantity(ctx context.Context, id int64, delta float64, note string, actor int64) (*Item, error) {
	return s.AdjustQuantityOperation(ctx, id, &delta, nil, note, actor, nil, nil)
}

// SetQuantity computes its delta under the same write transaction, so a
// concurrent adjustment cannot turn an absolute quantity into an addition.
func (s *Store) SetQuantity(ctx context.Context, id int64, qty float64, note string, actor int64) (*Item, error) {
	return s.AdjustQuantityOperation(ctx, id, nil, &qty, note, actor, nil, nil)
}

func (s *Store) AdjustQuantityOperation(ctx context.Context, id int64, delta, quantity *float64, note string, actor int64, op *core.Operation, after func(*sql.Tx, *Item) error) (*Item, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Check access even on replay: a saved receipt is not an access grant.
	it, err := scanItem(tx.QueryRowContext(ctx, `SELECT `+itemCols+itemFrom+` WHERE i.id=? AND `+visibleTo, id, actor, actor))
	if err != nil {
		return nil, err
	}
	var replay Item
	if ok, err := op.Replay(ctx, tx, &replay); err != nil {
		return nil, err
	} else if ok {
		return &replay, nil
	}
	before := it.Quantity
	if quantity != nil {
		it.Quantity = *quantity
	} else if delta != nil {
		it.Quantity += *delta
	} else {
		return nil, fmt.Errorf("quantity or delta required")
	}
	if it.Quantity < 0 {
		it.Quantity = 0
	}
	change := it.Quantity - before
	it.Low = it.MinQuantity > 0 && it.Quantity <= it.MinQuantity
	it.UpdatedAt = Now()
	if _, err = tx.ExecContext(ctx, `UPDATE items SET quantity=?,updated_at=? WHERE id=?`, it.Quantity, it.UpdatedAt, id); err != nil {
		return nil, err
	}
	if change != 0 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO movements(item_id,user_id,delta,quantity_after,note) VALUES(?,?,?,?,?)`, id, nullID(actor), change, it.Quantity, note); err != nil {
			return nil, err
		}
	}
	if err = attachItemUsers(ctx, tx, []*Item{it}); err != nil {
		return nil, err
	}
	if err = op.Save(ctx, tx, it); err != nil {
		return nil, err
	}
	if after != nil {
		if err = after(tx, it); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return it, nil
}

// SetArchived hides or restores an item uid can see.
func (s *Store) SetArchived(ctx context.Context, uid, id int64, archived bool, after ...core.CommitHook) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE items AS i SET archived=?,updated_at=? WHERE i.id=? AND `+visibleTo, boolInt(archived), Now(), id, uid, uid)
	if err = affected(res, err); err != nil {
		return err
	}
	return core.Commit(tx, after...)
}

// SetPhoto stores the photo file name ("" clears it) on an item uid can see.
func (s *Store) SetPhoto(ctx context.Context, uid, id int64, photo string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE items AS i SET photo = ?, updated_at = ? WHERE i.id = ? AND `+visibleTo, photo, Now(), id, uid, uid)
	return affected(res, err)
}

// DeleteItem removes one of uid's items and its movements. Only the owner may
// delete: someone it is shared with gets ErrForbidden, anyone else ErrNotFound.
func (s *Store) DeleteItem(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM items WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := s.GetItem(ctx, uid, id); err == nil {
			return ErrForbidden
		}
		return ErrNotFound
	}
	return nil
}

// OwnedPhotos lists the photo files of every item uid owns (removed from disk
// when the account is deleted, since its items go with it).
func (s *Store) OwnedPhotos(ctx context.Context, uid int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT photo FROM items WHERE user_id = ? AND photo <> ''`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
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

// ListMovements returns the newest movements of an item uid can see.
func (s *Store) ListMovements(ctx context.Context, uid, itemID int64, limit int) ([]Movement, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+movementCols+movementFrom+`JOIN items i ON i.id = m.item_id`+fmt.Sprintf(` WHERE m.item_id = ? AND `+visibleTo+` ORDER BY m.id DESC LIMIT %d`, limit), itemID, uid, uid)
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

// RecentActivity returns the latest movements of the items uid can see
// (ownedOnly: only of the items uid owns).
func (s *Store) RecentActivity(ctx context.Context, uid int64, limit int, ownedOnly bool) ([]ActivityRow, error) {
	if limit <= 0 {
		limit = 30
	}
	scope, args := visibleTo, []any{uid, uid}
	if ownedOnly {
		scope, args = "i.user_id = ?", []any{uid}
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+movementCols+`, i.name, i.unit`+movementFrom+fmt.Sprintf(`JOIN items i ON i.id = m.item_id WHERE `+scope+` ORDER BY m.id DESC LIMIT %d`, limit), args...)
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

// DashboardStats computes the widget numbers over the items uid can see.
func (s *Store) DashboardStats(ctx context.Context, uid int64, currency string, warnDays int) (*Dashboard, error) {
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
		FROM items i WHERE i.archived = 0 AND `+visibleTo, today, soon, today, time.Now().UTC().Format("2006-01-02"), uid, uid).Scan(
		&d.Items, &d.QuantityTotal, &d.LowStock, &d.TotalValue, &d.Expiring, &d.Expired, &d.UpdatedToday)
	if err != nil {
		return nil, err
	}
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM items i WHERE i.archived = 1 AND `+visibleTo, uid, uid).Scan(&d.Archived)
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM locations WHERE user_id = ?`, uid).Scan(&d.Locations)
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM categories WHERE user_id = ?`, uid).Scan(&d.Categories)
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&d.Users)
	return d, nil
}

func sortStrings(ss []string) {
	sort.Slice(ss, func(i, j int) bool { return strings.ToLower(ss[i]) < strings.ToLower(ss[j]) })
}
