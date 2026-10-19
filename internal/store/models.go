package store

// User is an account that can sign in. The first user is the admin; admins
// manage accounts and box-wide settings. Everyone's items, places and
// categories are their own; admins get no extra access to them.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// UserRef is the short form of a user shown on items and movements.
type UserRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Location is a place where items live.
type Location struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
	ItemCount   int    `json:"item_count"`
	CreatedAt   string `json:"created_at"`
}

// Category is a kind of item.
type Category struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
	ItemCount int    `json:"item_count"`
	CreatedAt string `json:"created_at"`
}

// Item is one thing (or one stack of the same thing) you own.
type Item struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Notes        string  `json:"notes"`
	CategoryID   *int64  `json:"category_id"`
	LocationID   *int64  `json:"location_id"`
	Quantity     float64 `json:"quantity"`
	Unit         string  `json:"unit"`
	MinQuantity  float64 `json:"min_quantity"`
	Barcode      string  `json:"barcode"`
	Brand        string  `json:"brand"`
	Model        string  `json:"model"`
	Serial       string  `json:"serial"`
	PurchaseDate string  `json:"purchase_date"`
	Price        float64 `json:"price"`
	ExpiryDate   string  `json:"expiry_date"`
	Tags         string  `json:"tags"`
	URL          string  `json:"url"`
	Photo        string  `json:"photo"`
	Archived     bool    `json:"archived"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`

	// OwnerID is the person who created the item; only they see it unless it is
	// shared. UserIDs / Users are the people it is shared with ("whose is it?").
	OwnerID int64     `json:"owner_id"`
	UserIDs []int64   `json:"user_ids"`
	Users   []UserRef `json:"users"`

	// Joined / derived, read-only.
	CategoryName string `json:"category_name"`
	LocationName string `json:"location_name"`
	Low          bool   `json:"low"`
}

// Movement is one quantity change of an item.
type Movement struct {
	ID            int64   `json:"id"`
	ItemID        int64   `json:"item_id"`
	UserID        *int64  `json:"user_id"`
	UserName      string  `json:"user_name"`
	Delta         float64 `json:"delta"`
	QuantityAfter float64 `json:"quantity_after"`
	Note          string  `json:"note"`
	CreatedAt     string  `json:"created_at"`
}

// Settings merges the box-wide settings (Title, setup flags; admin-only to
// change) with the signed-in person's own preferences (the rest).
type Settings struct {
	Title          string `json:"title"`
	Currency       string `json:"currency"`     // ISO code shown next to prices
	DefaultUnit    string `json:"default_unit"` // pre-filled unit for new items ("pcs")
	Landing        string `json:"landing"`      // items | shopping
	ExpiryWarnDays int    `json:"expiry_warn_days"`
	SetupComplete  bool   `json:"setup_complete"`
	DefaultsSeeded bool   `json:"-"`
}

// Dashboard is the compact status used by widgets.
type Dashboard struct {
	Items         int     `json:"items"`
	QuantityTotal float64 `json:"quantity_total"`
	LowStock      int     `json:"low_stock"`
	TotalValue    float64 `json:"total_value"`
	Currency      string  `json:"currency"`
	Locations     int     `json:"locations"`
	Categories    int     `json:"categories"`
	Users         int     `json:"users"`
	Expiring      int     `json:"expiring"`
	Expired       int     `json:"expired"`
	UpdatedToday  int     `json:"updated_today"`
	Archived      int     `json:"archived"`
}
