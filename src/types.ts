// === Client → Server Messages ===

export type ClientMessage =
  | { type: "message"; text: string }
  | { type: "skip" }
  | { type: "typing"; state: "typing" | "stopped" };

// === Server → Client Messages ===

export type ServerMessage =
  | { type: "matched"; room_id: string; stranger: StrangerProfile }
  | { type: "message"; from: "stranger"; text: string; timestamp: string }
  | { type: "stranger_left" }
  | { type: "waiting" }
  | { type: "error"; message: string }
  | { type: "typing"; state: "typing" | "stopped" };

export interface StrangerProfile {
  username: string;
  avatar_url: string;
  bio: string;
  public_repos: number;
  github_created_at: string;
  top_languages: string;
  top_repo: string;
  top_repo_stars: number;
  contributions: number;
  contribution_graph: string;
}

// === Session ===

export interface Session {
  github_id: number;
  username: string;
  avatar_url: string;
  bio: string;
  public_repos: number;
  github_created_at: string;
  top_languages: string;
  top_repo: string;
  top_repo_stars: number;
  contributions: number;
  contribution_graph: string;
}

// === DB Models ===

export interface Profile {
  github_id: number;
  username: string;
  avatar_url: string;
  bio: string;
  public_repos: number;
  github_created_at: string;
  top_languages: string;
  top_repo: string;
  top_repo_stars: number;
  contributions: number;
  created_at: string;
  last_seen: string;
}

// === Env Bindings ===

export interface Env {
  MATCHMAKER: DurableObjectNamespace;
  CHATROOM: DurableObjectNamespace;
  DB: D1Database;
  SESSIONS: KVNamespace;
  GITHUB_CLIENT_ID: string;
  GITHUB_CLIENT_SECRET: string;
  CALLBACK_URL: string;
}
