package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
)

// The capability routes (capabilities.go) as a signed-in person and through
// link tokens pcx_bo_p<search|day|widget|capture> that Pocket Hub holds for
// Bo, one pocket:* scope each (the fake Core in peruser_test.go).

// add creates an item as tok and returns it.
func (h *perUserHarness) add(tok string, body map[string]any) store.Item {
	h.t.Helper()
	var it store.Item
	if code := h.json(tok, "POST", "/api/v1/items", body, &it); code != 201 {
		h.t.Fatalf("create %v: %d", body["name"], code)
	}
	return it
}

// change adjusts an item's quantity as tok and moves that entry of the
// activity log to a moment (nothing in the API can).
func (h *perUserHarness) change(tok string, id int64, delta float64, note, at string) {
	h.t.Helper()
	if code, body := h.do(tok, "POST", "/api/v1/items/"+itoa(id)+"/adjust", map[string]any{"delta": delta, "note": note}); code != 200 {
		h.t.Fatalf("adjust %d: %d %s", id, code, body)
	}
	if _, err := h.st.DB.Exec(`UPDATE movements SET created_at = ? WHERE id = (SELECT MAX(id) FROM movements WHERE item_id = ?)`, at, id); err != nil {
		h.t.Fatal(err)
	}
}

// born moves an item and everything in its log so far to a moment.
func (h *perUserHarness) born(id int64, at string) {
	h.t.Helper()
	if _, err := h.st.DB.Exec(`UPDATE items SET created_at = ? WHERE id = ?; UPDATE movements SET created_at = ? WHERE item_id = ?`, at, id, at, id); err != nil {
		h.t.Fatal(err)
	}
}

func (h *perUserHarness) search(tok, q string) []core.SearchItem {
	h.t.Helper()
	var res core.SearchResults
	if code := h.json(tok, "GET", "/api/v1/pocket/search?"+q, nil, &res); code != 200 || res.Items == nil {
		h.t.Fatalf("search %s: %d %+v", q, code, res)
	}
	return res.Items
}

func titles[T any](items []T, title func(T) string) string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, title(it))
	}
	return strings.Join(out, " | ")
}

func searchTitles(items []core.SearchItem) string {
	return titles(items, func(it core.SearchItem) string { return it.Title })
}

func day(n int) string { return time.Now().AddDate(0, 0, n).Format("2006-01-02") }

func TestPocketSearch(t *testing.T) {
	h := newPerUserHarness(t)
	filter := h.add(h.boTok, map[string]any{"name": "Oil filter", "quantity": 3, "unit": "pcs", "brand": "Mann", "model": "W712", "barcode": "4011558", "location": "Garage"})
	olive := h.add(h.boTok, map[string]any{"name": "Olive oil", "quantity": 0.5, "unit": "l", "location": "Pantry", "expiry_date": day(3)})
	h.add(h.boTok, map[string]any{"name": "Chain lube", "notes": "The oil for the bike chain", "category": "Bike"})
	h.add(h.boTok, map[string]any{"name": "Oil", "quantity": 0, "unit": "l", "tags": "pantry"})
	old := h.add(h.boTok, map[string]any{"name": "Old oil can"})
	if code, _ := h.do(h.boTok, "POST", "/api/v1/items/"+itoa(old.ID)+"/archive", nil); code != 200 {
		t.Fatal("archive", code)
	}
	h.add(h.boTok, map[string]any{"name": "Drill"})
	h.add(h.anaTok, map[string]any{"name": "Ana's oil"})
	h.add(h.anaTok, map[string]any{"name": "Motor oil", "user_ids": []int64{h.bo.ID}})

	// the item called exactly that, then names that have the word, then the rest
	got := h.search(h.boTok, "q=oil")
	if searchTitles(got) != "Oil | Motor oil | Oil filter | Olive oil | Chain lube" {
		t.Fatalf("oil finds: %s", searchTitles(got))
	}
	by := map[string]core.SearchItem{}
	for _, it := range got {
		by[it.Title] = it
		if it.Kind != "item" {
			t.Errorf("%q: kind %q", it.Title, it.Kind)
		}
		if _, err := time.Parse(time.RFC3339, it.Date); err != nil {
			t.Errorf("%q: date %q is not when it last changed", it.Title, it.Date)
		}
	}
	if f := by["Oil filter"]; f.ID != itoa(filter.ID) || f.URL != "/items/"+itoa(filter.ID) || f.Snippet != "3 pcs · Garage" || f.Score != 2 {
		t.Errorf("oil filter: %+v", f)
	}
	if o := by["Olive oil"]; o.ID != itoa(olive.ID) || o.Snippet != "0.5 l · Pantry · expires in 3 days" {
		t.Errorf("olive oil: %+v", o)
	}
	if o := by["Oil"]; o.Snippet != "none left" || o.Score != 3 {
		t.Errorf("oil: %+v", o)
	}
	if c := by["Chain lube"]; c.Snippet != "1 on hand · The oil for the bike chain" || c.Score != 1 {
		t.Errorf("found in the notes: %+v", c)
	}
	// what the line does not show is said: brand and model, barcode, tags, category
	for q, want := range map[string]string{"mann": "3 pcs · Garage · Mann W712", "w712": "3 pcs · Garage · Mann W712", "4011558": "3 pcs · Garage · Barcode 4011558", "garage": "3 pcs · Garage"} {
		if got := h.search(h.boTok, "q="+q); len(got) != 1 || got[0].Title != "Oil filter" || got[0].Snippet != want {
			t.Errorf("%s: %+v", q, got)
		}
	}
	if got := h.search(h.boTok, "q=pantry"); searchTitles(got) != "Oil | Olive oil" || got[0].Snippet != "none left · Tags: pantry" || got[1].Snippet != "0.5 l · Pantry · expires in 3 days" {
		t.Errorf("pantry: %+v", got)
	}
	if got := h.search(h.boTok, "q=bike"); len(got) != 1 || got[0].Snippet != "1 on hand · The oil for the bike chain" {
		t.Errorf("bike: %+v", got)
	}

	if got := h.search(h.boTok, "q=oil&limit=1"); searchTitles(got) != "Oil" {
		t.Errorf("limit=1: %s", searchTitles(got))
	}
	// nothing to look for finds nothing, even where the list would show everything
	for _, q := range []string{"q=", "q=%3F%3F", "q=%25", "q=*", "q=%20"} {
		if got := h.search(h.boTok, q); len(got) != 0 {
			t.Errorf("%s: %s", q, searchTitles(got))
		}
	}
	// strictly per person: her own, and what she shared stays hers too
	if got := h.search(h.anaTok, "q=oil"); searchTitles(got) != "Ana's oil | Motor oil" {
		t.Errorf("ana finds: %s", searchTitles(got))
	}
	if code, _ := h.do("", "GET", "/api/v1/pocket/search?q=oil", nil); code != 401 {
		t.Errorf("signed out: %d", code)
	}
}

