package server

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handlePutSettings saves the caller's own preferences; the app title is
// box-wide and only an admin's change to it is kept.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	uid := actorID(r.Context())
	cur, err := s.store.GetSettings(r.Context(), uid)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	var in store.Settings
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = "Pocket Inventory"
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		in.Currency = "EUR"
	}
	in.DefaultUnit = strings.TrimSpace(in.DefaultUnit)
	if in.Landing != "shopping" {
		in.Landing = "items"
	}
	if in.ExpiryWarnDays < 0 || in.ExpiryWarnDays > 3650 {
		in.ExpiryWarnDays = 30
	}
	in.SetupComplete, in.DefaultsSeeded = cur.SetupComplete, cur.DefaultsSeeded
	if !isAdmin(r.Context()) {
		in.Title = cur.Title
	} else if in.Title != cur.Title {
		if err := s.store.SaveGlobalSettings(r.Context(), in); err != nil {
			s.fail(w, err, "save settings")
			return
		}
	}
	if err := s.store.SaveUserSettings(r.Context(), uid, in); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// handleSystemInfo: everyone sees the version and runtime; paths, box-wide
// row counts and the Core link are for admins.
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r.Context()) {
		writeJSON(w, http.StatusOK, map[string]any{
			"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
			"auth_mode": s.cfg.AuthMode, "uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		})
		return
	}
	var size int64
	if fi, err := os.Stat(s.cfg.DBPath); err == nil {
		size = fi.Size()
	}
	var photoBytes int64
	var photos int
	_ = filepath.Walk(s.cfg.PhotoDir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			photoBytes += info.Size()
			photos++
		}
		return nil
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"core":      s.coreStatus(),
		"auth_mode": s.cfg.AuthMode, "data_dir": s.cfg.DataDir, "db_path": s.cfg.DBPath, "db_bytes": size,
		"photo_dir": s.cfg.PhotoDir, "photos": photos, "photo_bytes": photoBytes,
		"uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		"counts": s.store.Counts(r.Context()),
	})
}

// coreStatus summarises the Pocket Core link for the About page.
func (s *Server) coreStatus() map[string]any {
	s.coreMu.Lock()
	defer s.coreMu.Unlock()
	return map[string]any{"configured": s.core.Configured(), "url": s.cfg.CoreURL, "registered": s.coreOK, "error": s.coreErr}
}

// ---------- backup / export / import ----------

// handleBackup streams the whole database (every account), so it is admin-only;
// everyone can export their own data with export.json / export.csv.
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	dir := filepath.Join(s.cfg.DataDir, "backups")
	name := fmt.Sprintf("pocket-inventory-%s.db", time.Now().Format("20060102-150405"))
	dst := filepath.Join(dir, name)
	if err := s.store.Backup(r.Context(), dst); err != nil {
		s.fail(w, err, "backup")
		return
	}
	defer os.Remove(dst)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	http.ServeFile(w, r, dst)
}

// Export is the portable JSON shape (also accepted by /import).
type Export struct {
	Version    string              `json:"version"`
	ExportedAt string              `json:"exported_at"`
	Settings   store.Settings      `json:"settings"`
	Locations  []store.Location    `json:"locations"`
	Categories []store.Category    `json:"categories"`
	Users      []store.UserRef     `json:"users"`
	Items      []store.Item        `json:"items"`
	Movements  []store.ActivityRow `json:"movements"`
}

