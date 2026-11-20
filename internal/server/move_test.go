package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-inventory/internal/config"
	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
)

// moveHarness is Ana (admin) and Bo (member) behind a fake Core whose
// pcx_<ana|bo>_<export|import|read> tokens are Pocket Move links.
func moveHarness(t *testing.T) *perUserHarness {
	t.Helper()
	fakeCore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/introspect" {
			fmt.Fprint(w, `{}`)
			return
		}
		var in struct{ Token string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := strings.SplitN(strings.TrimPrefix(in.Token, "pcx_"), "_", 2)
		if len(parts) != 2 {
			fmt.Fprint(w, `{"active":false}`)
			return
		}
		scope := map[string]string{"export": core.ScopeMoveExport, "import": core.ScopeMoveImport, "read": "items:read"}[parts[1]]
		email := parts[0] + "@example.com"
		fmt.Fprintf(w, `{"active":true,"kind":"delegated","scope":%q,"audience":"inventory","user":{"id":7,"sub":%q,"email":%q,"name":%q,"role":"member"},"app":{"id":3,"slug":"move","name":"Pocket Move"}}`, scope, email, email, parts[0])
	}))
	t.Cleanup(fakeCore.Close)
	dir := t.TempDir()
	cfg := &config.Config{AuthMode: "local", SessionTTL: time.Hour, DataDir: dir, DBPath: dir + "/t.db", PhotoDir: dir + "/photos",
		MaxPhotoBytes: 5 << 20, CoreURL: fakeCore.URL}
	_ = os.MkdirAll(cfg.PhotoDir, 0o755)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := New(cfg, st, log, nil)
	if err := s.core.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	h := &perUserHarness{t: t, st: st}
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

func TestMoveLinkScopes(t *testing.T) {
	h := moveHarness(t)
	exp, imp, read := "pcx_bo_export", "pcx_bo_import", "pcx_bo_read"
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{exp, "GET", core.PortablePath, 200},
		{exp, "GET", "/api/v1/export.json", 200},
		{exp, "GET", "/api/v1/export.csv", 200},
		{exp, "POST", "/api/v1/import", 403},
		{exp, "GET", "/api/v1/items", 403},
		{exp, "GET", "/api/v1/settings", 403},
		{exp, "GET", "/api/v1/backup", 403},
		{imp, "GET", core.PortablePath, 200},
		{imp, "GET", "/api/v1/export.json", 403},
		{imp, "POST", "/api/v1/import.csv", 403},
		{imp, "POST", "/api/v1/items", 403},
		{imp, "PUT", "/api/v1/settings", 403},
		{read, "GET", core.PortablePath, 403},
		{read, "GET", "/api/v1/export.json", 403},
	} {
		if code, body := h.do(c.tok, c.method, c.path, map[string]any{}); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, body)
		}
	}
	_, body := h.do(exp, "GET", core.PortablePath, nil)
	var p core.Portable
	if err := json.Unmarshal([]byte(body), &p); err != nil || len(p.Exports) != 2 || len(p.Imports) != 1 || p.Exports[0].Kind != core.KindPocket {
		t.Fatalf("portable: %s", body)
	}
}

// Move brings Ana's items to Bo: the import adds once, never overwrites
// Bo's own item, and keeps the two apart.
func TestMoveImportsAddOnce(t *testing.T) {
	h := moveHarness(t)
	h.json(h.anaTok, "POST", "/api/v1/items", map[string]any{"name": "Ana's drill", "quantity": 1, "location": "Garage"}, nil)
	h.json(h.anaTok, "POST", "/api/v1/items", map[string]any{"name": "Tent", "quantity": 1, "location": "Attic"}, nil)
	h.json(h.boTok, "POST", "/api/v1/items", map[string]any{"name": "Tent", "quantity": 3, "location": "Attic"}, nil)

	code, export := h.do("pcx_ana_export", "GET", "/api/v1/export.json", nil)
	if code != 200 || !strings.Contains(export, "Ana's drill") {
		t.Fatalf("ana's export: %d %s", code, export)
	}
	if _, bo := h.do("pcx_bo_export", "GET", "/api/v1/export.json", nil); strings.Contains(bo, "Ana's drill") {
		t.Fatalf("bo's export holds ana's items: %s", bo)
	}
	var doc map[string]any
	_ = json.Unmarshal([]byte(export), &doc)
	imp := func() map[string]float64 {
		code, out := h.do("pcx_bo_import", "POST", "/api/v1/import", doc)
		if code != 200 {
			t.Fatalf("import → %d %s", code, out)
		}
		var m map[string]float64
		_ = json.Unmarshal([]byte(out), &m)
		return m
	}
	if first := imp(); first["created"] != 1 || first["skipped"] != 1 || first["updated"] != 0 {
		t.Fatalf("first import: %v", first)
	}
	if second := imp(); second["created"] != 0 || second["updated"] != 0 {
		t.Fatalf("second import added: %v", second)
	}
	var items []store.Item
	h.json(h.boTok, "GET", "/api/v1/items?archived=1", nil, &items)
	if len(items) != 2 {
		t.Fatalf("bo's items: %+v", items)
	}
	for _, it := range items {
		if it.Name == "Tent" && it.Quantity != 3 {
			t.Fatalf("bo's own tent was overwritten: %+v", it)
		}
	}
	var anas []store.Item
	h.json(h.anaTok, "GET", "/api/v1/items", nil, &anas)
	if len(anas) != 2 {
		t.Fatalf("ana's items: %+v", anas)
	}
}
