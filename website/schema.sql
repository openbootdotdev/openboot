DROP TABLE IF EXISTS configs;
DROP TABLE IF EXISTS users;

CREATE TABLE users (
  id TEXT PRIMARY KEY,
  username TEXT UNIQUE NOT NULL,
  email TEXT,
  avatar_url TEXT,
  created_at TEXT DEFAULT (datetime('now')),
  updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE configs (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT,
  base_preset TEXT DEFAULT 'developer',
  packages TEXT,
  custom_script TEXT,
  is_public INTEGER DEFAULT 1,
  alias TEXT UNIQUE,
  created_at TEXT DEFAULT (datetime('now')),
  updated_at TEXT DEFAULT (datetime('now')),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  UNIQUE(user_id, slug)
);

CREATE INDEX idx_configs_user_id ON configs(user_id);
CREATE INDEX idx_configs_alias ON configs(alias);
CREATE INDEX idx_users_username ON users(username);
