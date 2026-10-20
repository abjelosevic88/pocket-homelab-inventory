package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
)

func filterFrom(r *http.Request) store.ItemFilter {
	q := r.URL.Query()
	f := store.ItemFilter{
		Query:    q.Get("q"),
		Tag:      q.Get("tag"),
		Barcode:  q.Get("barcode"),
		LowOnly:  queryBool(r, "low"),
		Archived: queryBool(r, "archived"),
		Sort:     q.Get("sort"),
		Limit:    int(queryInt(r, "limit")),
		Expiring: int(queryInt(r, "expiring")),
	}
	switch q.Get("location") {
	case "", "any":
	case "none":
		f.LocationID = -1
	default:
		f.LocationID = queryInt(r, "location")
	}
	switch q.Get("category") {
	case "", "any":
	case "none":
		f.CategoryID = -1
	default:
		f.CategoryID = queryInt(r, "category")
	}
	switch q.Get("user") {
	case "", "any":
	case "none", "household", "unshared":
		f.UserID = -1
	case "me":
		f.UserID = actorID(r.Context())
	default:
		f.UserID = queryInt(r, "user")
	}
	return f
}

func (s *Server) handleListItems(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListItems(r.Context(), actorID(r.Context()), filterFrom(r))
	if err != nil {
		s.fail(w, err, "list items")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// handleLookup finds an item by barcode (for scanner / Shortcuts flows).
func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("barcode"))
	if code == "" {
		writeErr(w, http.StatusBadRequest, "barcode is required")
		return
	}
	items, err := s.store.ListItems(r.Context(), actorID(r.Context()), store.ItemFilter{Barcode: code, Archived: true, Limit: 1})
	if err != nil {
		s.fail(w, err, "lookup")
		return
	}
	if len(items) == 0 {
		writeErr(w, http.StatusNotFound, "no item with that barcode")
		return
	}
	writeJSON(w, http.StatusOK, items[0])
}

func (s *Server) handleGetItem(w http.ResponseWriter, r *http.Request) {
	uid := actorID(r.Context())
	it, err := s.store.GetItem(r.Context(), uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get item")
		return
	}
	moves, _ := s.store.ListMovements(r.Context(), uid, it.ID, 30)
	writeJSON(w, http.StatusOK, map[string]any{"item": it, "movements": moves})
}

type itemInput struct {
	Name         string   `json:"name"`
	Notes        string   `json:"notes"`
	CategoryID   *int64   `json:"category_id"`
	LocationID   *int64   `json:"location_id"`
	Location     string   `json:"location"` // name — created when it does not exist
	Category     string   `json:"category"` // name — created when it does not exist
	Quantity     *float64 `json:"quantity"`
	Unit         string   `json:"unit"`
	MinQuantity  *float64 `json:"min_quantity"`
	Barcode      string   `json:"barcode"`
	Brand        string   `json:"brand"`
	Model        string   `json:"model"`
	Serial       string   `json:"serial"`
	PurchaseDate string   `json:"purchase_date"`
	Price        *float64 `json:"price"`
	ExpiryDate   string   `json:"expiry_date"`
	Tags         string   `json:"tags"`
	URL          string   `json:"url"`
	UserIDs      *[]int64 `json:"user_ids"`
	Note         string   `json:"note"` // movement note when the quantity changes
}

func (in itemInput) validate() string {
	if strings.TrimSpace(in.Name) == "" {
		return "name is required"
	}
	if in.Quantity != nil && *in.Quantity < 0 {
		return "quantity cannot be negative"
	}
	if in.MinQuantity != nil && *in.MinQuantity < 0 {
		return "min_quantity cannot be negative"
	}
	if in.Price != nil && *in.Price < 0 {
		return "price cannot be negative"
	}
	if in.PurchaseDate != "" && !validDate(in.PurchaseDate) {
		return "purchase_date must be YYYY-MM-DD"
	}
	if in.ExpiryDate != "" && !validDate(in.ExpiryDate) {
		return "expiry_date must be YYYY-MM-DD"
	}
	return ""
}

