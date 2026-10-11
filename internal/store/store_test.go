package store

import (
	"context"
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

func TestItemsLowStockAndMovements(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	loc := &Location{Name: "Garage"}
	if err := s.CreateLocation(ctx, loc); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLocation(ctx, &Location{Name: "garage"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict on duplicate name, got %v", err)
	}
	cat := &Category{Name: "Consumables"}
	if err := s.CreateCategory(ctx, cat); err != nil {
		t.Fatal(err)
	}
	it := &Item{Name: "AA batteries", Quantity: 8, Unit: "pcs", MinQuantity: 4, LocationID: &loc.ID, CategoryID: &cat.ID, Tags: "power, power ,Spare"}
	if err := s.CreateItem(ctx, it, 0); err != nil {
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
	got, err := s.AdjustQuantity(ctx, it.ID, -5, "used", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Quantity != 3 || !got.Low {
		t.Fatalf("expected 3 and low, got %v low=%v", got.Quantity, got.Low)
	}
	got, err = s.AdjustQuantity(ctx, it.ID, -10, "oops", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Quantity != 0 {
		t.Fatalf("quantity must clamp at zero, got %v", got.Quantity)
	}
	moves, err := s.ListMovements(ctx, it.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(moves) != 3 || moves[0].Delta != -3 || moves[2].Delta != 8 {
		t.Fatalf("unexpected movements: %+v", moves)
	}
	low, err := s.ListItems(ctx, ItemFilter{LowOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(low) != 1 {
		t.Fatalf("expected one low item, got %d", len(low))
	}
	byTag, _ := s.ListItems(ctx, ItemFilter{Tag: "spare"})
	if len(byTag) != 1 {
		t.Fatalf("tag filter failed: %d", len(byTag))
	}
	byQ, _ := s.ListItems(ctx, ItemFilter{Query: "garage"})
	if len(byQ) != 1 {
		t.Fatalf("search by location name failed: %d", len(byQ))
	}
	if err := s.DeleteLocation(ctx, loc.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetItem(ctx, it.ID)
	if after.LocationID != nil || after.LocationName != "" {
		t.Fatalf("deleting a location must detach, not delete, items: %+v", after)
	}
	d, err := s.DashboardStats(ctx, "EUR", 30)
	if err != nil {
		t.Fatal(err)
	}
	if d.Items != 1 || d.LowStock != 1 || d.Locations != 0 || d.Categories != 1 {
		t.Fatalf("dashboard wrong: %+v", d)
	}
	if err := s.SetArchived(ctx, it.ID, true); err != nil {
		t.Fatal(err)
	}
	vis, _ := s.ListItems(ctx, ItemFilter{})
	if len(vis) != 0 {
		t.Fatal("archived items must be hidden by default")
	}
	all, _ := s.ListItems(ctx, ItemFilter{Archived: true})
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
	forB, _ := s.ListItems(ctx, ItemFilter{UserID: b.ID})
	if len(forB) != 2 {
		t.Fatalf("Bo should see 2 linked items, got %d", len(forB))
	}
	forA, _ := s.ListItems(ctx, ItemFilter{UserID: a.ID})
	if len(forA) != 1 {
		t.Fatalf("Ana should see 1 linked item, got %d", len(forA))
	}
	none, _ := s.ListItems(ctx, ItemFilter{UserID: -1})
	if len(none) != 1 || none[0].Name != "Kettle" {
		t.Fatalf("household filter wrong: %+v", none)
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
	mv, _ := s.ListMovements(ctx, mine.ID, 5)
	if len(mv) != 2 || mv[0].UserName != "Bo" || mv[0].UserID == nil || *mv[0].UserID != b.ID {
		t.Fatalf("movement actor missing: %+v", mv)
	}
	if err := s.DeleteUser(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetItem(ctx, mine.ID)
	if len(after.Users) != 0 {
		t.Fatalf("deleting a user must drop links: %+v", after.Users)
	}
	mv, _ = s.ListMovements(ctx, mine.ID, 5)
	if mv[0].UserID != nil || mv[0].UserName != "" {
		t.Fatalf("movement should survive user deletion with no actor: %+v", mv[0])
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	st, err := s.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Title != "Pocket Inventory" || st.Currency != "EUR" {
		t.Fatalf("defaults wrong: %+v", st)
	}
	st.Title, st.Currency, st.Landing, st.ExpiryWarnDays = "Our stuff", "BAM", "shopping", 14
	if err := s.SaveSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	back, _ := s.GetSettings(ctx)
	if back.Title != "Our stuff" || back.Currency != "BAM" || back.Landing != "shopping" || back.ExpiryWarnDays != 14 {
		t.Fatalf("round trip failed: %+v", back)
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
