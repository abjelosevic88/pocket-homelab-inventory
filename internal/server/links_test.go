package server

import (
	"strconv"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
)

// A linked app (Pocket Garage) acting for Bo through Pocket Core sees and
// adjusts only Bo's items, and nothing else in the app.
func TestCoreLinkScopes(t *testing.T) {
	h := newPerUserHarness(t)
	var anas, bos store.Item
	h.json(h.anaTok, "POST", "/api/v1/items", map[string]any{"name": "Ana's oil", "quantity": 5}, &anas)
	h.json(h.boTok, "POST", "/api/v1/items", map[string]any{"name": "Oil filter", "quantity": 3}, &bos)
	if bos.ID == 0 || anas.ID == 0 {
		t.Fatal("items not created")
	}
	full, read := "pcx_bo_full", "pcx_bo_read"

	if code, body := h.do(full, "GET", "/api/v1/items?q=oil", nil); code != 200 || !strings.Contains(body, "Oil filter") || strings.Contains(body, "Ana's oil") {
		t.Fatalf("linked search %d %s", code, body)
	}
	var after store.Item
	if code := h.json(full, "POST", "/api/v1/items/"+itoa(bos.ID)+"/adjust", map[string]any{"delta": -1, "note": "Pocket Garage: Skoda · oil change"}, &after); code != 200 || after.Quantity != 2 {
		t.Fatalf("linked adjust %d %+v", code, after)
	}
	if code, _ := h.do(full, "POST", "/api/v1/items/"+itoa(anas.ID)+"/adjust", map[string]any{"delta": -1}); code != 404 {
		t.Fatalf("someone else's item through the link: %d", code)
	}
	if code, _ := h.do(read, "POST", "/api/v1/items/"+itoa(bos.ID)+"/adjust", map[string]any{"delta": -1}); code != 403 {
		t.Fatalf("read-only link adjusted: %d", code)
	}
	for _, c := range []struct{ m, p string }{
		{"POST", "/api/v1/items"}, {"PUT", "/api/v1/items/" + itoa(bos.ID)}, {"DELETE", "/api/v1/items/" + itoa(bos.ID)},
		{"GET", "/api/v1/locations"}, {"GET", "/api/v1/backup"}, {"GET", "/api/v1/tokens"},
	} {
		if code, _ := h.do(full, c.m, c.p, map[string]any{}); code != 403 {
			t.Fatalf("%s %s with a link token: %d, want 403", c.m, c.p, code)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
