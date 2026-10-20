package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
	"io"
	"net/http"
	"sync"
	"testing"
)

func TestAdjustmentReplayAndAtomicEvents(t *testing.T) {
	h := newPerUserHarness(t)
	var item store.Item
	if h.json(h.boTok, "POST", "/api/v1/items", map[string]any{"name": "Stock", "quantity": 20}, &item) != 201 {
		t.Fatal("create")
	}
	call := func(key, body string) (int, store.Item) {
		req, _ := http.NewRequest("POST", h.ts.URL+fmt.Sprintf("/api/v1/items/%d/adjust", item.ID), bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+h.boTok)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Error(err)
			return 0, store.Item{}
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		var it store.Item
		json.Unmarshal(raw, &it)
		return res.StatusCode, it
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, it := call("one-adjustment", `{"delta":-2}`)
			if code != 200 || it.Quantity != 18 {
				t.Errorf("replay %d %+v", code, it)
			}
		}()
	}
	wg.Wait()
	var quantity float64
	var movements int
	h.st.DB.QueryRow(`SELECT quantity FROM items WHERE id=?`, item.ID).Scan(&quantity)
	h.st.DB.QueryRow(`SELECT COUNT(*) FROM movements WHERE item_id=?`, item.ID).Scan(&movements)
	if quantity != 18 || movements != 2 {
		t.Fatal(quantity, movements)
	}
	if code, _ := call("one-adjustment", `{"delta":-3}`); code != 409 {
		t.Fatal("key conflict", code)
	}
	if _, err := h.st.DB.Exec(`CREATE TRIGGER reject_event BEFORE INSERT ON pocket_outbox BEGIN SELECT RAISE(ABORT,'queue unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if code, _ := call("retry-after-failure", `{"quantity":7}`); code != 500 {
		t.Fatal("enqueue failure", code)
	}
	h.st.DB.QueryRow(`SELECT quantity FROM items WHERE id=?`, item.ID).Scan(&quantity)
	if quantity != 18 {
		t.Fatal("domain write survived failed enqueue", quantity)
	}
	if code, _ := h.do(h.boTok, "POST", fmt.Sprintf("/api/v1/items/%d/archive", item.ID), nil); code != 500 {
		t.Fatal("archive committed without event", code)
	}
	if code, _ := h.do(h.boTok, "POST", "/api/v1/items", map[string]any{"name": "Should roll back"}); code != 500 {
		t.Fatal("create committed without event", code)
	}
	var count int
	h.st.DB.QueryRow(`SELECT COUNT(*) FROM items WHERE name='Should roll back' OR archived=1`).Scan(&count)
	if count != 0 {
		t.Fatal("failed writes persisted", count)
	}
	h.st.DB.Exec(`DROP TRIGGER reject_event`)
	if code, it := call("retry-after-failure", `{"quantity":7}`); code != 200 || it.Quantity != 7 {
		t.Fatal(code, it)
	}
	// Replaying an old adjustment returns its original response, without replacing current stock.
	if code, it := call("one-adjustment", `{"delta":-2}`); code != 200 || it.Quantity != 18 {
		t.Fatal(code, it)
	}
	h.st.DB.QueryRow(`SELECT quantity FROM items WHERE id=?`, item.ID).Scan(&quantity)
	if quantity != 7 {
		t.Fatal(quantity)
	}
}

func TestCreateItemOperationReplaysOneCreation(t *testing.T) {
	h := newPerUserHarness(t)
	call := func(body string) (int, int64) {
		req, _ := http.NewRequest("POST", h.ts.URL+"/api/v1/items", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+h.boTok)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "item-create-retry")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Error(err)
			return 0, 0
		}
		defer res.Body.Close()
		var item store.Item
		json.NewDecoder(res.Body).Decode(&item)
		return res.StatusCode, item.ID
	}
	ids := make(chan int64, 12)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, id := call(`{"name":"Stock","quantity":20}`)
			if code != 201 {
				t.Errorf("create %d", code)
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	var id int64
	for got := range ids {
		if id == 0 {
			id = got
		}
		if got != id {
			t.Fatal("different replay id", got, id)
		}
	}
	if code, _ := call(`{"name":"Stock","quantity":21}`); code != 409 {
		t.Fatal("conflict", code)
	}
	var count int
	h.st.DB.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&count)
	if count != 1 {
		t.Fatal(count)
	}
	h.st.DB.QueryRow(`SELECT COUNT(*) FROM movements`).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate initial movement", count)
	}

	if code, _ := h.do(h.boTok, "DELETE", fmt.Sprintf("/api/v1/items/%d", id), nil); code != 204 {
		t.Fatal(code)
	}
	var next store.Item
	if h.json(h.boTok, "POST", "/api/v1/items", map[string]any{"name": "Replacement", "quantity": 1}, &next) != 201 {
		t.Fatal("replacement")
	}
	if next.ID <= id {
		t.Fatal("deleted item ID reused", next.ID, id)
	}
	if code, _ := call(`{"name":"Stock","quantity":20}`); code != 404 {
		t.Fatal("receipt resolved to replacement item", code)
	}
}
