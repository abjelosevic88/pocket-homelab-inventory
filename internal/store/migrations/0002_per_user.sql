-- Every record belongs to exactly one person. Items, places and categories get
-- an owner (user_id); a person sees an item only when they own it or it is
-- shared with them through item_users. item_users and movements stay children
-- of items and are only ever read through a visible item.
--
-- Everything that exists before this migration belongs to the original admin
-- (members had no way to own anything yet). The owner is the first admin, or
-- the first user when there is no admin. A database without users keeps
-- user_id NULL; the first account created adopts those rows.
--
-- Places and categories are rebuilt so their unique name becomes unique per
-- person. The migration runner turns foreign keys off around this file, so
-- dropping the old tables does not null items.location_id / category_id.

CREATE TABLE locations_new (
  id          INTEGER PRIMARY KEY,
  user_id     INTEGER REFERENCES users(id) ON DELETE CASCADE,
  name        TEXT NOT NULL COLLATE NOCASE,
  description TEXT NOT NULL DEFAULT '',
  sort_order  INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (user_id, name)
);
INSERT INTO locations_new (id, user_id, name, description, sort_order, created_at)
  SELECT id, (SELECT id FROM users ORDER BY CASE WHEN role = 'admin' THEN 0 ELSE 1 END, id LIMIT 1),
         name, description, sort_order, created_at
  FROM locations;
DROP TABLE locations;
ALTER TABLE locations_new RENAME TO locations;
CREATE INDEX locations_user ON locations(user_id, sort_order);

CREATE TABLE categories_new (
  id         INTEGER PRIMARY KEY,
  user_id    INTEGER REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT NOT NULL COLLATE NOCASE,
  color      TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (user_id, name)
);
INSERT INTO categories_new (id, user_id, name, color, sort_order, created_at)
  SELECT id, (SELECT id FROM users ORDER BY CASE WHEN role = 'admin' THEN 0 ELSE 1 END, id LIMIT 1),
         name, color, sort_order, created_at
  FROM categories;
DROP TABLE categories;
ALTER TABLE categories_new RENAME TO categories;
CREATE INDEX categories_user ON categories(user_id, sort_order);

ALTER TABLE items ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE CASCADE;
UPDATE items SET user_id = (SELECT id FROM users ORDER BY CASE WHEN role = 'admin' THEN 0 ELSE 1 END, id LIMIT 1)
  WHERE user_id IS NULL;
CREATE INDEX items_user ON items(user_id);

-- Personal preferences (currency, default unit, landing page, expiry window)
-- move to per-person settings; the owner keeps the values set so far. Only the
-- app title and setup flags stay box-wide in settings.
CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);
INSERT INTO user_settings (user_id, key, value)
  SELECT (SELECT id FROM users ORDER BY CASE WHEN role = 'admin' THEN 0 ELSE 1 END, id LIMIT 1), key, value
  FROM settings
  WHERE key IN ('currency', 'default_unit', 'landing', 'expiry_warn_days')
    AND EXISTS (SELECT 1 FROM users);
DELETE FROM settings WHERE key IN ('currency', 'default_unit', 'landing', 'expiry_warn_days');