// buildExport is the caller's own data: their settings, places, categories,
// the items they own (with who each is shared with) and those items' history.
func (s *Server) buildExport(ctx context.Context) (*Export, error) {
	uid := actorID(ctx)
	st, err := s.store.GetSettings(ctx, uid)
	if err != nil {
		return nil, err
	}
	locs, err := s.store.ListLocations(ctx, uid)
	if err != nil {
		return nil, err
	}
	cats, err := s.store.ListCategories(ctx, uid)
	if err != nil {
		return nil, err
	}
	items, err := s.store.ListItems(ctx, uid, store.ItemFilter{Archived: true, OwnedOnly: true})
	if err != nil {
		return nil, err
	}
	moves, err := s.store.RecentActivity(ctx, uid, 100000, true)
	if err != nil {
		return nil, err
	}
	ex := &Export{Version: Version, ExportedAt: store.Now(), Settings: st, Locations: locs, Categories: cats, Items: items, Movements: moves, Users: []store.UserRef{}}
	seen := map[int64]bool{}
	for _, it := range items {
		for _, u := range it.Users {
			if !seen[u.ID] {
				seen[u.ID] = true
				ex.Users = append(ex.Users, u)
			}
		}
	}
	return ex, nil
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	ex, err := s.buildExport(r.Context())
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-inventory-export.json"`)
	writeJSON(w, http.StatusOK, ex)
}

var csvHeader = []string{"name", "quantity", "unit", "min_quantity", "location", "category", "owners", "tags", "brand", "model", "serial", "barcode", "price", "purchase_date", "expiry_date", "url", "notes", "archived"}

// handleExportCSV writes one row per item the caller owns — the same columns import.csv accepts.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListItems(r.Context(), actorID(r.Context()), store.ItemFilter{Archived: true, OwnedOnly: true})
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-inventory-export.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write(csvHeader)
	for _, it := range items {
		owners := make([]string, 0, len(it.Users))
		for _, u := range it.Users {
			owners = append(owners, u.Name)
		}
		arch := ""
		if it.Archived {
			arch = "yes"
		}
		_ = cw.Write([]string{it.Name, fmtQty(it.Quantity), it.Unit, fmtQty(it.MinQuantity), it.LocationName, it.CategoryName, strings.Join(owners, "; "), it.Tags,
			it.Brand, it.Model, it.Serial, it.Barcode, fmtQty(it.Price), it.PurchaseDate, it.ExpiryDate, it.URL, it.Notes, arch})
	}
	cw.Flush()
}

// handleImportJSON merges an export file into the caller's own data: places are
// matched by name in their lists (created when missing), items are matched by
// name + location among the items they own and updated or created.
func (s *Server) handleImportJSON(w http.ResponseWriter, r *http.Request) {
	var ex Export
	if err := decode(r, &ex); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	rows := make([]importRow, 0, len(ex.Items))
	userNames := map[int64]string{}
	for _, u := range ex.Users {
		userNames[u.ID] = u.Name
	}
	for _, it := range ex.Items {
		owners := make([]string, 0, len(it.Users))
		for _, u := range it.Users {
			owners = append(owners, u.Name)
		}
		for _, id := range it.UserIDs {
			if n, ok := userNames[id]; ok && !contains(owners, n) {
				owners = append(owners, n)
			}
		}
		rows = append(rows, importRow{item: it, location: it.LocationName, category: it.CategoryName, owners: owners})
	}
	created, updated, err := s.importRows(r, rows)
	if err != nil {
		s.fail(w, err, "import")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"created": created, "updated": updated})
}

type importRow struct {
	item     store.Item
	location string
	category string
	owners   []string
}