// apply copies the input onto an item, resolving location/category names in
// the item owner's own lists (owner = the item's creator; actor for new items).
// Names that do not exist yet are created for the owner only when the owner is
// the one saving; the store keeps place/category as they were otherwise.
func (s *Server) apply(r *http.Request, it *store.Item, in itemInput) error {
	ctx := r.Context()
	owner := it.OwnerID
	if owner == 0 {
		owner = actorID(ctx)
	}
	mayCreate := owner == actorID(ctx)
	it.Name = strings.TrimSpace(in.Name)
	it.Notes = strings.TrimSpace(in.Notes)
	it.Unit = strings.TrimSpace(in.Unit)
	it.Barcode = strings.TrimSpace(in.Barcode)
	it.Brand = strings.TrimSpace(in.Brand)
	it.Model = strings.TrimSpace(in.Model)
	it.Serial = strings.TrimSpace(in.Serial)
	it.PurchaseDate = in.PurchaseDate
	it.ExpiryDate = in.ExpiryDate
	it.Tags = in.Tags
	it.URL = strings.TrimSpace(in.URL)
	if in.Quantity != nil {
		it.Quantity = *in.Quantity
	}
	if in.MinQuantity != nil {
		it.MinQuantity = *in.MinQuantity
	}
	if in.Price != nil {
		it.Price = *in.Price
	}
	if in.UserIDs != nil {
		it.UserIDs = *in.UserIDs
	}
	it.LocationID = in.LocationID
	if in.LocationID != nil && *in.LocationID <= 0 {
		it.LocationID = nil
	}
	if name := strings.TrimSpace(in.Location); name != "" && it.LocationID == nil && mayCreate {
		l, err := s.store.FindLocationByName(ctx, owner, name)
		if err != nil {
			l = &store.Location{Name: name}
			if err := s.store.CreateLocation(ctx, owner, l); err != nil {
				return err
			}
		}
		it.LocationID = &l.ID
	}
	it.CategoryID = in.CategoryID
	if in.CategoryID != nil && *in.CategoryID <= 0 {
		it.CategoryID = nil
	}
	if name := strings.TrimSpace(in.Category); name != "" && it.CategoryID == nil && mayCreate {
		c, err := s.store.FindCategoryByName(ctx, owner, name)
		if err != nil {
			c = &store.Category{Name: name}
			if err := s.store.CreateCategory(ctx, owner, c); err != nil {
				return err
			}
		}
		it.CategoryID = &c.ID
	}
	return nil
}

