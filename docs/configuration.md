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
| `CORE_URL` / `CORE_REGISTRATION_KEY` | – | [Pocket Core](https://git.bjelke.org/abjelosevic88/pocket-homelab-core): register on start (the key is needed once; the minted app token lives in `$DATA_DIR/core.token`), publish `inventory.item_created` / `inventory.adjusted` / `inventory.item_archived`, and accept the tokens Core hands to paired phones. |
| `CORE_APP_URL` | `BASE_URL` | How Core reaches this app for health checks (e.g. `http://host.docker.internal:8170`). |
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
| `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | – | Sign in with an OpenID Connect provider (PocketID, Authelia, Authentik, Keycloak…). Needs `BASE_URL`; the callback is `BASE_URL/api/v1/auth/oidc/callback`. On a box with Pocket Core none of these are needed — the app receives its client when it registers. |
| `OIDC_NAME` | `single sign-on` | Button label: "Sign in with …" (the Core-issued client is labelled "Pocket Core"). |
| `OIDC_AUTO_CREATE` | `true` | Give an unknown (verified) email a member account on first sign-in instead of refusing it. Always on for the Core-issued client. |

Roles: **admin** can add/edit/remove users; **member** can do everything else. There must always be at least one admin. API tokens (`Authorization: Bearer …` or `X-API-Key`) work in every mode and act as the user who created them.

### Single sign-on (OIDC)

In `local` mode a configured provider adds a "Sign in with …" button above the password form; the password form stays. Pocket Inventory uses the authorization-code flow with PKCE and the client secret, checks the ID token's RS256 signature against the provider's published keys (plus issuer, audience, expiry and nonce) and matches the person to a user by **verified** email. With Pocket Core on the box this is zero configuration: the app asks for a relying-party client when it registers and uses the one Core issues (kept in `$DATA_DIR/core.token.oidc.json`); explicit `OIDC_*` variables take precedence when set. Without Core and without `OIDC_*` nothing changes: no button, and `/api/v1/auth/oidc/*` answers 404.


## In-app settings

Under **Settings → General**: app title, currency code (for prices and the total value), default unit for new items, expiry warning window in days, and which page opens first.

## Forgot your password?

```bash
docker exec pocket-inventory pocket-inventory reset-password            # first user, random password printed
docker exec pocket-inventory pocket-inventory reset-password -email me@example.com -password NewPass123
```