func TestPocketSearchLongNotes(t *testing.T) {
	h := newPerUserHarness(t)
	h.add(h.boTok, map[string]any{"name": "Paint", "unit": "l", "notes": strings.Repeat("Šara ", 60) + "colour code RAL 7016 " + strings.Repeat("wall ", 60)})
	got := h.search(h.boTok, "q=ral+7016")
	if len(got) != 1 || !strings.Contains(got[0].Snippet, "colour code RAL 7016 wall") || !strings.HasPrefix(got[0].Snippet, "1 l · …") || !strings.HasSuffix(got[0].Snippet, "…") ||
		len([]rune(got[0].Snippet)) > 180 {
		t.Fatalf("long notes: %+v", got)
	}
}

func (h *perUserHarness) dayOf(tok, date, tz string) core.Day {
	h.t.Helper()
	var d core.Day
	path := "/api/v1/pocket/day/" + date
	if tz != "" {
		path += "?tz=" + tz
	}
	if code := h.json(tok, "GET", path, nil, &d); code != 200 || d.Items == nil || d.Date != date {
		h.t.Fatalf("day %s: %d %+v", date, code, d)
	}
	return d
}

func TestPocketDay(t *testing.T) {
	h := newPerUserHarness(t)
	const before, on = "2026-03-01T08:00:00.000Z", "2026-03-10"
	old := func(body map[string]any) store.Item {
		it := h.add(h.boTok, body)
		h.born(it.ID, before)
		return it
	}
	flour := old(map[string]any{"name": "Flour", "quantity": 2000, "unit": "g", "location": "Pantry"})
	milk := old(map[string]any{"name": "Milk", "quantity": 1, "unit": "l", "location": "Fridge"})
	eggs := old(map[string]any{"name": "Eggs", "quantity": 10})
	rice := old(map[string]any{"name": "Rice", "quantity": 2, "unit": "kg"})
	aspirin := old(map[string]any{"name": "Aspirin", "expiry_date": on, "location": "Bathroom"})
	old(map[string]any{"name": "Drill"})
	old(map[string]any{"name": "Vitamins", "expiry_date": "2099-01-01"})
	gone := old(map[string]any{"name": "Old plasters", "expiry_date": on})
	if code, _ := h.do(h.boTok, "POST", "/api/v1/items/"+itoa(gone.ID)+"/archive", nil); code != 200 {
		t.Fatal("archive", code)
	}

	// several changes of one thing are one line, with what they add up to
	h.change(h.boTok, flour.ID, -300, "Pocket Kitchen: Bread", "2026-03-10T09:00:00.000Z")
	h.change(h.boTok, flour.ID, -200, "Pocket Kitchen: Bread", "2026-03-10T18:30:00.000Z")
	h.change(h.boTok, flour.ID, -100, "another day", "2026-03-11T00:00:00.000Z")
	h.change(h.boTok, flour.ID, -50, "the day before", "2026-03-09T23:59:59.999Z")
	h.change(h.boTok, milk.ID, 6, "", "2026-03-10T10:00:00.000Z")
	h.change(h.boTok, eggs.ID, -4, "", "2026-03-10T07:00:00.000Z")
	h.change(h.boTok, eggs.ID, -6, "omelette", "2026-03-10T07:30:00.000Z")
	h.change(h.boTok, rice.ID, 1, "counted", "2026-03-10T11:00:00.000Z")
	h.change(h.boTok, rice.ID, -1, "counted again", "2026-03-10T11:05:00.000Z")
	// added that day: one with some on hand (and one used since), one with none yet
	oil := h.add(h.boTok, map[string]any{"name": "Olive oil", "quantity": 3, "unit": "l", "location": "Pantry"})
	h.born(oil.ID, "2026-03-10T12:00:00.000Z")
	h.change(h.boTok, oil.ID, -1, "", "2026-03-10T20:00:00.000Z")
	jar := h.add(h.boTok, map[string]any{"name": "Jar", "quantity": 0})
	h.born(jar.ID, "2026-03-10T13:00:00.000Z")
	// Ana's own things never show; what she shares with Bo does, as in his activity
	secret := h.add(h.anaTok, map[string]any{"name": "Ana's secret", "quantity": 5})
	h.born(secret.ID, before)
	h.change(h.anaTok, secret.ID, -1, "", "2026-03-10T10:00:00.000Z")
	shared := h.add(h.anaTok, map[string]any{"name": "Shared salt", "quantity": 5, "unit": "kg", "user_ids": []int64{h.bo.ID}})
	h.born(shared.ID, before)
	h.change(h.anaTok, shared.ID, -2, "", "2026-03-10T15:00:00.000Z")

	d := h.dayOf(h.boTok, on, "UTC")
	by := map[string]core.DayItem{}
	for _, it := range d.Items {
		by[it.Title] = it
	}
	if len(d.Items) != 8 || len(by) != 8 {
		t.Fatalf("day: %s", titles(d.Items, func(it core.DayItem) string { return it.Title }))
	}
	for title, want := range map[string]core.DayItem{
		"Flour: 500 g used":          {ID: "item:" + itoa(flour.ID), Time: "18:30", Sub: "Pantry", Kind: "used", URL: "/items/" + itoa(flour.ID), Note: "Pocket Kitchen: Bread"},
		"Milk: 6 l more":             {ID: "item:" + itoa(milk.ID), Time: "10:00", Sub: "Fridge", Kind: "added", URL: "/items/" + itoa(milk.ID)},
		"Eggs: 10 used, none left":   {ID: "item:" + itoa(eggs.ID), Time: "07:30", Kind: "used", URL: "/items/" + itoa(eggs.ID), Note: "omelette"},
		"Rice: 2 kg, same as before": {ID: "item:" + itoa(rice.ID), Time: "11:05", Kind: "adjusted", URL: "/items/" + itoa(rice.ID), Note: "counted · counted again"},
		"Olive oil: new, 2 l":        {ID: "item:" + itoa(oil.ID), Time: "20:00", Sub: "Pantry", Kind: "added", URL: "/items/" + itoa(oil.ID)},
		"Jar: new":                   {ID: "item:" + itoa(jar.ID), Time: "13:00", Kind: "added", URL: "/items/" + itoa(jar.ID)},
		"Shared salt: 2 kg used":     {ID: "item:" + itoa(shared.ID), Time: "15:00", Kind: "used", URL: "/items/" + itoa(shared.ID)},
		"Aspirin: expired":           {ID: "expired:" + itoa(aspirin.ID), Sub: "Bathroom", Kind: "expired", URL: "/items/" + itoa(aspirin.ID)},
	} {
		want.Title = title
		if by[title] != want {
			t.Errorf("%s:\n got %+v\nwant %+v", title, by[title], want)
		}
	}
	if d.Summary != "3 added, 3 used, 1 adjusted, 1 expired" {
		t.Errorf("summary: %q", d.Summary)
	}
	if m, _ := json.Marshal(d.Metrics); string(m) != `[{"label":"Added","value":"3"},{"label":"Used","value":"3"},{"label":"Adjusted","value":"1"},{"label":"Expired","value":"1"}]` {
		t.Errorf("metrics: %s", m)
	}

	// the neighbouring days have only their own
	if d := h.dayOf(h.boTok, "2026-03-11", "UTC"); len(d.Items) != 1 || d.Items[0].Title != "Flour: 100 g used" || d.Items[0].Note != "another day" || d.Summary != "1 used" {
		t.Errorf("the day after: %+v", d)
	}
	if d := h.dayOf(h.boTok, "2026-03-09", "UTC"); len(d.Items) != 1 || d.Items[0].Title != "Flour: 50 g used" || d.Items[0].Time != "23:59" {
		t.Errorf("the day before: %+v", d)
	}
	// a day with nothing says nothing; a date still to come has not run out
	if d := h.dayOf(h.boTok, "2026-03-05", "UTC"); len(d.Items) != 0 || d.Summary != "" || len(d.Metrics) != 0 {
		t.Errorf("an empty day: %+v", d)
	}
	if d := h.dayOf(h.boTok, "2099-01-01", "UTC"); len(d.Items) != 0 {
		t.Errorf("a day to come: %+v", d)
	}
	// the day is the caller's: 00:00 UTC on the 11th is 14:00 on the 10th in Honolulu,
	// and what happened before 10:00 UTC on the 10th was still the 9th there
	hon := h.dayOf(h.boTok, on, "Pacific/Honolulu")
	var hf core.DayItem
	for _, it := range hon.Items {
		if strings.HasPrefix(it.Title, "Flour") {
			hf = it
		}
		if strings.HasPrefix(it.Title, "Eggs") {
			t.Errorf("Honolulu's 10th has the morning in UTC: %+v", it)
		}
	}
	if hf.Title != "Flour: 300 g used" || hf.Time != "14:00" || hf.Note != "Pocket Kitchen: Bread · another day" {
		t.Errorf("Honolulu: %+v (%s)", hf, titles(hon.Items, func(it core.DayItem) string { return it.Title }))
	}

	// Ana has her own day: her secret and the salt she shared
	if d := h.dayOf(h.anaTok, on, "UTC"); len(d.Items) != 2 || d.Summary != "2 used" {
		t.Errorf("ana's day: %+v", d)
	}
	for _, bad := range []string{"today", "2026-3-10", "2026-02-30"} {
		if code, _ := h.do(h.boTok, "GET", "/api/v1/pocket/day/"+bad, nil); code != 400 {
			t.Errorf("day %s: %d", bad, code)
		}
	}
	if code, _ := h.do("", "GET", "/api/v1/pocket/day/"+on, nil); code != 401 {
		t.Errorf("signed out: %d", code)
	}
}

