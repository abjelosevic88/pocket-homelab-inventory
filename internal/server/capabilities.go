package server

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
)

// What Pocket Search, Pocket Day and Pocket Hub ask Inventory (the
// capabilities value in links.go): one route each, in the shape every Pocket
// app answers in. Each runs as the request's person and reads what the app's
// own pages read: their items and the ones shared with them. Only the capture
// adds something, one new item.

func itemRef(id int64) string { return strconv.FormatInt(id, 10) }

// itemURL opens an item in the app.
func itemURL(id int64) string { return "/items/" + itemRef(id) }

// dots joins the parts of a line that are there: "2 l · Fridge".
func dots(parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// amount is a quantity with its unit, as the pages show it: "2 l", "8 pcs", "3".
func amount(n float64, unit string) string {
	s := strconv.FormatFloat(math.Round(n*1000)/1000, 'f', -1, 64)
	if unit = strings.TrimSpace(unit); unit != "" {
		s += " " + unit
	}
	return s
}

// none reports a quantity that shows as 0.
func none(n float64) bool { return math.Abs(n) < 0.0005 }

// daysUntil counts the days from today to a YYYY-MM-DD date (negative: past).
func daysUntil(date, today string) (int, bool) {
	d, err := store.ParseDate(date)
	t, err2 := store.ParseDate(today)
	if err != nil || err2 != nil {
		return 0, false
	}
	return int(math.Round(d.Sub(t).Hours() / 24)), true
}

// expiryWords says when an item expires when that is within warn days or
// already past, "" otherwise. Like the app's own badges, the day on the
// label counts as expired.
func expiryWords(date, today string, warn int) string {
	n, ok := daysUntil(date, today)
	switch {
	case !ok || n > warn:
		return ""
	case n > 1:
		return fmt.Sprintf("expires in %d days", n)
	case n == 1:
		return "expires tomorrow"
	case n == 0:
		return "expired today"
	case n == -1:
		return "expired yesterday"
	}
	return fmt.Sprintf("expired %d days ago", -n)
}

// ---------- search ----------

// hasWord reports whether a query has a letter or a digit to look for.
func hasWord(q string) bool {
	return strings.IndexFunc(q, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

// pocketSearch answers Pocket Search with the search behind GET /items?q=:
// items by name, brand, model, serial, barcode, notes, tags, place and
// category, archived ones left out. The item called exactly that comes first,
// then names that have the words, then the rest.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	if !hasWord(q) {
		return nil, nil
	}
	ctx := r.Context()
	uid := actorID(ctx)
	items, err := s.store.ListItems(ctx, uid, store.ItemFilter{Query: q, NameFirst: true, Limit: limit})
	if err != nil {
		return nil, err
	}
	st, _ := s.store.GetSettings(ctx, uid)
	today, lq := store.Today(), strings.ToLower(q)
	has := func(v string) bool { return strings.Contains(strings.ToLower(v), lq) }
	out := make([]core.SearchItem, 0, len(items))
	for _, it := range items {
		left := amount(it.Quantity, it.Unit)
		switch {
		case none(it.Quantity):
			left = "none left"
		case it.Unit == "":
			left += " on hand"
		}
		res := core.SearchItem{ID: itemRef(it.ID), Title: it.Name, Kind: "item", Date: it.UpdatedAt, URL: itemURL(it.ID), Score: 1,
			Snippet: dots(left, it.LocationName, expiryWords(it.ExpiryDate, today, st.ExpiryWarnDays))}
		switch {
		case strings.EqualFold(it.Name, q):
			res.Score = 3
		case has(it.Name):
			res.Score = 2
		}
		// say where the words were found when the line above does not show it
		if !has(it.Name) && !has(res.Snippet) {
			for _, d := range []string{strings.TrimSpace(it.Brand + " " + it.Model), prefixed("Barcode ", it.Barcode), prefixed("Serial ", it.Serial), around(it.Notes, lq),
				prefixed("Tags: ", it.Tags), it.CategoryName} {
				if has(d) {
					res.Snippet = dots(res.Snippet, d)
					break
				}
			}
		}
		out = append(out, res)
	}
	return out, nil
}

func prefixed(label, v string) string {
	if v == "" {
		return ""
	}
	return label + v
}

// around cuts long notes down to the part that has the words.
func around(text, lq string) string {
	text = strings.Join(strings.Fields(text), " ")
	r := []rune(text)
	if len(r) <= 160 {
		return text
	}
	low := strings.ToLower(text)
	i := strings.Index(low, lq)
	if i < 0 {
		return ""
	}
	start := min(max(utf8.RuneCountInString(low[:i])-40, 0), len(r)-160)
	out := strings.TrimSpace(string(r[start : start+160]))
	if start > 0 {
		out = "…" + out
	}
	if start+160 < len(r) {
		out += "…"
	}
	return out
}

// ---------- day ----------

// dayBounds gives the UTC bounds of a local day as stored timestamps compare
// (no zone letter, so "…:00Z" and "…:00.123Z" both sort after the bound).
func dayBounds(date string, loc *time.Location) (from, to string, ok bool) {
	d, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return "", "", false
	}
	const layout = "2006-01-02T15:04:05"
	return d.UTC().Format(layout), d.AddDate(0, 0, 1).UTC().Format(layout), true
}