func (s *Server) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	var in itemInput
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.validate(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if linkApp(r.Context()) != "" {
		in.UserIDs = nil // sharing stays with the person, never a linked app
	}
	op, err := core.NewOperation(fmt.Sprintf("%d:%s:items:create", actorID(r.Context()), linkApp(r.Context())), r.Header.Get("Idempotency-Key"), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var previous store.Item
	if ok, err := op.Replay(r.Context(), s.store.DB, &previous); err != nil {
		s.fail(w, err, "read operation")
		return
	} else if ok {
		if _, err := s.store.GetItem(r.Context(), actorID(r.Context()), previous.ID); err != nil {
			s.fail(w, err, "item")
			return
		}
		writeJSON(w, http.StatusCreated, previous)
		return
	}

	st, _ := s.store.GetSettings(r.Context(), actorID(r.Context()))
	it := &store.Item{Quantity: 1, Unit: st.DefaultUnit}
	if err := s.apply(r, it, in); err != nil {
		s.fail(w, err, "resolve places")
		return
	}
	if err := s.store.CreateItemOperation(r.Context(), it, actorID(r.Context()), op, s.itemEvent(r.Context(), userFrom(r.Context()), "inventory.item_created", it, nil)); err != nil {
		s.fail(w, err, "create item")
		return
	}
	writeJSON(w, http.StatusCreated, it)
}

// handleUpdateItem saves an item. Fields missing from the body keep their
// current values, so a client may send only what changed ({"tags": "…"}); the
// web app always sends the whole form. A location/category given by name
// without an id moves the item there (created for the owner when missing).
func (s *Server) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	it, err := s.store.GetItem(r.Context(), actorID(r.Context()), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get item")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	var sent map[string]json.RawMessage
	in := inputFrom(it)
	if json.Unmarshal(raw, &sent) != nil || json.Unmarshal(raw, &in) != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if _, ok := sent["location_id"]; !ok && strings.TrimSpace(in.Location) != "" {
		in.LocationID = nil
	}
	if _, ok := sent["category_id"]; !ok && strings.TrimSpace(in.Category) != "" {
		in.CategoryID = nil
	}
	app := linkApp(r.Context())
	if app != "" {
		// Who an item is shared with stays with the person, never a linked app.
		ids := append([]int64{}, it.UserIDs...)
		in.UserIDs = &ids
	}
	if msg := in.validate(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	note := strings.TrimSpace(in.Note)
	if note == "" && app != "" {
		note = app
	}
	if err := s.apply(r, it, in); err != nil {
		s.fail(w, err, "resolve places")
		return
	}
	if err := s.store.UpdateItem(r.Context(), it, note, actorID(r.Context())); err != nil {
		s.fail(w, err, "update item")
		return
	}
	writeJSON(w, http.StatusOK, it)
}

// inputFrom is an item as the body of a full update.
func inputFrom(it *store.Item) itemInput {
	q, mq, p := it.Quantity, it.MinQuantity, it.Price
	ids := append([]int64{}, it.UserIDs...)
	return itemInput{
		Name: it.Name, Notes: it.Notes, CategoryID: it.CategoryID, LocationID: it.LocationID,
		Quantity: &q, Unit: it.Unit, MinQuantity: &mq, Barcode: it.Barcode, Brand: it.Brand,
		Model: it.Model, Serial: it.Serial, PurchaseDate: it.PurchaseDate, Price: &p,
		ExpiryDate: it.ExpiryDate, Tags: it.Tags, URL: it.URL, UserIDs: &ids,
	}
}

func (s *Server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	id, uid := idParam(r, "id"), actorID(r.Context())
	it, err := s.store.GetItem(r.Context(), uid, id)
	if err != nil {
		s.fail(w, err, "get item")
		return
	}
	if err := s.store.DeleteItem(r.Context(), uid, id); err != nil {
		s.fail(w, err, "delete item")
		return
	}
	if it.Photo != "" {
		_ = os.Remove(filepath.Join(s.cfg.PhotoDir, it.Photo))
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAdjustItem changes the quantity: {"delta": -1} or {"quantity": 12}, with an optional "note".
func (s *Server) handleAdjustItem(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Delta    *float64 `json:"delta"`
		Quantity *float64 `json:"quantity"`
		Note     string   `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	actor := actorID(r.Context())
	if in.Note = strings.TrimSpace(in.Note); in.Note == "" {
		in.Note = linkApp(r.Context())
	}
	if in.Quantity == nil && in.Delta == nil {
		writeErr(w, http.StatusBadRequest, "delta or quantity is required")
		return
	}
	op, err := core.NewOperation(fmt.Sprintf("%d:%s:items:%d:adjust", actor, linkApp(r.Context()), idParam(r, "id")), r.Header.Get("Idempotency-Key"), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	it, err := s.store.AdjustQuantityOperation(r.Context(), idParam(r, "id"), in.Delta, in.Quantity, in.Note, actor, op, func(tx *sql.Tx, it *store.Item) error {
		hook := s.itemEvent(r.Context(), userFrom(r.Context()), "inventory.adjusted", it, map[string]any{"delta": in.Delta, "set_to": in.Quantity, "note": in.Note, "low": it.MinQuantity > 0 && it.Quantity <= it.MinQuantity})
		if hook != nil {
			return hook(tx)
		}
		return nil
	})
	if err != nil {
		s.fail(w, err, "adjust quantity")
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) handleArchiveItem(w http.ResponseWriter, r *http.Request) {
	uid := actorID(r.Context())
	it, err := s.store.GetItem(r.Context(), uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "archive item")
		return
	}
	if err = s.store.SetArchived(r.Context(), uid, it.ID, true, s.itemEvent(r.Context(), userFrom(r.Context()), "inventory.item_archived", it, nil)); err != nil {
		s.fail(w, err, "archive item")
		return
	}
	it, _ = s.store.GetItem(r.Context(), uid, it.ID)
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) handleUnarchiveItem(w http.ResponseWriter, r *http.Request) {
	uid := actorID(r.Context())
	if err := s.store.SetArchived(r.Context(), uid, idParam(r, "id"), false); err != nil {
		s.fail(w, err, "unarchive item")
		return
	}
	it, _ := s.store.GetItem(r.Context(), uid, idParam(r, "id"))
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) handleItemMovements(w http.ResponseWriter, r *http.Request) {
	uid := actorID(r.Context())
	if _, err := s.store.GetItem(r.Context(), uid, idParam(r, "id")); err != nil {
		s.fail(w, err, "movements")
		return
	}
	m, err := s.store.ListMovements(r.Context(), uid, idParam(r, "id"), int(queryInt(r, "limit")))
	if err != nil {
		s.fail(w, err, "movements")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.RecentActivity(r.Context(), actorID(r.Context()), int(queryInt(r, "limit")), false)
	if err != nil {
		s.fail(w, err, "activity")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// ---- photos ----

var photoExt = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif", "image/heic": ".heic"}

func (s *Server) handleUploadPhoto(w http.ResponseWriter, r *http.Request) {
	uid := actorID(r.Context())
	it, err := s.store.GetItem(r.Context(), uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get item")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxPhotoBytes)
	if err := r.ParseMultipartForm(s.cfg.MaxPhotoBytes); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("photo too large or malformed upload (max %d MB)", s.cfg.MaxPhotoBytes>>20))
		return
	}
	f, hdr, err := r.FormFile("photo")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "form field 'photo' is required")
		return
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	ct := http.DetectContentType(head[:n])
	if ct == "application/octet-stream" {
		ct = hdr.Header.Get("Content-Type")
	}
	ct, _, _ = mime.ParseMediaType(ct)
	ext, ok := photoExt[ct]
	if !ok {
		writeErr(w, http.StatusBadRequest, "unsupported image type "+ct)
		return
	}
	name := fmt.Sprintf("%d-%s%s", it.ID, store.RandomToken(4), ext)
	dst, err := os.Create(filepath.Join(s.cfg.PhotoDir, name))
	if err != nil {
		s.fail(w, err, "save photo")
		return
	}
	if _, err := dst.Write(head[:n]); err == nil {
		_, err = io.Copy(dst, f)
	}
	dst.Close()
	if err != nil {
		_ = os.Remove(filepath.Join(s.cfg.PhotoDir, name))
		s.fail(w, err, "save photo")
		return
	}
	if it.Photo != "" {
		_ = os.Remove(filepath.Join(s.cfg.PhotoDir, it.Photo))
	}
	if err := s.store.SetPhoto(r.Context(), uid, it.ID, name); err != nil {
		s.fail(w, err, "save photo")
		return
	}
	it, _ = s.store.GetItem(r.Context(), uid, it.ID)
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) handleGetPhoto(w http.ResponseWriter, r *http.Request) {
	it, err := s.store.GetItem(r.Context(), actorID(r.Context()), idParam(r, "id"))
	if err != nil || it.Photo == "" {
		writeErr(w, http.StatusNotFound, "no photo")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, filepath.Join(s.cfg.PhotoDir, filepath.Base(it.Photo)))
}

func (s *Server) handleDeletePhoto(w http.ResponseWriter, r *http.Request) {
	uid := actorID(r.Context())
	it, err := s.store.GetItem(r.Context(), uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get item")
		return
	}
	if it.Photo != "" {
		_ = os.Remove(filepath.Join(s.cfg.PhotoDir, filepath.Base(it.Photo)))
		_ = s.store.SetPhoto(r.Context(), uid, it.ID, "")
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- dashboard / shopping ----

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	uid := actorID(r.Context())
	st, _ := s.store.GetSettings(r.Context(), uid)
	d, err := s.store.DashboardStats(r.Context(), uid, st.Currency, st.ExpiryWarnDays)
	if err != nil {
		s.fail(w, err, "dashboard")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// ShoppingGroup is the low-stock list for one location.
type ShoppingGroup struct {
	Location string       `json:"location"`
	Items    []store.Item `json:"items"`
}

// handleShopping returns low-stock items grouped by location, plus items past
// or near their expiry date.
func (s *Server) handleShopping(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	low, err := s.store.ListItems(ctx, uid, store.ItemFilter{LowOnly: true, Sort: "location"})
	if err != nil {
		s.fail(w, err, "shopping")
		return
	}
	groups := map[string][]store.Item{}
	for _, it := range low {
		groups[it.LocationName] = append(groups[it.LocationName], it)
	}
	names := make([]string, 0, len(groups))
	for k := range groups {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == "" {
			return false
		}
		if names[j] == "" {
			return true
		}
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	out := make([]ShoppingGroup, 0, len(names))
	for _, n := range names {
		out = append(out, ShoppingGroup{Location: n, Items: groups[n]})
	}
	expiring, _ := s.store.ListItems(ctx, uid, store.ItemFilter{Expiring: st.ExpiryWarnDays, Sort: "expiry"})
	if expiring == nil {
		expiring = []store.Item{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out, "low_count": len(low), "expiring": expiring, "today": store.Today()})
}

func fmtQty(q float64) string { return strconv.FormatFloat(q, 'f', -1, 64) }
