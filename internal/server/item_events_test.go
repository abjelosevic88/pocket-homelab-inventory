package server

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
)

func TestItemCreatedAndUpdatedEventsCarryExpiryDate(t *testing.T) {
	h := newPerUserHarness(t)
	// keep a copy of every queued event (the outbox itself drains to Core)
	if _, err := h.st.DB.Exec(`CREATE TABLE seen_events(body TEXT); CREATE TRIGGER copy_event AFTER INSERT ON pocket_outbox BEGIN INSERT INTO seen_events VALUES(NEW.body); END`); err != nil {
		t.Fatal(err)
	}
	type event struct {
		Topic   string         `json:"topic"`
		Subject string         `json:"subject"`
		Payload map[string]any `json:"payload"`
	}
	last := func(topic string) event {
		t.Helper()
		var body string
		if err := h.st.DB.QueryRow(`SELECT body FROM seen_events WHERE json_extract(body,'$.topic')=? ORDER BY rowid DESC LIMIT 1`, topic).Scan(&body); err != nil {
			t.Fatal(topic, err)
		}
		var e event
		if err := json.Unmarshal([]byte(body), &e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	var item store.Item
	if h.json(h.boTok, "POST", "/api/v1/items", map[string]any{"name": "First-aid kit", "location": "Hallway", "category": "Safety", "expiry_date": "2027-03-01"}, &item) != 201 {
		t.Fatal("create")
	}
	c := last("inventory.item_created")
	if c.Subject != fmt.Sprintf("item:%d", item.ID) || c.Payload["expiry_date"] != "2027-03-01" || c.Payload["name"] != "First-aid kit" ||
		c.Payload["location"] != "Hallway" || c.Payload["category"] != "Safety" || c.Payload["user_email"] == "" || c.Payload["user_email"] == nil {
		t.Fatalf("item_created %+v", c)
	}
	if h.json(h.boTok, "PUT", fmt.Sprintf("/api/v1/items/%d", item.ID), map[string]any{"expiry_date": "2028-01-15"}, &item) != 200 {
		t.Fatal("update")
	}
	u := last("inventory.item_updated")
	if u.Subject != c.Subject || u.Payload["expiry_date"] != "2028-01-15" || u.Payload["item_id"] != float64(item.ID) ||
		u.Payload["name"] != "First-aid kit" || u.Payload["location"] != "Hallway" || u.Payload["category"] != "Safety" || u.Payload["user_email"] != c.Payload["user_email"] {
		t.Fatalf("item_updated %+v", u)
	}
	if h.json(h.boTok, "PUT", fmt.Sprintf("/api/v1/items/%d", item.ID), map[string]any{"expiry_date": ""}, &item) != 200 {
		t.Fatal("clear expiry")
	}
	if u := last("inventory.item_updated"); u.Payload["expiry_date"] != "" {
		t.Fatalf("cleared expiry %+v", u)
	}
}
