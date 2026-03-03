import type { Profile } from "./types";

export async function upsertProfile(
  db: D1Database,
  githubId: number,
  username: string,
  avatarUrl: string,
  bio: string,
  publicRepos: number,
  githubCreatedAt: string,
  topLanguages: string,
  topRepo: string,
  topRepoStars: number,
  contributions: number,
  contributionGraph: string,
): Promise<void> {
  await db
    .prepare(
      `INSERT INTO profiles (github_id, username, avatar_url, bio, public_repos, github_created_at, top_languages, top_repo, top_repo_stars, contributions, contribution_graph)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
       ON CONFLICT(github_id) DO UPDATE SET
         username = excluded.username,
         avatar_url = excluded.avatar_url,
         bio = excluded.bio,
         public_repos = excluded.public_repos,
         github_created_at = excluded.github_created_at,
         top_languages = excluded.top_languages,
         top_repo = excluded.top_repo,
         top_repo_stars = excluded.top_repo_stars,
         contributions = excluded.contributions,
         contribution_graph = excluded.contribution_graph,
         last_seen = datetime('now')`,
    )
    .bind(githubId, username, avatarUrl, bio, publicRepos, githubCreatedAt, topLanguages, topRepo, topRepoStars, contributions, contributionGraph)
    .run();
}

export async function getProfile(
  db: D1Database,
  githubId: number,
): Promise<Profile | null> {
  return db
    .prepare("SELECT * FROM profiles WHERE github_id = ?")
    .bind(githubId)
    .first<Profile>();
}

export async function isBanned(
  db: D1Database,
  githubId: number,
): Promise<boolean> {
  const row = await db
    .prepare(
      `SELECT 1 FROM bans
       WHERE github_id = ?
         AND (expires_at IS NULL OR expires_at > datetime('now'))`,
    )
    .bind(githubId)
    .first();
  return row !== null;
}

export async function updateLastSeen(
  db: D1Database,
  githubId: number,
): Promise<void> {
  await db
    .prepare("UPDATE profiles SET last_seen = datetime('now') WHERE github_id = ?")
    .bind(githubId)
    .run();
}