// handleImportCSV accepts the export.csv columns (header row required, order free).
func (s *Server) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	var src io.Reader = r.Body
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, "malformed upload")
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "form field 'file' is required")
			return
		}
		defer f.Close()
		src = f
	}
	cr := csv.NewReader(src)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "empty or invalid csv")
		return
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))] = i
	}
	if _, ok := col["name"]; !ok {
		writeErr(w, http.StatusBadRequest, "csv needs a 'name' column")
		return
	}
	get := func(rec []string, k string) string {
		if i, ok := col[k]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	num := func(rec []string, k string, def float64) float64 {
		v := strings.ReplaceAll(get(rec, k), ",", ".")
		if v == "" {
			return def
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		return def
	}
	var rows []importRow
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, "csv parse error: "+err.Error())
			return
		}
		name := get(rec, "name")
		if name == "" {
			continue
		}
		arch := strings.ToLower(get(rec, "archived"))
		it := store.Item{
			Name: name, Quantity: num(rec, "quantity", 1), Unit: get(rec, "unit"), MinQuantity: num(rec, "min_quantity", 0),
			Tags: get(rec, "tags"), Brand: get(rec, "brand"), Model: get(rec, "model"), Serial: get(rec, "serial"), Barcode: get(rec, "barcode"),
			Price: num(rec, "price", 0), PurchaseDate: get(rec, "purchase_date"), ExpiryDate: get(rec, "expiry_date"), URL: get(rec, "url"), Notes: get(rec, "notes"),
			Archived: arch == "yes" || arch == "true" || arch == "1",
		}
		if !validDate(it.PurchaseDate) {
			it.PurchaseDate = ""
		}
		if !validDate(it.ExpiryDate) {
			it.ExpiryDate = ""
		}
		var owners []string
		for _, o := range strings.FieldsFunc(get(rec, "owners"), func(r rune) bool { return r == ';' || r == '|' }) {
			if o = strings.TrimSpace(o); o != "" {
				owners = append(owners, o)
			}
		}
		rows = append(rows, importRow{item: it, location: get(rec, "location"), category: get(rec, "category"), owners: owners})
	}
	created, updated, err := s.importRows(r, rows)
	if err != nil {
		s.fail(w, err, "import")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"created": created, "updated": updated})
}

func (s *Server) importRows(r *http.Request, rows []importRow) (created, updated int, err error) {
	ctx := r.Context()
	actor := actorID(ctx)
	existing, err := s.store.ListItems(ctx, actor, store.ItemFilter{Archived: true, OwnedOnly: true})
	if err != nil {
		return 0, 0, err
	}
	users, _ := s.store.ListUsers(ctx)
	userByName := map[string]int64{}
	for _, u := range users {
		userByName[strings.ToLower(u.Name)] = u.ID
		userByName[strings.ToLower(u.Email)] = u.ID
	}
	key := func(name, loc string) string {
		return strings.ToLower(strings.TrimSpace(name)) + "\x00" + strings.ToLower(strings.TrimSpace(loc))
	}
	byKey := map[string]store.Item{}
	for _, e := range existing {
		byKey[key(e.Name, e.LocationName)] = e
	}
	for _, row := range rows {
		it := row.item
		it.ID, it.Photo, it.OwnerID = 0, "", 0
		it.LocationID, it.CategoryID = nil, nil
		if row.location != "" {
			l, err := s.store.FindLocationByName(ctx, actor, row.location)
			if err != nil {
				l = &store.Location{Name: row.location}
				if err := s.store.CreateLocation(ctx, actor, l); err != nil {
					return created, updated, err
				}
			}
			it.LocationID = &l.ID
		}
		if row.category != "" {
			c, err := s.store.FindCategoryByName(ctx, actor, row.category)
			if err != nil {
				c = &store.Category{Name: row.category}
				if err := s.store.CreateCategory(ctx, actor, c); err != nil {
					return created, updated, err
				}
			}
			it.CategoryID = &c.ID
		}
		it.UserIDs = nil
		for _, o := range row.owners {
			if id, ok := userByName[strings.ToLower(o)]; ok {
				it.UserIDs = append(it.UserIDs, id)
			}
		}
		if cur, ok := byKey[key(it.Name, row.location)]; ok {
			it.ID, it.Photo, it.CreatedAt, it.OwnerID = cur.ID, cur.Photo, cur.CreatedAt, cur.OwnerID
			if err := s.store.UpdateItem(ctx, &it, "import", actor); err != nil {
				return created, updated, err
			}
			updated++
			continue
		}
		if err := s.store.CreateItem(ctx, &it, actor); err != nil {
			return created, updated, err
		}
		byKey[key(it.Name, row.location)] = it
		created++
	}
	return created, updated, nil
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
