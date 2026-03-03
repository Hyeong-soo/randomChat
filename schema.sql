CREATE TABLE IF NOT EXISTS profiles (
  github_id INTEGER PRIMARY KEY,
  username TEXT NOT NULL,
  avatar_url TEXT NOT NULL DEFAULT '',
  bio TEXT NOT NULL DEFAULT '',
  public_repos INTEGER NOT NULL DEFAULT 0,
  github_created_at TEXT NOT NULL DEFAULT '',
  top_languages TEXT NOT NULL DEFAULT '',
  top_repo TEXT NOT NULL DEFAULT '',
  top_repo_stars INTEGER NOT NULL DEFAULT 0,
  contributions INTEGER NOT NULL DEFAULT 0,
  contribution_graph TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_seen TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS reports (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  reporter_github_id INTEGER NOT NULL,
  reported_github_id INTEGER NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  FOREIGN KEY (reporter_github_id) REFERENCES profiles(github_id),
  FOREIGN KEY (reported_github_id) REFERENCES profiles(github_id)
);

CREATE TABLE IF NOT EXISTS bans (
  github_id INTEGER PRIMARY KEY,
  ip_hash TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  banned_at TEXT NOT NULL DEFAULT (datetime('now')),
  expires_at TEXT,
  FOREIGN KEY (github_id) REFERENCES profiles(github_id)
);

CREATE TABLE IF NOT EXISTS ip_history (
  github_id INTEGER NOT NULL,
  ip_hash TEXT NOT NULL,
  first_seen TEXT NOT NULL DEFAULT (datetime('now')),
  last_seen TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (github_id, ip_hash),
  FOREIGN KEY (github_id) REFERENCES profiles(github_id)
);

CREATE INDEX IF NOT EXISTS idx_reports_reported ON reports(reported_github_id);
CREATE INDEX IF NOT EXISTS idx_ip_history_ip ON ip_history(ip_hash);