// changeLine words one item's day: its kind and the line shown.
//
//	a new item             added     "Olive oil: new, 1 l"
//	more than before       added     "Milk: 6 l more"
//	less than before       used      "Flour: 500 g used" (", none left" when it ran out)
//	changed, same at the end  adjusted  "Rice: 2 kg, same as before"
func changeLine(c store.DayChange) (kind, title string) {
	switch {
	case c.New && !none(c.After):
		return "added", c.Name + ": new, " + amount(c.After, c.Unit)
	case c.New:
		return "added", c.Name + ": new"
	case none(c.Net) && none(c.After):
		return "adjusted", c.Name + ": none left, same as before"
	case none(c.Net):
		return "adjusted", c.Name + ": " + amount(c.After, c.Unit) + ", same as before"
	case c.Net > 0:
		return "added", c.Name + ": " + amount(c.Net, c.Unit) + " more"
	case none(c.After):
		return "used", c.Name + ": " + amount(-c.Net, c.Unit) + " used, none left"
	}
	return "used", c.Name + ": " + amount(-c.Net, c.Unit) + " used"
}

// The notes the app writes on its own when nobody said anything.
var plainNotes = map[string]bool{"added": true, "edited": true}

// pocketDay answers Pocket Day from the activity log: one line per item whose
// quantity changed that day, its changes added up (added, used, adjusted),
// the items added that day, and the ones whose date ran out that day
// (expired). Like GET /activity it covers the items the person can see,
// shared ones with the changes others made. The day runs in the caller's
// time zone, else the box's.
func (s *Server) pocketDay(r *http.Request, date string, tz *time.Location) (*core.Day, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	loc := time.Local
	if tz != nil {
		loc = tz
	}
	from, to, ok := dayBounds(date, loc)
	if !ok {
		return nil, nil
	}
	changes, err := s.store.DayChanges(ctx, uid, from, to)
	if err != nil {
		return nil, err
	}
	day := &core.Day{Date: date}
	count := map[string]int{}
	for _, c := range changes {
		kind, title := changeLine(c)
		it := core.DayItem{ID: "item:" + itemRef(c.ItemID), Title: title, Sub: c.Location, Kind: kind, URL: itemURL(c.ItemID)}
		if at, err := time.Parse(time.RFC3339Nano, c.At); err == nil {
			it.Time = at.In(loc).Format("15:04")
		}
		var notes []string
		for _, n := range c.Notes {
			if !plainNotes[n] {
				notes = append(notes, n)
			}
		}
		it.Note = dots(notes...)
		day.Items = append(day.Items, it)
		count[kind]++
	}
	// a date that has not come yet has not run out
	if date <= time.Now().In(loc).Format("2006-01-02") {
		expired, err := s.store.ListItems(ctx, uid, store.ItemFilter{ExpiryFrom: date, ExpiryTo: date})
		if err != nil {
			return nil, err
		}
		for _, x := range expired {
			day.Items = append(day.Items, core.DayItem{ID: "expired:" + itemRef(x.ID), Title: x.Name + ": expired", Sub: x.LocationName, Kind: "expired", URL: itemURL(x.ID)})
			count["expired"]++
		}
	}
	if len(day.Items) == 0 {
		return nil, nil
	}
	var parts []string
	for _, k := range []string{"added", "used", "adjusted", "expired"} {
		if n := count[k]; n > 0 {
			parts = append(parts, strconv.Itoa(n)+" "+k)
			day.Metrics = append(day.Metrics, core.Metric{Label: strings.ToUpper(k[:1]) + k[1:], Value: strconv.Itoa(n)})
		}
	}
	day.Summary = strings.Join(parts, ", ")
	return day, nil
}

