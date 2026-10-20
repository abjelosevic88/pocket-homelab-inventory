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
		{"POST", "/api/v1/items"}, {"PUT", "/api/v1/items/" + itoa(bos.ID)}, {"POST", "/api/v1/items/" + itoa(bos.ID) + "/archive"},
		{"DELETE", "/api/v1/items/" + itoa(bos.ID)}, {"GET", "/api/v1/activity"}, {"GET", "/api/v1/backup"}, {"GET", "/api/v1/tokens"},
	} {
		if code, _ := h.do(full, c.m, c.p, map[string]any{}); code != 403 {
			t.Fatalf("%s %s with a link token: %d, want 403", c.m, c.p, code)
		}
	}
}

// Pocket Kitchen keeps Bo's pantry in Inventory: every call its client makes
// works with items:read + items:write + items:adjust, on Bo's items only.
func TestCoreLinkKitchenPantry(t *testing.T) {
	h := newPerUserHarness(t)
	var anas, shared store.Item
	h.json(h.anaTok, "POST", "/api/v1/items", map[string]any{"name": "Ana's flour", "barcode": "111", "location": "Ana's shelf"}, &anas)
	h.json(h.anaTok, "POST", "/api/v1/items", map[string]any{"name": "Shared rice", "user_ids": []int64{h.bo.ID}}, &shared)
	k := "pcx_bo_kitchen"

	if code, body := h.do(k, "GET", "/api/v1/auth/me", nil); code != 200 || !strings.Contains(body, "bo@example.com") {
		t.Fatalf("auth/me %d %s", code, body)
	}
	var milk store.Item
	if code := h.json(k, "POST", "/api/v1/items", map[string]any{"name": "Milk", "quantity": 2, "unit": "l", "barcode": "4001", "tags": "pantry",
		"location": "Fridge", "category": "Dairy", "user_ids": []int64{h.ana.ID}}, &milk); code != 201 || milk.OwnerID != h.bo.ID || milk.LocationName != "Fridge" {
		t.Fatalf("linked create %d %+v", code, milk)
	}
	if len(milk.UserIDs) != 0 {
		t.Fatalf("a link shared the item: %+v", milk.UserIDs)
	}
	// Ana does not see Bo's milk.
	if code, _ := h.do(h.anaTok, "GET", "/api/v1/items/"+itoa(milk.ID), nil); code != 404 {
		t.Fatalf("Ana sees Bo's pantry item: %d", code)
	}
	if code, body := h.do(k, "GET", "/api/v1/items?tag=pantry&sort=name", nil); code != 200 || !strings.Contains(body, "Milk") || strings.Contains(body, "flour") {
		t.Fatalf("linked list %d %s", code, body)
	}
	if code, body := h.do(k, "GET", "/api/v1/items/"+itoa(milk.ID), nil); code != 200 || !strings.Contains(body, `"movements"`) {
		t.Fatalf("linked get %d %s", code, body)
	}
	if code, body := h.do(k, "GET", "/api/v1/lookup?barcode=4001", nil); code != 200 || !strings.Contains(body, "Milk") {
		t.Fatalf("linked lookup %d %s", code, body)
	}
	if code, _ := h.do(k, "GET", "/api/v1/lookup?barcode=111", nil); code != 404 {
		t.Fatalf("lookup found Ana's barcode: %d", code)
	}
	if code, body := h.do(k, "GET", "/api/v1/locations", nil); code != 200 || !strings.Contains(body, "Fridge") || strings.Contains(body, "Ana's shelf") {
		t.Fatalf("linked locations %d %s", code, body)
	}

	// A partial update changes only what was sent and keeps the rest.
	var upd store.Item
	if code := h.json(k, "PUT", "/api/v1/items/"+itoa(milk.ID), map[string]any{"quantity": 1}, &upd); code != 200 ||
		upd.Quantity != 1 || upd.Name != "Milk" || upd.Tags != "pantry" || upd.Barcode != "4001" || upd.LocationName != "Fridge" || upd.CategoryName != "Dairy" {
		t.Fatalf("partial update %d %+v", code, upd)
	}
	var moves []store.Movement
	h.json(h.boTok, "GET", "/api/v1/items/"+itoa(milk.ID)+"/movements", nil, &moves)
	if len(moves) == 0 || moves[0].Note != "Pocket Kitchen" {
		t.Fatalf("link quantity change not attributed: %+v", moves)
	}
	if code := h.json(k, "PUT", "/api/v1/items/"+itoa(milk.ID), map[string]any{"location": "Pantry", "tags": "pantry, dairy"}, &upd); code != 200 ||
		upd.LocationName != "Pantry" || upd.Tags != "dairy, pantry" && upd.Tags != "pantry, dairy" && upd.Tags != "pantry,dairy" && upd.Tags != "dairy,pantry" {
		t.Fatalf("move by name %d %+v", code, upd)
	}
	// Edits to a shared item go through; its sharing stays as the owner set it.
	if code := h.json(k, "PUT", "/api/v1/items/"+itoa(shared.ID), map[string]any{"notes": "basmati", "user_ids": []int64{}}, &upd); code != 200 || upd.Notes != "basmati" || len(upd.UserIDs) != 1 {
		t.Fatalf("shared edit %d %+v", code, upd)
	}
	if code := h.json(k, "POST", "/api/v1/items/"+itoa(milk.ID)+"/adjust", map[string]any{"delta": -1}, &upd); code != 200 || upd.Quantity != 0 {
		t.Fatalf("linked adjust %d %+v", code, upd)
	}
	if code := h.json(k, "POST", "/api/v1/items/"+itoa(milk.ID)+"/archive", nil, &upd); code != 200 || !upd.Archived {
		t.Fatalf("linked archive %d %+v", code, upd)
	}
	// Someone else's items stay invisible.
	for _, c := range []struct{ m, p string }{
		{"GET", "/api/v1/items/" + itoa(anas.ID)}, {"PUT", "/api/v1/items/" + itoa(anas.ID)},
		{"POST", "/api/v1/items/" + itoa(anas.ID) + "/archive"}, {"POST", "/api/v1/items/" + itoa(anas.ID) + "/adjust"},
	} {
		if code, _ := h.do(k, c.m, c.p, map[string]any{"name": "x", "delta": 1}); code != 404 {
			t.Fatalf("%s %s on Ana's item through a link: %d, want 404", c.m, c.p, code)
		}
	}
	// items:write alone does not read or adjust; nothing else opens.
	w := "pcx_bo_write"
	for _, c := range []struct{ m, p string }{
		{"GET", "/api/v1/items"}, {"GET", "/api/v1/lookup?barcode=4001"}, {"GET", "/api/v1/locations"},
		{"POST", "/api/v1/items/" + itoa(milk.ID) + "/adjust"}, {"POST", "/api/v1/items/" + itoa(milk.ID) + "/unarchive"},
		{"DELETE", "/api/v1/items/" + itoa(milk.ID)}, {"POST", "/api/v1/items/" + itoa(milk.ID) + "/photo"},
		{"POST", "/api/v1/locations"}, {"GET", "/api/v1/categories"}, {"GET", "/api/v1/activity"}, {"GET", "/api/v1/export.json"},
	} {
		if code, _ := h.do(w, c.m, c.p, map[string]any{"delta": 1, "name": "x"}); code != 403 {
			t.Fatalf("%s %s with items:write only: %d, want 403", c.m, c.p, code)
		}
	}
	if code, _ := h.do(w, "PUT", "/api/v1/items/"+itoa(milk.ID), map[string]any{"notes": "ok"}); code != 200 {
		t.Fatalf("items:write update: %d", code)
	}
}

