package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-inventory/internal/config"
	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
)

// perUserHarness runs the app with two accounts: Ana (the admin who owns the
// box's data) and Bo (a member). Ana calls with an API token; Bo with an API
// token or with a device token Pocket Core issued (introspected by a fake Core).
type perUserHarness struct {
	t        *testing.T
	ts       *httptest.Server
	st       *store.Store
	ana, bo  *store.User
	anaTok   string
	boTok    string
	boDevice string
}

func newPerUserHarness(t *testing.T) *perUserHarness {
	t.Helper()
	fakeCore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/introspect":
			var in struct{ Token string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.Token == "pcd_core_admin" {
				fmt.Fprint(w, `{"active":true,"kind":"device","user":{"id":1,"email":"boss@example.com","name":"Core admin","role":"admin"}}`)
				return
			}
			if strings.HasPrefix(in.Token, "pcx_bo_") { // a linked app acting for Bo
				scope := map[string]string{"pcx_bo_full": "items:read items:adjust", "pcx_bo_read": "items:read",
					"pcx_bo_kitchen": "items:read items:write items:adjust", "pcx_bo_write": "items:write", "pcx_bo_day": "activity:read"}[in.Token]
				slug, name := "garage", "Pocket Garage"
				switch in.Token {
				case "pcx_bo_kitchen", "pcx_bo_write":
					slug, name = "kitchen", "Pocket Kitchen"
				case "pcx_bo_day":
					slug, name = "day", "Pocket Day"
				}
				fmt.Fprintf(w, `{"active":true,"kind":"delegated","scope":%q,"audience":"inventory","user":{"id":7,"email":"bo@example.com","name":"Bo","role":"member"},"app":{"id":3,"slug":%q,"name":%q}}`, scope, slug, name)
				return
			}
			if in.Token == "pcx_ana_all" { // a linked app acting for Ana, the admin, with every scope
				fmt.Fprint(w, `{"active":true,"kind":"delegated","scope":"items:read items:write items:adjust activity:read","audience":"inventory","user":{"id":1,"email":"ana@example.com","name":"Ana","role":"admin"},"app":{"id":4,"slug":"kitchen","name":"Pocket Kitchen"}}`)
				return
			}
			if in.Token == "pcd_bo_phone" {
				fmt.Fprint(w, `{"active":true,"kind":"device","user":{"id":7,"email":"bo@example.com","name":"Bo","role":"member"}}`)
				return
			}
			fmt.Fprint(w, `{"active":false}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	t.Cleanup(fakeCore.Close)
	dir := t.TempDir()
	cfg := &config.Config{AuthMode: "local", SessionTTL: time.Hour, DataDir: dir, DBPath: dir + "/t.db", PhotoDir: dir + "/photos",
		MaxPhotoBytes: 5 << 20, CoreURL: fakeCore.URL, MetricsEnabled: true}
	_ = os.MkdirAll(cfg.PhotoDir, 0o755)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := New(cfg, st, log, nil)
	ctx := context.Background()
	h := &perUserHarness{t: t, st: st, boDevice: "pcd_bo_phone"}
	h.ana = &store.User{Email: "ana@example.com", Name: "Ana", Role: "admin"}
	h.bo = &store.User{Email: "bo@example.com", Name: "Bo", Role: "member"}
	for _, u := range []*store.User{h.ana, h.bo} {
		if err := s.newUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	h.anaTok, _, _ = st.CreateAPIToken(ctx, h.ana.ID, "ana")
	h.boTok, _, _ = st.CreateAPIToken(ctx, h.bo.ID, "bo")
	h.ts = httptest.NewServer(s.Router())
	t.Cleanup(h.ts.Close)
	return h
}

// do calls the API as tok and returns the status and body.
func (h *perUserHarness) do(tok, method, path string, body any) (int, string) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.ts.URL+path, rd)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func (h *perUserHarness) json(tok, method, path string, body any, out any) int {
	h.t.Helper()
	code, b := h.do(tok, method, path, body)
	if out != nil && code < 300 {
		if err := json.Unmarshal([]byte(b), out); err != nil {
			h.t.Fatalf("%s %s: %v (%s)", method, path, err, b)
		}
	}
	return code
}

func TestPerUserIsolation(t *testing.T) {
	h := newPerUserHarness(t)

	// Ana's private data: a place, a category, an item with a barcode, tag and photo.
	var loc store.Location
	if code := h.json(h.anaTok, "POST", "/api/v1/locations", map[string]any{"name": "Garage"}, &loc); code != 201 {
		t.Fatalf("create location: %d", code)
	}
	var cat store.Category
	if code := h.json(h.anaTok, "POST", "/api/v1/categories", map[string]any{"name": "Secret stash"}, &cat); code != 201 {
		t.Fatalf("create category: %d", code)
	}
	var it store.Item
	if code := h.json(h.anaTok, "POST", "/api/v1/items", map[string]any{"name": "Grandma's ring", "barcode": "4006381333931", "tags": "pantry, heirloom",
		"location_id": loc.ID, "category_id": cat.ID, "quantity": 2, "min_quantity": 5}, &it); code != 201 {
		t.Fatalf("create item: %d", code)
	}
	if it.OwnerID != h.ana.ID || it.LocationName != "Garage" {
		t.Fatalf("item owner/place wrong: %+v", it)
	}
	{
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("photo", "p.png")
		fw.Write([]byte("\x89PNG\r\n\x1a\n0000000000000000"))
		mw.Close()
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/items/%d/photo", h.ts.URL, it.ID), &buf)
		req.Header.Set("Authorization", "Bearer "+h.anaTok)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		res, err := http.DefaultClient.Do(req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("photo upload: %v %v", err, res.StatusCode)
		}
		res.Body.Close()
	}

	for _, who := range []struct{ name, tok string }{{"api token", h.boTok}, {"core device token", h.boDevice}} {
		t.Run(who.name, func(t *testing.T) {
			tok := who.tok
			id := it.ID
			// list / search / filters / lookup
			for _, path := range []string{"/api/v1/items", "/api/v1/items?archived=1", "/api/v1/items?q=ring", "/api/v1/items?tag=pantry",
				"/api/v1/items?low=1", "/api/v1/items?barcode=4006381333931", fmt.Sprintf("/api/v1/items?location=%d", loc.ID)} {
				var items []store.Item
				if code := h.json(tok, "GET", path, nil, &items); code != 200 || len(items) != 0 {
					t.Errorf("%s: Bo got %d items (code %d)", path, len(items), code)
				}
			}
			if code, _ := h.do(tok, "GET", "/api/v1/lookup?barcode=4006381333931", nil); code != 404 {
				t.Errorf("lookup: %d", code)
			}
			// read / update / delete / adjust / archive / history / photo: 404, never 403
			for _, c := range []struct {
				method, path string
				body         any
			}{
				{"GET", fmt.Sprintf("/api/v1/items/%d", id), nil},
				{"PUT", fmt.Sprintf("/api/v1/items/%d", id), map[string]any{"name": "mine now"}},
				{"POST", fmt.Sprintf("/api/v1/items/%d/adjust", id), map[string]any{"delta": -1}},
				{"POST", fmt.Sprintf("/api/v1/items/%d/adjust", id), map[string]any{"quantity": 0}},
				{"POST", fmt.Sprintf("/api/v1/items/%d/archive", id), nil},
				{"POST", fmt.Sprintf("/api/v1/items/%d/unarchive", id), nil},
				{"GET", fmt.Sprintf("/api/v1/items/%d/movements", id), nil},
				{"GET", fmt.Sprintf("/api/v1/items/%d/photo", id), nil},
				{"DELETE", fmt.Sprintf("/api/v1/items/%d/photo", id), nil},
				{"DELETE", fmt.Sprintf("/api/v1/items/%d", id), nil},
				{"PUT", fmt.Sprintf("/api/v1/locations/%d", loc.ID), map[string]any{"name": "Hijacked"}},
				{"DELETE", fmt.Sprintf("/api/v1/locations/%d", loc.ID), nil},
				{"PUT", fmt.Sprintf("/api/v1/categories/%d", cat.ID), map[string]any{"name": "Hijacked"}},
				{"DELETE", fmt.Sprintf("/api/v1/categories/%d", cat.ID), nil},
			} {
				if code, body := h.do(tok, c.method, c.path, c.body); code != 404 {
					t.Errorf("%s %s: want 404, got %d %s", c.method, c.path, code, body)
				}
			}
			// reorder silently ignores someone else's ids
			h.do(tok, "PUT", "/api/v1/locations/reorder", map[string]any{"ids": []int64{999, loc.ID}})
			// places, categories, tags, activity, dashboard, shopping
			var locs []store.Location
			h.json(tok, "GET", "/api/v1/locations", nil, &locs)
			if len(locs) != 0 {
				t.Errorf("Bo sees places: %+v", locs)
			}
			var cats []store.Category
			h.json(tok, "GET", "/api/v1/categories", nil, &cats)
			for _, c := range cats {
				if c.ID == cat.ID || c.Name == "Secret stash" {
					t.Errorf("Bo sees Ana's category: %+v", c)
				}
			}
			if len(cats) != 8 {
				t.Errorf("Bo should have his own 8 starter categories, got %d", len(cats))
			}
			_, tags := h.do(tok, "GET", "/api/v1/tags", nil)
			_, act := h.do(tok, "GET", "/api/v1/activity", nil)
			_, shop := h.do(tok, "GET", "/api/v1/shopping", nil)
			for name, body := range map[string]string{"tags": tags, "activity": act, "shopping": shop} {
				if strings.Contains(body, "heirloom") || strings.Contains(body, "Grandma") {
					t.Errorf("%s leaks Ana's item: %s", name, body)
				}
			}
			var d store.Dashboard
			h.json(tok, "GET", "/api/v1/dashboard", nil, &d)
			if d.Items != 0 || d.LowStock != 0 || d.Locations != 0 || d.Categories != 8 || d.TotalValue != 0 {
				t.Errorf("dashboard counts Ana's data: %+v", d)
			}
			// exports and the database backup
			for _, path := range []string{"/api/v1/export.json", "/api/v1/export.csv"} {
				code, body := h.do(tok, "GET", path, nil)
				if code != 200 || strings.Contains(body, "Grandma") || strings.Contains(body, "Garage") || strings.Contains(body, "Secret stash") {
					t.Errorf("%s leaks Ana's data (%d): %s", path, code, body)
				}
			}
			if code, _ := h.do(tok, "GET", "/api/v1/backup", nil); code != 403 {
				t.Errorf("member downloaded the whole database: %d", code)
			}
			// box-wide About details are admin-only
			_, sys := h.do(tok, "GET", "/api/v1/settings/system", nil)
			if strings.Contains(sys, "counts") || strings.Contains(sys, "db_path") {
				t.Errorf("member sees box-wide system info: %s", sys)
			}
		})
	}

	// nothing Bo tried changed Ana's data
	var back struct {
		Item      store.Item       `json:"item"`
		Movements []store.Movement `json:"movements"`
	}
	if code := h.json(h.anaTok, "GET", fmt.Sprintf("/api/v1/items/%d", it.ID), nil, &back); code != 200 {
		t.Fatalf("Ana lost her item: %d", code)
	}
	if back.Item.Name != "Grandma's ring" || back.Item.Quantity != 2 || back.Item.Archived || back.Item.Photo == "" || back.Item.LocationID == nil || len(back.Movements) != 1 {
		t.Fatalf("Ana's item was changed by Bo: %+v", back)
	}
	if code, _ := h.do(h.anaTok, "GET", fmt.Sprintf("/api/v1/items/%d/photo", it.ID), nil); code != 200 {
		t.Errorf("Ana's photo: %d", code)
	}
	var anaLocs []store.Location
	h.json(h.anaTok, "GET", "/api/v1/locations", nil, &anaLocs)
	if len(anaLocs) != 1 || anaLocs[0].Name != "Garage" {
		t.Errorf("Ana's places changed: %+v", anaLocs)
	}

	// Inserts take the owner from the principal, never from the body; names are unique per person.
	var boLoc store.Location
	if code := h.json(h.boTok, "POST", "/api/v1/locations", map[string]any{"name": "garage"}, &boLoc); code != 201 {
		t.Fatalf("Bo cannot have his own Garage: %d", code)
	}
	var boItem store.Item
	h.json(h.boTok, "POST", "/api/v1/items", map[string]any{"name": "Bike", "owner_id": h.ana.ID, "user_id": h.ana.ID, "location_id": loc.ID, "category_id": cat.ID}, &boItem)
	if boItem.OwnerID != h.bo.ID || boItem.LocationID != nil || boItem.CategoryID != nil {
		t.Errorf("Bo's item must be his and must not point at Ana's place/category: %+v", boItem)
	}
	var anaItems []store.Item
	h.json(h.anaTok, "GET", "/api/v1/items", nil, &anaItems)
	if len(anaItems) != 1 {
		t.Errorf("Ana sees Bo's item: %+v", anaItems)
	}
	// Admins get no special read access.
	if code, _ := h.do(h.anaTok, "GET", fmt.Sprintf("/api/v1/items/%d", boItem.ID), nil); code != 404 {
		t.Errorf("admin read a member's item: %d", code)
	}
	// Preferences are per person, the title is the admin's to change.
	h.do(h.boTok, "PUT", "/api/v1/settings", map[string]any{"title": "Bo's box", "currency": "USD", "landing": "shopping", "expiry_warn_days": 7})
	var anaSt, boSt store.Settings
	h.json(h.anaTok, "GET", "/api/v1/settings", nil, &anaSt)
	h.json(h.boTok, "GET", "/api/v1/settings", nil, &boSt)
	if anaSt.Title == "Bo's box" || boSt.Title == "Bo's box" || anaSt.Currency != "EUR" || boSt.Currency != "USD" || boSt.Landing != "shopping" {
		t.Errorf("settings: ana %+v bo %+v", anaSt, boSt)
	}
}

// TestPerUserSharing: an item shared through item_users is visible to that
// person, who may use and edit it but not delete it, re-share it or move it
// into their own places.
func TestPerUserSharing(t *testing.T) {
	h := newPerUserHarness(t)
	var loc store.Location
	h.json(h.anaTok, "POST", "/api/v1/locations", map[string]any{"name": "Attic"}, &loc)
	var tent store.Item
	h.json(h.anaTok, "POST", "/api/v1/items", map[string]any{"name": "Tent", "location_id": loc.ID, "user_ids": []int64{h.bo.ID}, "quantity": 1}, &tent)
	var private store.Item
	h.json(h.anaTok, "POST", "/api/v1/items", map[string]any{"name": "Diary"}, &private)

	var items []store.Item
	h.json(h.boDevice, "GET", "/api/v1/items", nil, &items)
	if len(items) != 1 || items[0].ID != tent.ID || items[0].LocationName != "Attic" {
		t.Fatalf("Bo should see exactly the shared tent: %+v", items)
	}
	if code := h.json(h.boTok, "POST", fmt.Sprintf("/api/v1/items/%d/adjust", tent.ID), map[string]any{"delta": 1, "note": "bought a second"}, nil); code != 200 {
		t.Errorf("Bo cannot adjust a shared item: %d", code)
	}
	var boLoc store.Location
	h.json(h.boTok, "POST", "/api/v1/locations", map[string]any{"name": "Bo's car"}, &boLoc)
	var edited store.Item
	if code := h.json(h.boTok, "PUT", fmt.Sprintf("/api/v1/items/%d", tent.ID), map[string]any{"name": "Tent (4p)", "location_id": boLoc.ID, "user_ids": []int64{}, "quantity": 2}, &edited); code != 200 {
		t.Fatalf("Bo cannot edit a shared item: %d", code)
	}
	if edited.Name != "Tent (4p)" || edited.LocationID == nil || *edited.LocationID != loc.ID || len(edited.UserIDs) != 1 {
		t.Errorf("a shared person's edit moved or re-shared the item: %+v", edited)
	}
	if code, _ := h.do(h.boTok, "DELETE", fmt.Sprintf("/api/v1/items/%d", tent.ID), nil); code != 403 {
		t.Errorf("Bo deleting Ana's shared tent: %d (want 403)", code)
	}
	if code, _ := h.do(h.boTok, "GET", fmt.Sprintf("/api/v1/items/%d", private.ID), nil); code != 404 {
		t.Errorf("unshared item: %d", code)
	}
	// Ana's history shows Bo's change; un-sharing hides the tent from Bo again.
	var back struct {
		Movements []store.Movement `json:"movements"`
	}
	h.json(h.anaTok, "GET", fmt.Sprintf("/api/v1/items/%d", tent.ID), nil, &back)
	if len(back.Movements) != 2 || back.Movements[0].UserName != "Bo" {
		t.Errorf("history: %+v", back.Movements)
	}
	h.json(h.anaTok, "PUT", fmt.Sprintf("/api/v1/items/%d", tent.ID), map[string]any{"name": "Tent (4p)", "location_id": loc.ID, "user_ids": []int64{}}, nil)
	if code, _ := h.do(h.boTok, "GET", fmt.Sprintf("/api/v1/items/%d", tent.ID), nil); code != 404 {
		t.Errorf("unshared tent still visible to Bo: %d", code)
	}
	// Exports hold only what you own, even when something is shared with you.
	h.json(h.anaTok, "PUT", fmt.Sprintf("/api/v1/items/%d", tent.ID), map[string]any{"name": "Tent (4p)", "location_id": loc.ID, "user_ids": []int64{h.bo.ID}}, nil)
	if _, body := h.do(h.boTok, "GET", "/api/v1/export.csv", nil); strings.Contains(body, "Tent") {
		t.Errorf("Bo's export contains Ana's shared tent: %s", body)
	}
	// Importing into Bo's account never touches Ana's same-named item.
	h.do(h.boTok, "POST", "/api/v1/import", map[string]any{"items": []map[string]any{{"name": "Tent (4p)", "location_name": "Attic", "quantity": 9}}})
	var anaTent struct {
		Item store.Item `json:"item"`
	}
	h.json(h.anaTok, "GET", fmt.Sprintf("/api/v1/items/%d", tent.ID), nil, &anaTent)
	if anaTent.Item.Quantity == 9 {
		t.Errorf("Bo's import overwrote Ana's item")
	}
}

// TestAutoCreatedAccountsAreMembers: an account created on first contact
// (here a Core device token whose person is an admin in Core) is a member;
// only the box's first account is ever an admin.
func TestAutoCreatedAccountsAreMembers(t *testing.T) {
	h := newPerUserHarness(t)
	if code, body := h.do("pcd_core_admin", "GET", "/api/v1/auth/me", nil); code != 200 || !strings.Contains(body, `"role":"member"`) {
		t.Fatalf("auto-created account: %d %s", code, body)
	}
	u, err := h.st.GetUserByEmail(context.Background(), "boss@example.com")
	if err != nil || u.Role != "member" {
		t.Fatalf("stored role: %+v %v", u, err)
	}
	if code, _ := h.do("pcd_core_admin", "POST", "/api/v1/users", map[string]any{"email": "x@example.com", "password": "12345678", "role": "admin"}); code != 403 {
		t.Errorf("auto-created member could create an admin: %d", code)
	}
}
