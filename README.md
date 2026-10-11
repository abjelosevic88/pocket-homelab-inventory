<p align="center">
  <img src="web/public/favicon.svg" width="72" alt="">
</p>
<h1 align="center">Pocket Inventory</h1>
<p align="center">
  A self-hosted home inventory for households: what you own, how many, where it is and whose it is.<br>
  <b>One binary · one SQLite file · one volume.</b> No Postgres, no Redis, no five-level location trees.
</p>
<p align="center">
  <a href="https://github.com/abjelosevic88/pocket-homelab-inventory/actions"><img src="https://img.shields.io/github/actions/workflow/status/abjelosevic88/pocket-homelab-inventory/ci.yml?branch=main&label=CI" alt="CI"></a>
  <a href="https://github.com/abjelosevic88/pocket-homelab-inventory/pkgs/container/pocket-homelab-inventory"><img src="https://img.shields.io/badge/ghcr.io-amd64%20%7C%20arm64%20%7C%20armv7-blue" alt="image"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green" alt="MIT"></a>
  <img src="https://img.shields.io/badge/image%20size-~15MB-lightgrey" alt="size">
</p>

---

Most inventory apps die because entering things is a chore. This one is built around the three things you actually do:

- **Add something in one line.** A quick-add box at the top of the list: name, how many, where. Enter. Done. The full form (brand, serial, price, warranty, barcode, photo) is there when you want it, folded away when you don't.
- **Tap − when you use one, + when you buy more.** Every item has a plus/minus right on the list. Set an *alert below* level and the item appears on the **shopping list** by itself, grouped by place, with a *Bought* button that puts it back.
- **Know whose it is.** Several people sign in to the same inventory. An item can belong to one person, a few, or the whole household. Filter by *Mine*, by a name, or by *Household*. Every quantity change records who did it.

## Features

| Area | What you get |
|---|---|
| **Items** | Name, quantity + unit, alert level, place, category, tags, notes, photo. Optional details: brand, model, serial, barcode/SKU, price, purchase date, expiry / warranty date, link. Archive instead of delete. |
| **List** | Search across everything (names, serials, barcodes, tags, places), filter by place / category / person / tag, sort by name, recent, lowest quantity, place or expiry. Stat tiles for low stock, expiring, archived. |
| **Shopping list** | Everything at or below its alert level, grouped by place, with *need N* and one-tap *Bought N*. Expiring and expired items below it. *Copy as text* for the phone. |
| **People** | Multi-user with roles (admin manages users, members do everything else). Items link to any number of users. Per-person filters, avatars on rows, who-did-what history. |
| **Places & categories** | Two flat lists, reorderable, with item counts. Deleting one detaches items rather than deleting them. New ones can be created inline from the item form. |
| **History** | Every quantity change: delta, resulting quantity, note, who, when. Per item and as a global activity feed. |
| **Automation** | REST API with personal tokens: `GET /api/v1/dashboard` for Homepage/Glance widgets, `GET /api/v1/lookup?barcode=` for scanner flows, `POST /api/v1/items/{id}/adjust` for Shortcuts / Home Assistant / n8n. Prometheus `/metrics`, `/healthz`, `/readyz`. |
| **Ops-friendly** | Single static binary, embedded SPA, SQLite with WAL, `VACUUM INTO` backups from UI/CLI/API, JSON + CSV export, JSON + CSV import, proxy auth, non-root container, dark mode, installable PWA. |

## Quick start

```bash
mkdir pocket-inventory && cd pocket-inventory
curl -O https://raw.githubusercontent.com/abjelosevic88/pocket-homelab-inventory/main/docker-compose.yml
docker compose up -d
```

Open `http://<host>:8080`, create your account, add the rest of the household under **Settings → Users**, and start typing into the quick-add box.

<details>
<summary><b>docker run one-liner</b></summary>

