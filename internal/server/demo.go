package server

import (
	"context"
	"time"

	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
)

// seedDemo fills an empty database with a believable household so screenshots
// and first impressions work.
func (s *Server) seedDemo(ctx context.Context) error {
	if items, _ := s.store.ListItems(ctx, store.ItemFilter{Archived: true, Limit: 1}); len(items) > 0 {
		return nil
	}
	locs := map[string]*store.Location{}
	for _, n := range []string{"Garage", "Kitchen pantry", "Bathroom cabinet", "Office", "Hallway closet", "Attic"} {
		l := &store.Location{Name: n}
		if err := s.store.CreateLocation(ctx, l); err != nil {
			return err
		}
		locs[n] = l
	}
	cats := map[string]*store.Category{}
	all, _ := s.store.ListCategories(ctx)
	for i := range all {
		cats[all[i].Name] = &all[i]
	}
	users, _ := s.store.ListUsers(ctx)
	var me, partner int64
	if len(users) > 0 {
		me = users[0].ID
	}
	if len(users) > 1 {
		partner = users[1].ID
	} else {
		p := &store.User{Email: "sam@example.com", Name: "Sam", Role: "member"}
		if err := s.store.CreateUser(ctx, p); err == nil {
			partner = p.ID
		}
	}
	type row struct {
		name, loc, cat, unit, tags, brand string
		qty, min, price                   float64
		expiry                            string
		owners                            []int64
	}
	soon := time.Now().AddDate(0, 0, 12).Format("2006-01-02")
	past := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	rows := []row{
		{"AA batteries", "Garage", "Electronics", "pcs", "power, spare", "Panasonic", 3, 8, 0.6, "", nil},
		{"AAA batteries", "Garage", "Electronics", "pcs", "power, spare", "Panasonic", 12, 6, 0.5, "", nil},
		{"USB-C cables", "Office", "Electronics", "pcs", "cables", "Anker", 5, 2, 9, "", []int64{me}},
		{"HDMI cable 2 m", "Office", "Electronics", "pcs", "cables", "", 2, 0, 7, "", nil},
		{"Cordless drill", "Garage", "Tools", "pcs", "power tools", "Bosch", 1, 0, 120, "", []int64{me}},
		{"Drill bits set", "Garage", "Tools", "set", "", "Bosch", 1, 0, 25, "", nil},
		{"Duct tape", "Garage", "Tools", "roll", "consumable", "", 1, 2, 4, "", nil},
		{"Olive oil", "Kitchen pantry", "Food & drink", "l", "cooking", "", 1, 1, 8, "", nil},
		{"Rice", "Kitchen pantry", "Food & drink", "kg", "staple", "", 4, 2, 2, "", nil},
		{"Coffee beans", "Kitchen pantry", "Food & drink", "kg", "staple", "Lavazza", 0.5, 1, 18, soon, nil},
		{"Canned tomatoes", "Kitchen pantry", "Food & drink", "cans", "", "", 6, 4, 1.2, "", nil},
		{"Ibuprofen 400", "Bathroom cabinet", "Medicine", "tablets", "", "", 10, 10, 5, past, nil},
		{"Plasters", "Bathroom cabinet", "Medicine", "box", "first aid", "", 2, 1, 3, "", nil},
		{"Toothpaste", "Bathroom cabinet", "Household", "tubes", "", "", 1, 2, 2.5, "", nil},
		{"Dishwasher tablets", "Kitchen pantry", "Household", "pcs", "cleaning", "", 14, 10, 0.3, "", nil},
		{"Printer paper", "Office", "Household", "sheets", "", "", 150, 100, 0.01, "", nil},
		{"Passports", "Office", "Documents", "pcs", "important", "", 2, 0, 0, "2031-04-20", []int64{me, partner}},
		{"Camping tent", "Attic", "Outdoor", "pcs", "camping", "Quechua", 1, 0, 180, "", []int64{me, partner}},
		{"Sleeping bags", "Attic", "Outdoor", "pcs", "camping", "", 2, 0, 60, "", []int64{me, partner}},
		{"Winter coat", "Hallway closet", "Clothing", "pcs", "", "", 1, 0, 150, "", []int64{partner}},
		{"Running shoes", "Hallway closet", "Clothing", "pairs", "sport", "Hoka", 1, 0, 140, "", []int64{me}},
		{"Noise-cancelling headphones", "Office", "Electronics", "pcs", "", "Sony", 1, 0, 300, "", []int64{me}},
	}
	for _, r := range rows {
		it := &store.Item{Name: r.name, Unit: r.unit, Tags: r.tags, Brand: r.brand, Quantity: r.qty, MinQuantity: r.min, Price: r.price, ExpiryDate: r.expiry}
		if l := locs[r.loc]; l != nil {
			it.LocationID = &l.ID
		}
		if c := cats[r.cat]; c != nil {
			it.CategoryID = &c.ID
		}
		for _, o := range r.owners {
			if o > 0 {
				it.UserIDs = append(it.UserIDs, o)
			}
		}
		if err := s.store.CreateItem(ctx, it, me); err != nil {
			return err
		}
	}
	return nil
}