// ---------- widget ----------

// widgetRows is how many rows a widget shows (the contract's limit).
const widgetRows = 8

// pocketWidget answers Pocket Hub with what needs attention, as the
// Shopping page lists it: the items running low, the fewest left first, then
// the ones past their date or about to be (within the person's own warning
// days). The counts are the dashboard's. Someone with no items gets nothing
// to show; when all is fine there is no line and no rows.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	d, err := s.store.DashboardStats(ctx, uid, st.Currency, st.ExpiryWarnDays)
	if err != nil {
		return nil, err
	}
	if d.Items == 0 {
		return nil, nil
	}
	w := &core.WidgetData{URL: "/shopping", Metrics: []core.Metric{
		{Label: "Running low", Value: strconv.Itoa(d.LowStock)},
		{Label: "Expiring soon", Value: strconv.Itoa(d.Expiring)},
		{Label: "Expired", Value: strconv.Itoa(d.Expired)},
	}}
	var parts []string
	for _, c := range []struct {
		n    int
		what string
	}{{d.LowStock, "running low"}, {d.Expired, "expired"}, {d.Expiring, "expiring soon"}} {
		if c.n > 0 {
			parts = append(parts, strconv.Itoa(c.n)+" "+c.what)
		}
	}
	w.Line = strings.Join(parts, ", ")

	// only the few rows a widget shows are read, not the whole list
	var low, dated []store.Item
	if d.LowStock > 0 {
		if low, err = s.store.ListItems(ctx, uid, store.ItemFilter{LowOnly: true, Sort: "quantity", Limit: widgetRows}); err != nil {
			return nil, err
		}
	}
	today := store.Today()
	if d.Expired+d.Expiring > 0 {
		soon := time.Now().AddDate(0, 0, st.ExpiryWarnDays).Format("2006-01-02")
		if dated, err = s.store.ListItems(ctx, uid, store.ItemFilter{ExpiryTo: soon, Sort: "expiry", Limit: 2 * widgetRows}); err != nil {
			return nil, err
		}
	}
	isLow := map[int64]bool{}
	for _, it := range low {
		isLow[it.ID] = true
	}
	// an item that is both shows once, among the low ones
	onlyDated := dated[:0:0]
	for _, it := range dated {
		if !isLow[it.ID] {
			onlyDated = append(onlyDated, it)
		}
	}
	// both kinds get a place when there is not room for everything
	if room := widgetRows - min(len(onlyDated), widgetRows/2); len(low) > room {
		low = low[:room]
	}
	for _, it := range low {
		left := amount(it.Quantity, it.Unit) + " left"
		if none(it.Quantity) {
			left = "none left"
		}
		w.Items = append(w.Items, core.WidgetItem{ID: itemRef(it.ID), Title: it.Name, URL: itemURL(it.ID),
			Detail: dots(left, expiryWords(it.ExpiryDate, today, st.ExpiryWarnDays))})
	}
	for _, it := range onlyDated {
		if len(w.Items) == widgetRows {
			break
		}
		w.Items = append(w.Items, core.WidgetItem{ID: itemRef(it.ID), Title: it.Name, URL: itemURL(it.ID), Detail: expiryWords(it.ExpiryDate, today, st.ExpiryWarnDays)})
	}
	return w, nil
}

