package store

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"), slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mkUser(t *testing.T, s *Store, email, name, role string) *User {
	t.Helper()
	u := &User{Email: email, Name: name, Role: role}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestItemsLowStockAndMovements(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	me := mkUser(t, s, "me@x", "Me", "admin").ID
	loc := &Location{Name: "Garage"}
	if err := s.CreateLocation(ctx, me, loc); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLocation(ctx, me, &Location{Name: "garage"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict on duplicate name, got %v", err)
	}
	cat := &Category{Name: "Consumables"}
	if err := s.CreateCategory(ctx, me, cat); err != nil {
		t.Fatal(err)
	}
	it := &Item{Name: "AA batteries", Quantity: 8, Unit: "pcs", MinQuantity: 4, LocationID: &loc.ID, CategoryID: &cat.ID, Tags: "power, power ,Spare"}
	if err := s.CreateItem(ctx, it, me); err != nil {
		t.Fatal(err)
	}
	if it.ID == 0 || it.LocationName != "Garage" || it.CategoryName != "Consumables" {
		t.Fatalf("joined names missing: %+v", it)
	}
	if it.Tags != "power, Spare" {
		t.Fatalf("tags not normalised: %q", it.Tags)
	}
	if it.Low {
		t.Fatal("8 of min 4 should not be low")
	}
	got, err := s.AdjustQuantity(ctx, it.ID, -5, "used", me)
	if err != nil {
		t.Fatal(err)
	}
	if got.Quantity != 3 || !got.Low {
		t.Fatalf("expected 3 and low, got %v low=%v", got.Quantity, got.Low)
	}
	got, err = s.AdjustQuantity(ctx, it.ID, -10, "oops", me)
	if err != nil {
		t.Fatal(err)
	}
	if got.Quantity != 0 {
		t.Fatalf("quantity must clamp at zero, got %v", got.Quantity)
	}
	moves, err := s.ListMovements(ctx, me, it.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(moves) != 3 || moves[0].Delta != -3 || moves[2].Delta != 8 {
		t.Fatalf("unexpected movements: %+v", moves)
	}
	low, err := s.ListItems(ctx, me, ItemFilter{LowOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(low) != 1 {
		t.Fatalf("expected one low item, got %d", len(low))
	}
	byTag, _ := s.ListItems(ctx, me, ItemFilter{Tag: "spare"})
	if len(byTag) != 1 {
		t.Fatalf("tag filter failed: %d", len(byTag))
	}
	byQ, _ := s.ListItems(ctx, me, ItemFilter{Query: "garage"})
	if len(byQ) != 1 {
		t.Fatalf("search by location name failed: %d", len(byQ))
	}
	if err := s.DeleteLocation(ctx, me, loc.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetItem(ctx, me, it.ID)
	if after.LocationID != nil || after.LocationName != "" {
		t.Fatalf("deleting a location must detach, not delete, items: %+v", after)
	}
	d, err := s.DashboardStats(ctx, me, "EUR", 30)
	if err != nil {
		t.Fatal(err)
	}
	if d.Items != 1 || d.LowStock != 1 || d.Locations != 0 || d.Categories != 1 {
		t.Fatalf("dashboard wrong: %+v", d)
	}
	if err := s.SetArchived(ctx, me, it.ID, true); err != nil {
		t.Fatal(err)
	}
	vis, _ := s.ListItems(ctx, me, ItemFilter{})
	if len(vis) != 0 {
		t.Fatal("archived items must be hidden by default")
	}
	all, _ := s.ListItems(ctx, me, ItemFilter{Archived: true})
	if len(all) != 1 {
		t.Fatal("archived filter should include it")
	}
}

func TestItemUsers(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	a := &User{Email: "a@x", Name: "Ana", Role: "admin"}
	b := &User{Email: "b@x", Name: "Bo", Role: "member"}
	for _, u := range []*User{a, b} {
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	shared := &Item{Name: "Tent", Quantity: 1, UserIDs: []int64{a.ID, b.ID, b.ID, 999}}
	if err := s.CreateItem(ctx, shared, a.ID); err != nil {
		t.Fatal(err)
	}
	if len(shared.Users) != 2 || shared.Users[0].Name != "Ana" || shared.Users[1].Name != "Bo" {
		t.Fatalf("expected two linked users, got %+v", shared.Users)
	}
	mine := &Item{Name: "Headphones", Quantity: 1, UserIDs: []int64{b.ID}}
	if err := s.CreateItem(ctx, mine, b.ID); err != nil {
		t.Fatal(err)
	}
	household := &Item{Name: "Kettle", Quantity: 1}
	if err := s.CreateItem(ctx, household, a.ID); err != nil {
		t.Fatal(err)
	}
	// Bo sees the tent (shared with him) and his headphones, not Ana's kettle.
	forB, _ := s.ListItems(ctx, b.ID, ItemFilter{})
	if len(forB) != 2 {
		t.Fatalf("Bo should see 2 items (own + shared), got %d", len(forB))
	}
	linkedB, _ := s.ListItems(ctx, a.ID, ItemFilter{UserID: b.ID})
	if len(linkedB) != 1 || linkedB[0].Name != "Tent" {
		t.Fatalf("Ana's view filtered to Bo should be the tent only, got %+v", linkedB)
	}
	forA, _ := s.ListItems(ctx, a.ID, ItemFilter{UserID: a.ID})
	if len(forA) != 1 {
		t.Fatalf("Ana should see 1 linked item, got %d", len(forA))
	}
	none, _ := s.ListItems(ctx, a.ID, ItemFilter{UserID: -1})
	if len(none) != 1 || none[0].Name != "Kettle" {
		t.Fatalf("unshared filter wrong: %+v", none)
	}
	shared.UserIDs = []int64{a.ID}
	if err := s.UpdateItem(ctx, shared, "", a.ID); err != nil {
		t.Fatal(err)
	}
	if len(shared.Users) != 1 {
		t.Fatalf("update should replace links, got %+v", shared.Users)
	}
	if _, err := s.AdjustQuantity(ctx, mine.ID, 1, "found another", b.ID); err != nil {
		t.Fatal(err)
	}
	mv, _ := s.ListMovements(ctx, b.ID, mine.ID, 5)
	if len(mv) != 2 || mv[0].UserName != "Bo" || mv[0].UserID == nil || *mv[0].UserID != b.ID {
		t.Fatalf("movement actor missing: %+v", mv)
	}
	// Bo adjusts the tent Ana shared with him: the movement survives his deletion with no actor.
	if _, err := s.AdjustQuantity(ctx, shared.ID, 1, "", a.ID); err != nil {
		t.Fatal(err)
	}
	shared.UserIDs = []int64{a.ID, b.ID}
	if err := s.UpdateItem(ctx, shared, "", a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdjustQuantity(ctx, shared.ID, -1, "took one", b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetItem(ctx, a.ID, shared.ID)
	if len(after.Users) != 1 {
		t.Fatalf("deleting a user must drop links: %+v", after.Users)
	}
	mv, _ = s.ListMovements(ctx, a.ID, shared.ID, 5)
	if mv[0].UserID != nil || mv[0].UserName != "" {
		t.Fatalf("movement should survive user deletion with no actor: %+v", mv[0])
	}
	// Bo's own items went with his account.
	var n int
	_ = s.DB.QueryRow(`SELECT COUNT(1) FROM items WHERE id = ?`, mine.ID).Scan(&n)
	if n != 0 {
		t.Fatal("a deleted account's own items must be deleted with it")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	a := mkUser(t, s, "a@x", "A", "admin").ID
	b := mkUser(t, s, "b@x", "B", "member").ID
	st, err := s.GetSettings(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if st.Title != "Pocket Inventory" || st.Currency != "EUR" {
		t.Fatalf("defaults wrong: %+v", st)
	}
	st.Title, st.Currency, st.Landing, st.ExpiryWarnDays = "Our stuff", "BAM", "shopping", 14
	if err := s.SaveGlobalSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveUserSettings(ctx, a, st); err != nil {
		t.Fatal(err)
	}
	back, _ := s.GetSettings(ctx, a)
	if back.Title != "Our stuff" || back.Currency != "BAM" || back.Landing != "shopping" || back.ExpiryWarnDays != 14 {
		t.Fatalf("round trip failed: %+v", back)
	}
	other, _ := s.GetSettings(ctx, b)
	if other.Title != "Our stuff" || other.Currency != "EUR" || other.Landing != "items" || other.ExpiryWarnDays != 30 {
		t.Fatalf("preferences must be per person (title box-wide): %+v", other)
	}
}

func TestTokensAndSessions(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	u := &User{Email: "a@b.c", Name: "A", Role: "admin"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	plain, tok, err := s.CreateAPIToken(ctx, u.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if tok.Prefix != plain[:10] || plain[:3] != "pi_" {
		t.Fatalf("token shape wrong: %s %s", plain, tok.Prefix)
	}
	if got, err := s.GetUserByAPIToken(ctx, plain); err != nil || got.ID != u.ID {
		t.Fatalf("token lookup failed: %v", err)
	}
	if _, err := s.GetUserByAPIToken(ctx, "pi_nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

// TestMigratePerUser builds a v0.1 database (0001 only) with an admin, a
// member and shared data, then opens it: every row belongs to the admin,
// items keep their place and category through the table rebuilds, personal
// settings move to the admin and names become unique per person.
func TestMigratePerUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	init, _ := migrationFS.ReadFile("migrations/0001_init.sql")
	for _, q := range []string{
		`CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`,
		string(init),
		`INSERT INTO schema_migrations VALUES ('0001_init.sql', 'x')`,
		`INSERT INTO users (id, email, name, role) VALUES (1, 'owner@x', 'Owner', 'admin'), (2, 'member@x', 'Member', 'member')`,
		`INSERT INTO locations (id, name) VALUES (1, 'Garage'), (2, 'Pantry')`,
		`INSERT INTO categories (id, name) VALUES (1, 'Tools'), (2, 'Food')`,
		`INSERT INTO items (id, name, location_id, category_id, quantity) VALUES (1, 'Drill', 1, 1, 1), (2, 'Rice', 2, 2, 3), (3, 'Loose', NULL, NULL, 1)`,
		`INSERT INTO item_users VALUES (2, 2)`,
		`INSERT INTO movements (item_id, user_id, delta, quantity_after) VALUES (1, 1, 1, 1), (2, 2, 3, 3)`,
		`INSERT INTO settings VALUES ('title', 'Home'), ('currency', 'BAM'), ('landing', 'shopping'), ('setup_complete', '1')`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("%s: %v", q[:min(len(q), 40)], err)
		}
	}
	raw.Close()

	s, err := Open(path, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for _, tbl := range []string{"items", "locations", "categories"} {
		var total, owned int
		_ = s.DB.QueryRow(`SELECT COUNT(1), COALESCE(SUM(user_id = 1), 0) FROM `+tbl).Scan(&total, &owned)
		if total == 0 || total != owned {
			t.Errorf("%s: %d rows, %d owned by the admin", tbl, total, owned)
		}
	}
	drill, err := s.GetItem(ctx, 1, 1)
	if err != nil || drill.LocationName != "Garage" || drill.CategoryName != "Tools" {
		t.Fatalf("rebuild lost the item's place/category: %+v %v", drill, err)
	}
	if rice, err := s.GetItem(ctx, 2, 2); err != nil || rice.LocationName != "Pantry" {
		t.Errorf("shared item must stay visible to the member: %v", err)
	}
	if _, err := s.GetItem(ctx, 2, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("unshared item visible to the member: %v", err)
	}
	var moves int
	_ = s.DB.QueryRow(`SELECT COUNT(1) FROM movements`).Scan(&moves)
	if moves != 2 {
		t.Errorf("movements lost: %d", moves)
	}
	owner, _ := s.GetSettings(ctx, 1)
	member, _ := s.GetSettings(ctx, 2)
	if owner.Title != "Home" || owner.Currency != "BAM" || owner.Landing != "shopping" || !owner.SetupComplete {
		t.Errorf("owner settings: %+v", owner)
	}
	if member.Title != "Home" || member.Currency != "EUR" || member.Landing != "items" {
		t.Errorf("member settings: %+v", member)
	}
	if err := s.CreateLocation(ctx, 2, &Location{Name: "garage"}); err != nil {
		t.Errorf("member cannot have their own Garage: %v", err)
	}
	if err := s.CreateLocation(ctx, 1, &Location{Name: "garage"}); !errors.Is(err, ErrConflict) {
		t.Errorf("owner's duplicate place: %v", err)
	}
	var fk int
	_ = s.DB.QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	if fk != 1 {
		t.Error("foreign keys must be back on after migrating")
	}
	// re-opening is a no-op
	s.Close()
	s2, err := Open(path, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
}

func TestAdoptOrphans(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.CreateItem(ctx, &Item{Name: "Before anyone"}, 0); err != nil {
		t.Fatal(err)
	}
	u := mkUser(t, s, "first@x", "First", "admin")
	if items, _ := s.ListItems(ctx, u.ID, ItemFilter{}); len(items) != 0 {
		t.Fatal("ownerless rows must be invisible until adopted")
	}
	if err := s.AdoptOrphans(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if items, _ := s.ListItems(ctx, u.ID, ItemFilter{}); len(items) != 1 {
		t.Fatal("first account should adopt ownerless rows")
	}
}