func (h *perUserHarness) widget(tok string) core.WidgetData {
	h.t.Helper()
	var w core.WidgetData
	if code := h.json(tok, "GET", "/api/v1/pocket/widget", nil, &w); code != 200 || w.Items == nil {
		h.t.Fatalf("widget: %d %+v", code, w)
	}
	return w
}

func widgetLines(w core.WidgetData) string {
	return titles(w.Items, func(it core.WidgetItem) string { return it.Title + " (" + it.Detail + ")" })
}

func TestPocketWidget(t *testing.T) {
	h := newPerUserHarness(t)
	// nothing kept yet: nothing to show
	if w := h.widget(h.boTok); len(w.Items) != 0 || w.Line != "" || len(w.Metrics) != 0 || w.URL != "" {
		t.Fatalf("no items: %+v", w)
	}
	// all is fine: the numbers, no line, no rows
	h.add(h.boTok, map[string]any{"name": "Drill"})
	h.add(h.boTok, map[string]any{"name": "Wine", "quantity": 6, "expiry_date": day(200)})
	fine := h.widget(h.boTok)
	if m, _ := json.Marshal(fine.Metrics); len(fine.Items) != 0 || fine.Line != "" || fine.URL != "/shopping" ||
		string(m) != `[{"label":"Running low","value":"0"},{"label":"Expiring soon","value":"0"},{"label":"Expired","value":"0"}]` {
		t.Fatalf("all fine: %+v", fine)
	}

	batteries := h.add(h.boTok, map[string]any{"name": "Batteries", "quantity": 2, "min_quantity": 4, "unit": "pcs"})
	h.add(h.boTok, map[string]any{"name": "Salt", "quantity": 0, "min_quantity": 1})
	h.add(h.boTok, map[string]any{"name": "Butter", "quantity": 1, "min_quantity": 2, "expiry_date": day(1)})
	h.add(h.boTok, map[string]any{"name": "Milk", "quantity": 3, "unit": "l", "expiry_date": day(3)})
	h.add(h.boTok, map[string]any{"name": "Yoghurt", "expiry_date": day(-1)})
	h.add(h.boTok, map[string]any{"name": "Cream", "expiry_date": day(0)})
	h.add(h.boTok, map[string]any{"name": "Ham", "expiry_date": day(-5)})
	archived := h.add(h.boTok, map[string]any{"name": "Old glue", "quantity": 0, "min_quantity": 1, "expiry_date": day(-9)})
	if code, _ := h.do(h.boTok, "POST", "/api/v1/items/"+itoa(archived.ID)+"/archive", nil); code != 200 {
		t.Fatal("archive", code)
	}
	h.add(h.anaTok, map[string]any{"name": "Ana's tea", "quantity": 0, "min_quantity": 1})
	h.add(h.anaTok, map[string]any{"name": "Shared soap", "quantity": 1, "min_quantity": 3, "user_ids": []int64{h.bo.ID}})

	w := h.widget(h.boTok)
	// running low first, the fewest left on top; then by date, the longest past first
	if got := widgetLines(w); got != "Salt (none left) | Butter (1 left · expires tomorrow) | Shared soap (1 left) | Batteries (2 pcs left) | "+
		"Ham (expired 5 days ago) | Yoghurt (expired yesterday) | Cream (expired today) | Milk (expires in 3 days)" {
		t.Errorf("rows: %s", got)
	}
	if w.Line != "4 running low, 3 expired, 2 expiring soon" || w.URL != "/shopping" {
		t.Errorf("line: %q, url %q", w.Line, w.URL)
	}
	if m, _ := json.Marshal(w.Metrics); string(m) != `[{"label":"Running low","value":"4"},{"label":"Expiring soon","value":"2"},{"label":"Expired","value":"3"}]` {
		t.Errorf("metrics: %s", m)
	}
	for _, it := range w.Items {
		if it.Title == "Batteries" && (it.ID != itoa(batteries.ID) || it.URL != "/items/"+itoa(batteries.ID)) {
			t.Errorf("row: %+v", it)
		}
	}
	// Ana sees her own and what she shared, never Bo's
	if a := h.widget(h.anaTok); widgetLines(a) != "Ana's tea (none left) | Shared soap (1 left)" || a.Line != "2 running low" {
		t.Errorf("ana: %s / %q", widgetLines(a), a.Line)
	}

	// more than fits: both kinds keep a place
	for i := range 6 {
		h.add(h.boTok, map[string]any{"name": fmt.Sprintf("Spare %d", i), "quantity": 0, "min_quantity": 1})
	}
	w = h.widget(h.boTok)
	if got := widgetLines(w); len(w.Items) != 8 || !strings.HasPrefix(got, "Salt (none left) | Spare 0 (none left) | Spare 1 (none left) | Spare 2 (none left) | Ham (expired 5 days ago) | Yoghurt") ||
		w.Line != "10 running low, 3 expired, 2 expiring soon" {
		t.Errorf("full: %s / %q", got, w.Line)
	}

	// no warning days: only what is past its date, not everything with a date
	st, _ := h.st.GetSettings(context.Background(), h.bo.ID)
	st.ExpiryWarnDays = 0
	if err := h.st.SaveUserSettings(context.Background(), h.bo.ID, st); err != nil {
		t.Fatal(err)
	}
	w = h.widget(h.boTok)
	if got := widgetLines(w); strings.Contains(got, "Milk") || strings.Contains(got, "Wine") || strings.Contains(got, "tomorrow") || !strings.Contains(got, "Cream (expired today)") ||
		w.Line != "10 running low, 3 expired" {
		t.Errorf("no warning days: %s / %q", got, w.Line)
	}
	if code, _ := h.do("", "GET", "/api/v1/pocket/widget", nil); code != 401 {
		t.Errorf("signed out: %d", code)
	}
}

