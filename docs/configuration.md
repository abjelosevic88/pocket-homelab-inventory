# Configuration

All settings are environment variables. Nothing is required.

## Server

| Variable | Default | Description |
|---|---|---|
| `HOST` | `0.0.0.0` | Listen address. |
| `PORT` | `8080` | Listen port. |
| `BASE_URL` | – | Public URL (e.g. `https://inventory.example.com`). Informational. |
| `DATA_DIR` | `./data` (`/data` in the container) | Database, photos and backups. |
| `DB_PATH` | `$DATA_DIR/pocket-inventory.db` | Override the database file. |
| `PHOTO_DIR` | `$DATA_DIR/photos` | Where item photos are stored. |
| `MAX_PHOTO_MB` | `10` | Upload limit per photo. |
| `TZ` | `UTC` | Timezone for "today" (expiry maths). The container ships `tzdata`. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |
| `LOG_FORMAT` | `text` | `text` or `json`. |
| `TRUST_PROXY` | `true` | Honour `X-Forwarded-For` / `X-Real-IP` for logging. |
| `METRICS_ENABLED` | `true` | Serve Prometheus metrics on `/metrics` (unauthenticated — restrict at the proxy if exposed). |
| `DEMO_DATA` | `false` | Seed a sample household into an *empty* database. |

## Authentication & users

| Variable | Default | Description |
|---|---|---|
| `AUTH_MODE` | `local` | `local`: built-in email + password, first-run setup screen, multiple users managed under Settings → Users. `proxy`: trust the user name from a header set by your SSO proxy; users are auto-created on first visit (first one becomes admin). `none`: no login at all, one implicit user — only for trusted networks. |
| `AUTH_PROXY_HEADER` | `Remote-User` | Header carrying the user name in `proxy` mode. |
| `AUTH_PROXY_EMAIL_HEADER` | `Remote-Email` | Header carrying the email in `proxy` mode. |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME` | – | In `local` mode, create this admin on first start instead of showing the setup screen. Password must be 8+ characters. |
| `SESSION_TTL` | `720h` | Browser session lifetime. |
| `SECURE_COOKIES` | `false` | Mark the session cookie `Secure`. Turn on behind HTTPS. |

Roles: **admin** can add/edit/remove users; **member** can do everything else. There must always be at least one admin. API tokens (`Authorization: Bearer …` or `X-API-Key`) work in every mode and act as the user who created them.

## In-app settings

Under **Settings → General**: app title, currency code (for prices and the total value), default unit for new items, expiry warning window in days, and which page opens first.

## Forgot your password?

```bash
docker exec pocket-inventory pocket-inventory reset-password            # first user, random password printed
docker exec pocket-inventory pocket-inventory reset-password -email me@example.com -password NewPass123
```