// Pocket Day reads Bo's activity and nothing else.
func TestCoreLinkActivity(t *testing.T) {
	h := newPerUserHarness(t)
	h.do(h.anaTok, "POST", "/api/v1/items", map[string]any{"name": "Ana's secret"})
	h.do(h.boTok, "POST", "/api/v1/items", map[string]any{"name": "Bo's drill"})
	d := "pcx_bo_day"
	if code, body := h.do(d, "GET", "/api/v1/activity?limit=1000", nil); code != 200 || !strings.Contains(body, "Bo's drill") || strings.Contains(body, "Ana's secret") {
		t.Fatalf("linked activity %d %s", code, body)
	}
	for _, p := range []string{"/api/v1/items", "/api/v1/dashboard", "/api/v1/lookup?barcode=1", "/api/v1/settings"} {
		if code, _ := h.do(d, "GET", p, nil); code != 403 {
			t.Fatalf("GET %s with activity:read only: %d, want 403", p, code)
		}
	}
}

// A link acting for an admin still never reaches admin or settings routes.
func TestCoreLinkAdminRefused(t *testing.T) {
	h := newPerUserHarness(t)
	for _, c := range []struct{ m, p string }{
		{"GET", "/api/v1/users"}, {"POST", "/api/v1/users"}, {"GET", "/api/v1/settings"}, {"PUT", "/api/v1/settings"},
		{"GET", "/api/v1/settings/system"}, {"GET", "/api/v1/backup"}, {"GET", "/api/v1/tokens"}, {"POST", "/api/v1/tokens"},
		{"POST", "/api/v1/import"}, {"PUT", "/api/v1/auth/me"}, {"POST", "/api/v1/auth/logout"},
	} {
		if code, _ := h.do("pcx_ana_all", c.m, c.p, map[string]any{}); code != 403 {
			t.Fatalf("%s %s with an admin's link: %d, want 403", c.m, c.p, code)
		}
	}
	if code, _ := h.do("pcx_unknown", "GET", "/api/v1/items", nil); code != 401 {
		t.Fatalf("inactive link token: %d, want 401", code)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