// keep sends a quick line and returns the status and the answer.
func (h *perUserHarness) keep(tok string, body map[string]any) (int, map[string]any) {
	h.t.Helper()
	code, raw := h.do(tok, "POST", "/api/v1/pocket/capture", body)
	var m map[string]any
	_ = json.Unmarshal([]byte(raw), &m)
	return code, m
}

func (h *perUserHarness) count(q string, args ...any) int {
	h.t.Helper()
	var n int
	if err := h.st.DB.QueryRow(q, args...).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

func TestPocketCapture(t *testing.T) {
	h := newPerUserHarness(t)
	if _, err := h.st.DB.Exec(`CREATE TABLE seen_events(body TEXT); CREATE TRIGGER copy_event AFTER INSERT ON pocket_outbox BEGIN INSERT INTO seen_events VALUES(NEW.body); END`); err != nil {
		t.Fatal(err)
	}
	line := map[string]any{"request_id": "req-0000000000000001", "kind": "item", "text": "  Olive   oil "}
	code, res := h.keep(h.boTok, line)
	if code != 201 || res["title"] != "Olive oil" || res["id"] == "" || res["url"] != "/items/"+res["id"].(string) {
		t.Fatalf("capture: %d %v", code, res)
	}
	// as the app adds one from a name alone: one of it, the person's unit, no place, his own, in the log
	var got struct {
		Item      store.Item       `json:"item"`
		Movements []store.Movement `json:"movements"`
	}
	if h.json(h.boTok, "GET", "/api/v1/items/"+res["id"].(string), nil, &got) != 200 {
		t.Fatal("the kept item is not there")
	}
	it := got.Item
	if it.Name != "Olive oil" || it.Quantity != 1 || it.Unit != "pcs" || it.OwnerID != h.bo.ID || it.LocationID != nil || it.CategoryID != nil || len(it.UserIDs) != 0 || it.Notes != "" {
		t.Fatalf("item: %+v", it)
	}
	if len(got.Movements) != 1 || got.Movements[0].Delta != 1 || got.Movements[0].Note != "added" || got.Movements[0].UserID == nil || *got.Movements[0].UserID != h.bo.ID {
		t.Fatalf("activity: %+v", got.Movements)
	}
	if n := h.count(`SELECT COUNT(1) FROM seen_events WHERE json_extract(body,'$.topic')='inventory.item_created' AND json_extract(body,'$.payload.name')='Olive oil'
		AND json_extract(body,'$.payload.user_email')='bo@example.com'`); n != 1 {
		t.Fatalf("item_created events: %d", n)
	}
	if code, _ := h.do(h.anaTok, "GET", "/api/v1/items/"+res["id"].(string), nil); code != 404 {
		t.Fatalf("ana sees what bo kept: %d", code)
	}

	// the same request again answers the same and adds nothing, also after a rename
	if code, _ := h.do(h.boTok, "PUT", "/api/v1/items/"+res["id"].(string), map[string]any{"name": "Olive oil, extra virgin"}); code != 200 {
		t.Fatal("rename", code)
	}
	if code, again := h.keep(h.boTok, line); code != 201 || again["id"] != res["id"] || again["title"] != "Olive oil" || again["url"] != res["url"] {
		t.Fatalf("retry: %d %v", code, again)
	}
	if n := h.count(`SELECT COUNT(1) FROM items`); n != 1 {
		t.Fatalf("items after a retry: %d", n)
	}
	if n := h.count(`SELECT COUNT(1) FROM seen_events WHERE json_extract(body,'$.topic')='inventory.item_created'`); n != 1 {
		t.Fatalf("events after a retry: %d", n)
	}
	// the same id with other words is refused
	if code, res := h.keep(h.boTok, map[string]any{"request_id": "req-0000000000000001", "text": "Vinegar"}); code != 400 || res["error"] != "this was already kept with other words" {
		t.Fatalf("same id, other words: %d %v", code, res)
	}
	// the id is the person's: Ana's line with it is her own item
	if code, res := h.keep(h.anaTok, line); code != 201 || res["title"] != "Olive oil" {
		t.Fatalf("ana, same id: %d %v", code, res)
	}

	// a name that is already there is not added again, capitals aside
	h.add(h.boTok, map[string]any{"name": "Šećer u prahu", "quantity": 3})
	h.add(h.anaTok, map[string]any{"name": "Shared rice", "user_ids": []int64{h.bo.ID}})
	before := h.count(`SELECT COUNT(1) FROM items`)
	for text, want := range map[string]string{
		"olive OIL, extra virgin": "“Olive oil, extra virgin” is already in your inventory",
		"šećer U PRAHU":           "“Šećer u prahu” is already in your inventory",
		"SHARED RICE":             "“Shared rice” is already in your inventory",
	} {
		id := fmt.Sprintf("req-dup-%012d", len(text))
		if code, res := h.keep(h.boTok, map[string]any{"request_id": id, "text": text}); code != 400 || res["error"] != want {
			t.Errorf("%s: %d %v", text, code, res)
		}
		// asked again, the answer is the same: nothing was kept under that id
		if code, _ := h.keep(h.boTok, map[string]any{"request_id": id, "text": text}); code != 400 {
			t.Errorf("%s again: %d", text, code)
		}
	}
	if n := h.count(`SELECT COUNT(1) FROM items`); n != before {
		t.Fatalf("a refused line added something: %d → %d", before, n)
	}
	if n := h.count(`SELECT quantity FROM items WHERE name = 'Šećer u prahu'`); n != 3 {
		t.Fatalf("capture changed how many there are: %d", n)
	}
	// a part of a name is another thing; so is the name of something archived or someone else's
	if code, res := h.keep(h.boTok, map[string]any{"request_id": "req-0000000000000002", "text": "Rice"}); code != 201 || res["title"] != "Rice" {
		t.Fatalf("rice: %d %v", code, res)
	}
	old := h.add(h.boTok, map[string]any{"name": "Candles"})
	if code, _ := h.do(h.boTok, "POST", "/api/v1/items/"+itoa(old.ID)+"/archive", nil); code != 200 {
		t.Fatal("archive", code)
	}
	if code, res := h.keep(h.boTok, map[string]any{"request_id": "req-0000000000000003", "text": "candles"}); code != 201 {
		t.Fatalf("the name of an archived item: %d %v", code, res)
	}
	h.add(h.anaTok, map[string]any{"name": "Ana's secret"})
	if code, res := h.keep(h.boTok, map[string]any{"request_id": "req-0000000000000004", "text": "ana's secret"}); code != 201 {
		t.Fatalf("the name of someone else's item: %d %v", code, res)
	}

	// a title names it and the text is the notes; without one the first line names it
	code, res = h.keep(h.boTok, map[string]any{"request_id": "req-0000000000000005", "title": "Tea", "text": "green, loose\nfrom the market"})
	if h.json(h.boTok, "GET", "/api/v1/items/"+fmt.Sprint(res["id"]), nil, &got); code != 201 || got.Item.Name != "Tea" || got.Item.Notes != "green, loose\nfrom the market" {
		t.Fatalf("title and text: %d %+v", code, got.Item)
	}
	code, res = h.keep(h.boTok, map[string]any{"request_id": "req-0000000000000006", "text": "Coffee\nbeans, 1 kg bag"})
	if h.json(h.boTok, "GET", "/api/v1/items/"+fmt.Sprint(res["id"]), nil, &got); code != 201 || got.Item.Name != "Coffee" || got.Item.Notes != "beans, 1 kg bag" {
		t.Fatalf("two lines: %d %+v", code, got.Item)
	}
	// the person's own default unit
	st, _ := h.st.GetSettings(context.Background(), h.bo.ID)
	st.DefaultUnit = "kom"
	if err := h.st.SaveUserSettings(context.Background(), h.bo.ID, st); err != nil {
		t.Fatal(err)
	}
	code, res = h.keep(h.boTok, map[string]any{"request_id": "req-0000000000000007", "text": "Sponge"})
	if h.json(h.boTok, "GET", "/api/v1/items/"+fmt.Sprint(res["id"]), nil, &got); code != 201 || got.Item.Unit != "kom" || got.Item.Quantity != 1 {
		t.Fatalf("default unit: %d %+v", code, got.Item)
	}

	// what the person can fix is said
	for _, c := range []struct {
		body map[string]any
		want string
	}{
		{map[string]any{"request_id": "req-0000000000000008", "text": " "}, "write something first"},
		{map[string]any{"request_id": "short", "text": "Soap"}, "request_id must be 16–80 letters, digits, _ or -"},
		{map[string]any{"request_id": "req-0000000000000008", "kind": "place", "text": "Shed"}, "this app does not keep a place"},
	} {
		if code, res := h.keep(h.boTok, c.body); code != 400 || res["error"] != c.want {
			t.Errorf("%v: %d %v", c.body, code, res)
		}
	}
	if code, _ := h.keep("", map[string]any{"request_id": "req-0000000000000009", "text": "Soap"}); code != 401 {
		t.Errorf("signed out: %d", code)
	}
}

// Lines sent at the same moment: the same request is kept once, and so is the
// same name under different requests.
func TestPocketCaptureAtOnce(t *testing.T) {
	h := newPerUserHarness(t)
	var wg sync.WaitGroup
	ids := make([]string, 10)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, raw := h.do(h.boTok, "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "req-same-000000000001", "text": "Vinegar"})
			var res core.CaptureResult
			_ = json.Unmarshal([]byte(raw), &res)
			if code != 201 || res.ID == "" {
				t.Errorf("same request: %d %s", code, raw)
			}
			ids[i] = res.ID
		}()
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("one request, several answers: %v", ids)
		}
	}
	codes := make([]int, 10)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i], _ = h.do(h.boTok, "POST", "/api/v1/pocket/capture", map[string]any{"request_id": fmt.Sprintf("req-other-%010d", i), "text": "Soda"})
		}()
	}
	wg.Wait()
	kept := 0
	for _, c := range codes {
		switch c {
		case 201:
			kept++
		case 400:
		default:
			t.Errorf("same name at once: %d", c)
		}
	}
	if n := h.count(`SELECT COUNT(1) FROM items WHERE name IN ('Vinegar', 'Soda')`); kept != 1 || n != 2 {
		t.Fatalf("kept %d of the same name, %d items", kept, n)
	}
	if n := h.count(`SELECT COUNT(1) FROM movements`); n != 2 {
		t.Fatalf("log entries: %d", n)
	}
}