// ---------- capture ----------

// captureName splits a quick line into an item's name and notes: the title
// when one was given (the text is then the notes), else the text's first line.
func captureName(in core.CaptureInput) (name, notes string) {
	if in.Title != "" {
		return strings.Join(strings.Fields(in.Title), " "), in.Text
	}
	name, notes, _ = strings.Cut(in.Text, "\n")
	name, notes = strings.Join(strings.Fields(name), " "), strings.TrimSpace(notes)
	if r := []rune(name); len(r) > 200 {
		// a long paragraph is not a name: keep all of it as the notes
		return strings.TrimSpace(string(r[:199])) + "…", in.Text
	}
	return name, notes
}

// alreadyThere stops a capture that would add an item a second time.
type alreadyThere struct{ name string }

func (e *alreadyThere) Error() string { return e.name + " is already there" }

// pocketCapture keeps a quick line as a new item, the way the app adds one
// from a name alone: one of it, in the person's default unit, with no place
// or category yet, written by the code behind POST /items (the opening
// quantity in the activity log, inventory.item_created published). It never changes how many there are of
// something: a name the person already has (their own item or one shared
// with them, capitals aside) is refused. The same request_id again answers
// the item made the first time and adds nothing: the receipt is written in
// the transaction that adds the item.
func (s *Server) pocketCapture(r *http.Request, in core.CaptureInput) (*core.CaptureResult, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	op, err := core.NewOperation(fmt.Sprintf("%d:%s", uid, core.CapturePath), in.RequestID, in)
	if err != nil {
		return nil, err
	}
	name, notes := captureName(in)
	if name == "" {
		return nil, fmt.Errorf("%w: an item needs a name", core.ErrCapture)
	}
	st, _ := s.store.GetSettings(ctx, uid)
	it := &store.Item{Quantity: 1}
	// the unit the app's own form fills in for a new item
	if err := s.apply(r, it, itemInput{Name: name, Notes: notes, Unit: st.DefaultUnit}); err != nil {
		return nil, err
	}
	// checked in the transaction that adds the item, so two lines sent at once
	// cannot both add it; a retry is answered before this runs
	onlyOne := func(tx *sql.Tx) error {
		have, err := store.ItemNamed(ctx, tx, uid, it.Name, it.ID)
		if err == nil && have != "" {
			err = &alreadyThere{name: have}
		}
		return err
	}
	err = s.store.CreateItemOperation(ctx, it, uid, op, onlyOne, s.itemEvent(ctx, userFrom(ctx), "inventory.item_created", it, nil))
	var dup *alreadyThere
	switch {
	case errors.As(err, &dup):
		return nil, fmt.Errorf("%w: “%s” is already in your inventory", core.ErrCapture, dup.name)
	case errors.Is(err, core.ErrOperationConflict):
		return nil, fmt.Errorf("%w: this was already kept with other words", core.ErrCapture)
	case errors.Is(err, store.ErrNotFound): // kept before, deleted since
		return nil, fmt.Errorf("%w: this was already kept, and the item has been deleted since", core.ErrCapture)
	case err != nil:
		return nil, err
	}
	return &core.CaptureResult{ID: itemRef(it.ID), Title: it.Name, URL: itemURL(it.ID)}, nil
}
