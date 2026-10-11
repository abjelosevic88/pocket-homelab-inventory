# API

Base path `/api/v1`. JSON in, JSON out. Authenticate with the session cookie (browser) or a personal token:

```
Authorization: Bearer pi_xxxxxxxx…
```

Create tokens under **Settings → API tokens**. Dates are `YYYY-MM-DD`. Quantities are numbers (decimals allowed).

For a ready-made client, [Pocket Homelab](https://pockethomelab.com/) (iPhone, iPad, Mac, Android) uses this API with a token: the items with − and +, the quick add, an item's page and editor, the shopping list, places and categories, the activity feed. See the README's [Companion app](../README.md#companion-app-pocket-homelab) section.

## Items

| Method | Path | Notes |
|---|---|---|
| `GET` | `/items` | List. Filters: `q=` (search), `location=<id>\|none`, `category=<id>\|none`, `user=<id>\|me\|none`, `tag=`, `barcode=`, `low=1`, `expiring=<days>`, `archived=1` (include archived), `sort=name\|updated\|created\|quantity\|location\|expiry`, `limit=`. |
| `POST` | `/items` | Create (body below). Returns the item. |
| `GET` | `/items/{id}` | `{"item": …, "movements": [last 30]}` |
| `PUT` | `/items/{id}` | Update every field (same body as create). Add `"note"` to describe a quantity change. |
| `DELETE` | `/items/{id}` | Delete item, photo and history. Prefer archive. |
| `POST` | `/items/{id}/adjust` | `{"delta": -2, "note": "used"}` **or** `{"quantity": 10, "note": "stock-take"}`. Never goes below zero. Returns the item. |
| `POST` | `/items/{id}/archive` · `/unarchive` | Hide / restore. |
| `GET` | `/items/{id}/movements?limit=` | History, newest first. |
| `GET` · `POST` · `DELETE` | `/items/{id}/photo` | Fetch / upload (`multipart/form-data`, field `photo`, jpeg/png/webp/gif/heic) / remove. |
| `GET` | `/lookup?barcode=` | The item with that barcode, or 404. |

### Item body

```json
{
  "name": "AA batteries",
  "quantity": 8, "unit": "pcs", "min_quantity": 4,
  "location_id": 3,            "location": "Garage",        // id wins; a name creates the place if missing
  "category_id": 2,            "category": "Electronics",   // same
  "user_ids": [1, 2],          // people it belongs to; [] = whole household
  "tags": "power, spare",
  "notes": "Top drawer of the red toolbox.",
  "brand": "Panasonic", "model": "Eneloop", "serial": "", "barcode": "4902580700416",
  "price": 0.6, "purchase_date": "2026-09-01", "expiry_date": "",
  "url": "https://…"
}
```

Every key except `name` is optional. The response adds `id`, `location_name`, `category_name`, `users` (`[{id,name}]`), `low` (true when `min_quantity > 0` and `quantity <= min_quantity`), `photo`, `archived`, timestamps.

## Lists & status

| Method | Path | Notes |
|---|---|---|
| `GET` | `/dashboard` | `{items, quantity_total, low_stock, total_value, currency, locations, categories, users, expiring, expired, updated_today, archived}` |
| `GET` | `/shopping` | `{groups: [{location, items}], low_count, expiring: [items], today}` |
| `GET` | `/activity?limit=` | Latest movements across all items, with `item_name`, `unit`, `user_name`. |
| `GET` | `/tags` | Distinct tags in use. |

## Places, categories, users

| Method | Path | Notes |
|---|---|---|
| `GET` · `POST` | `/locations` | `{"name", "description"}`; names are unique (case-insensitive). |
| `PUT` · `DELETE` | `/locations/{id}` | Deleting detaches items. |
| `PUT` | `/locations/reorder` | `{"ids":[3,1,2]}` |
| `GET` · `POST` | `/categories` | `{"name", "color"}` |
| `PUT` · `DELETE` | `/categories/{id}` | |
| `PUT` | `/categories/reorder` | `{"ids":[…]}` |
| `GET` | `/users` | Every signed-in user can list users (needed to link items). |
| `POST` | `/users` | Admin only. `{"email", "name", "password", "role": "member"\|"admin"}` |
| `PUT` · `DELETE` | `/users/{id}` | Admin only. There must always be one admin; you cannot delete yourself. |

## Account, settings, data

| Method | Path | Notes |
|---|---|---|
| `GET` | `/auth/status` | `{needs_setup, auth_mode, authenticated, user, version, title}` |
| `POST` | `/auth/setup` · `/auth/login` · `/auth/logout` | |
| `GET` · `PUT` | `/auth/me` | Own profile, `{"name","email","current_password","new_password"}` |
| `GET` · `PUT` | `/settings` | `{title, currency, default_unit, landing, expiry_warn_days}` |
| `GET` | `/settings/system` | Version, paths, sizes, counts. |
| `GET` · `POST` · `DELETE` | `/tokens` | Personal tokens. The plaintext is returned once on create. |
| `GET` | `/backup` | Consistent SQLite copy. |
| `GET` | `/export.json` · `/export.csv` | Full export / one row per item. |
| `POST` | `/import` | The JSON export. Places matched by name, items by name + place (update or create). |
| `POST` | `/import.csv` | CSV body or `multipart/form-data` field `file`. Needs a `name` column; others optional (see export). `owners` is `;`-separated user names. |

Unauthenticated: `/healthz`, `/readyz`, `/metrics` (Prometheus; disable with `METRICS_ENABLED=false` or block at the proxy).

## Homepage widget

```yaml
- Pocket Inventory:
    href: https://inventory.example.com/
    icon: mdi-package-variant-closed
    widget:
      type: customapi
      url: http://pocket-inventory:8080/api/v1/dashboard
      refreshInterval: 300000
      headers:
        Authorization: Bearer {{HOMEPAGE_VAR_POCKET_INVENTORY_TOKEN}}
      mappings:
        - { field: items, label: Items, format: text }
        - { field: low_stock, label: Low, format: text }
        - { field: expiring, label: Expiring, format: text }
        - { field: total_value, label: Value, format: number }
```

## iOS Shortcut: scan → minus one

1. *Scan QR/Barcode* → 2. *Get Contents of URL* `GET …/lookup?barcode=[Barcode]` with the bearer header → 3. *Get Dictionary Value* `id` → 4. *Get Contents of URL* `POST …/items/[id]/adjust` with JSON `{"delta": -1, "note": "scanned"}`.