// A link token with one pocket:* scope reaches its one route and nothing else.
func TestPocketLinkTokens(t *testing.T) {
	h := newPerUserHarness(t)
	bos := h.add(h.boTok, map[string]any{"name": "Oil filter", "quantity": 1, "min_quantity": 2})
	h.add(h.anaTok, map[string]any{"name": "Ana's oil", "quantity": 0, "min_quantity": 2})
	today := store.Today()
	line := func(requestID string) map[string]any {
		return map[string]any{"request_id": requestID, "kind": "item", "text": "Brake fluid"}
	}
	item := "/api/v1/items/" + itoa(bos.ID)
	for _, c := range []struct {
		tok, method, path string
		body              any
		want              int
	}{
		{"pcx_bo_psearch", "GET", "/api/v1/pocket/search?q=oil", nil, 200},
		{"pcx_bo_pday", "GET", "/api/v1/pocket/day/" + today, nil, 200},
		{"pcx_bo_pwidget", "GET", "/api/v1/pocket/widget", nil, 200},
		{"pcx_bo_pcapture", "POST", "/api/v1/pocket/capture", line("req-0000000000000001"), 201},
		{"pcx_bo_pwidget", "GET", "/api/v1/auth/me", nil, 200},
		// another capability's route
		{"pcx_bo_psearch", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		{"pcx_bo_psearch", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_bo_psearch", "POST", "/api/v1/pocket/capture", line("req-0000000000000002"), 403},
		{"pcx_bo_pday", "GET", "/api/v1/pocket/search?q=oil", nil, 403},
		{"pcx_bo_pday", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_bo_pday", "POST", "/api/v1/pocket/capture", line("req-0000000000000002"), 403},
		{"pcx_bo_pwidget", "GET", "/api/v1/pocket/search?q=oil", nil, 403},
		{"pcx_bo_pwidget", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		{"pcx_bo_pwidget", "POST", "/api/v1/pocket/capture", line("req-0000000000000002"), 403},
		{"pcx_bo_pcapture", "GET", "/api/v1/pocket/search?q=oil", nil, 403},
		{"pcx_bo_pcapture", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		{"pcx_bo_pcapture", "GET", "/api/v1/pocket/widget", nil, 403},
		// the wrong method is no route at all
		{"pcx_bo_psearch", "POST", "/api/v1/pocket/search?q=oil", nil, 405},
		{"pcx_bo_pcapture", "GET", "/api/v1/pocket/capture", nil, 405},
		// the rest of the app
		{"pcx_bo_psearch", "GET", "/api/v1/items?q=oil", nil, 403},
		{"pcx_bo_psearch", "GET", item, nil, 403},
		{"pcx_bo_psearch", "GET", "/api/v1/lookup?barcode=1", nil, 403},
		{"pcx_bo_pday", "GET", "/api/v1/activity", nil, 403},
		{"pcx_bo_pday", "GET", item + "/movements", nil, 403},
		{"pcx_bo_pwidget", "GET", "/api/v1/dashboard", nil, 403},
		{"pcx_bo_pwidget", "GET", "/api/v1/shopping", nil, 403},
		{"pcx_bo_pwidget", "GET", "/api/v1/items?low=1", nil, 403},
		{"pcx_bo_pcapture", "POST", "/api/v1/items", map[string]any{"name": "x"}, 403},
		{"pcx_bo_pcapture", "PUT", item, map[string]any{"name": "x"}, 403},
		{"pcx_bo_pcapture", "POST", item + "/adjust", map[string]any{"delta": 1}, 403},
		{"pcx_bo_pcapture", "POST", item + "/archive", nil, 403},
		{"pcx_bo_pcapture", "DELETE", item, nil, 403},
		{"pcx_bo_pcapture", "GET", "/api/v1/settings", nil, 403},
		{"pcx_bo_pcapture", "GET", "/api/v1/export.json", nil, 403},
		// the app's own scopes do not open the new routes
		{"pcx_bo_kitchen", "GET", "/api/v1/pocket/search?q=oil", nil, 403},
		{"pcx_bo_kitchen", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_bo_kitchen", "POST", "/api/v1/pocket/capture", line("req-0000000000000003"), 403},
		{"pcx_bo_read", "GET", "/api/v1/pocket/search?q=oil", nil, 403},
		{"pcx_bo_write", "POST", "/api/v1/pocket/capture", line("req-0000000000000003"), 403},
		{"pcx_bo_day", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		{"pcx_ana_all", "GET", "/api/v1/pocket/search?q=oil", nil, 403},
		{"pcx_ana_all", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		{"pcx_ana_all", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_ana_all", "POST", "/api/v1/pocket/capture", line("req-0000000000000003"), 403},
		// and they still open what they did
		{"pcx_bo_read", "GET", "/api/v1/items?q=oil", nil, 200},
		{"pcx_bo_day", "GET", "/api/v1/activity", nil, 200},
		{"pcx_unknown", "GET", "/api/v1/pocket/widget", nil, 401},
	} {
		if code, body := h.do(c.tok, c.method, c.path, c.body); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, body)
		}
	}
	// the token acts for its person only
	if got := h.search("pcx_bo_psearch", "q=oil"); searchTitles(got) != "Oil filter" {
		t.Errorf("bo through the link finds: %s", searchTitles(got))
	}
	if w := h.widget("pcx_bo_pwidget"); widgetLines(w) != "Oil filter (1 left)" || w.Line != "1 running low" {
		t.Errorf("bo's widget through the link: %s / %q", widgetLines(w), w.Line)
	}
	d := h.dayOf("pcx_bo_pday", today, "")
	if got := titles(d.Items, func(it core.DayItem) string { return it.Title }); got != "Oil filter: new, 1 | Brake fluid: new, 1 pcs" || d.Summary != "2 added" {
		t.Errorf("bo's day through the link: %s / %q", got, d.Summary)
	}
	// only the one allowed capture landed: Bo's own, shared with nobody; a retry adds nothing
	if code, res := h.keep("pcx_bo_pcapture", line("req-0000000000000001")); code != 201 || res["title"] != "Brake fluid" {
		t.Fatalf("retry through the link: %d %v", code, res)
	}
	var kept []store.Item
	h.json(h.boTok, "GET", "/api/v1/items?q=brake", nil, &kept)
	if len(kept) != 1 || kept[0].OwnerID != h.bo.ID || len(kept[0].UserIDs) != 0 || h.count(`SELECT COUNT(1) FROM items`) != 3 {
		t.Fatalf("kept through the link: %+v", kept)
	}
	// a request id is the person's whichever way it came: the same line with his own login is the same item
	if code, res := h.keep(h.boTok, line("req-0000000000000001")); code != 201 || res["id"] != itoa(kept[0].ID) {
		t.Fatalf("the same request with his own login: %d %v", code, res)
	}
}

func TestPocketLinkScopes(t *testing.T) {
	req := func(m, p string) *http.Request { return httptest.NewRequest(m, p, nil) }
	for _, c := range []struct {
		scope string
		r     *http.Request
		want  bool
	}{
		{core.ScopeSearch, req("GET", "/api/v1/pocket/search?q=x"), true},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/2026-01-02"), true},
		{core.ScopeWidget, req("GET", "/api/v1/pocket/widget"), true},
		{core.ScopeCapture, req("POST", "/api/v1/pocket/capture"), true},
		{core.ScopeWidget, req("GET", "/api/v1/auth/me"), true},
		{core.ScopeSearch, req("POST", "/api/v1/pocket/search"), false},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/today"), false},
		{core.ScopeDay, req("GET", "/api/v1/activity"), false},
		{core.ScopeWidget, req("GET", "/api/v1/dashboard"), false},
		{core.ScopeCapture, req("GET", "/api/v1/pocket/capture"), false},
		{core.ScopeCapture, req("POST", "/api/v1/items"), false},
		{"items:read", req("GET", "/api/v1/pocket/search?q=x"), false},
		{"items:read", req("GET", "/api/v1/pocket/widget"), false},
		{"activity:read", req("GET", "/api/v1/pocket/day/2026-01-02"), false},
		{"items:write", req("POST", "/api/v1/pocket/capture"), false},
		{"items:adjust", req("POST", "/api/v1/pocket/capture"), false},
	} {
		if got := linkAllows([]string{c.scope}, c.r); got != c.want {
			t.Errorf("%s %s %s = %v, want %v", c.scope, c.r.Method, c.r.URL.Path, got, c.want)
		}
	}
}

// Inventory tells Core what it can do, so Search, Day and Hub find it on their own.
func TestManifestCapabilities(t *testing.T) {
	h := newPerUserHarness(t)
	h.srv.cfg.CoreRegistrationKey = "key"
	if err := h.srv.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if raw := h.manifest.Load(); raw == nil || json.Unmarshal(*raw, &m) != nil {
		t.Fatal("no manifest reached Core")
	}
	if m["slug"] != "inventory" || m["category"] != "Home" {
		t.Fatalf("manifest: %v %v", m["slug"], m["category"])
	}
	search, _ := m["search"].(map[string]any)
	day, _ := m["day"].(map[string]any)
	widget, _ := m["widget"].(map[string]any)
	cap, _ := m["capture"].(map[string]any)
	if search == nil || fmt.Sprint(search["bangs"]) != "[inv inventory]" || fmt.Sprint(search["kinds"]) != "[item]" || day == nil || day["label"] != "Inventory" ||
		day["one"] != "change" || day["many"] != "changes" || widget == nil || widget["title"] != "Running low" || cap == nil {
		t.Fatalf("capabilities: %v %v %v %v", search, day, widget, cap)
	}
	if k := cap["kinds"].([]any)[0].(map[string]any); len(cap["kinds"].([]any)) != 1 || k["kind"] != "item" || fmt.Sprint(k["fields"]) != "[title]" {
		t.Fatalf("capture kinds: %v", cap)
	}
	offered := map[string]bool{}
	for _, o := range m["offers"].([]any) {
		offered[o.(map[string]any)["scope"].(string)] = true
	}
	for _, sc := range []string{"items:read", "items:adjust", "items:write", "activity:read", "move:export", "move:import",
		core.ScopeSearch, core.ScopeDay, core.ScopeWidget, core.ScopeCapture} {
		if !offered[sc] {
			t.Errorf("%s is not offered", sc)
		}
	}
}