```bash
docker run -d --name pocket-inventory -p 8080:8080 -v $PWD/data:/data \
  -e TZ=Europe/Berlin -e BASE_URL=https://inventory.example.com \
  ghcr.io/abjelosevic88/pocket-homelab-inventory:latest
```
</details>

<details>
<summary><b>Try it with demo data</b></summary>

```bash
docker run --rm -p 8080:8080 -e DEMO_DATA=true -e AUTH_MODE=none ghcr.io/abjelosevic88/pocket-homelab-inventory:latest
```
Six places, twenty-odd items, a second user, a few things running low and one past its date. No login.
</details>

<details>
<summary><b>Bare metal / systemd</b></summary>

Download a binary from the [releases page](https://github.com/abjelosevic88/pocket-homelab-inventory/releases), drop it in `/usr/local/bin`, and use [`deploy/systemd/pocket-inventory.service`](deploy/systemd/pocket-inventory.service).
</details>

## Configuration

Everything is an environment variable with a sensible default. The important ones:

| Variable | Default | Notes |
|---|---|---|
| `TZ` | `UTC` | Decides what "today" is for expiry dates. Set it. |
| `BASE_URL` | – | Public URL (informational). |
| `DATA_DIR` | `/data` | Database, photos and backups live here. |
| `AUTH_MODE` | `local` | `local` (built-in login, multi-user), `proxy` (trust `Remote-User` from Authelia/Authentik/Tinyauth; users auto-created), `none` (single implicit user, no login — LAN only!). |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | – | Create the first admin without the setup screen. |
| `SECURE_COOKIES` | `false` | Set `true` behind HTTPS. |
| `MAX_PHOTO_MB` | `10` | Upload limit per photo. |
| `DEMO_DATA` | `false` | Seed a sample household into an empty database. |

Full list in [docs/configuration.md](docs/configuration.md). Reverse-proxy snippets in [docs/reverse-proxy.md](docs/reverse-proxy.md).

## Automation

Create a token under **Settings → API tokens**, then:

```bash
# used two AA batteries, from an iOS Shortcut / Home Assistant / n8n
curl -X POST https://inventory.example.com/api/v1/items/12/adjust \
  -H "Authorization: Bearer pi_…" -H "Content-Type: application/json" \
  -d '{"delta": -2, "note": "remote"}'

# scanned a barcode — which item is it?
curl -H "Authorization: Bearer pi_…" "https://inventory.example.com/api/v1/lookup?barcode=4902580700416"

# compact status for a Homepage / Glance widget
curl -H "Authorization: Bearer pi_…" https://inventory.example.com/api/v1/dashboard
# → {"items":142,"low_stock":4,"expiring":2,"total_value":3180.5,"currency":"EUR",…}
```

Everything else is in [docs/api.md](docs/api.md), including a Homepage `customapi` widget.

## Backups

The database is a single file; photos sit next to it in `photos/`.

- **UI:** Settings → Backup & data → *Download database* (a consistent `VACUUM INTO` copy).
- **CLI:** `docker exec pocket-inventory pocket-inventory backup` writes to `/data/backups/`.
- **API:** `GET /api/v1/backup`.
- **Export:** JSON (full, re-importable) or CSV (one row per item, also importable).

Restic / Borg / Backrest users: back up `DATA_DIR`, but snapshot the live `pocket-inventory.db` through one of the above rather than copying the WAL files raw. See [docs/backup.md](docs/backup.md).

## Development

```bash
make dev      # Go API on :8080 (demo data, no login) + Vite on :5173
make test     # go vet + go test + tsc
make all      # production build: web UI embedded into ./pocket-inventory
```

Go 1.22+ and Node 20+. Stack: Go + chi + modernc SQLite (no CGO), React 19 + Vite + TypeScript, no UI framework.

## Credits

Sibling projects built the same way: [Pocket Habits](https://github.com/abjelosevic88/pocket-homelab-habit-tracker) and [Pocket Invoicing](https://github.com/abjelosevic88/pocket-homelab-invoicing).

## License

MIT
