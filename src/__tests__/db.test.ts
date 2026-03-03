import { describe, it, expect, beforeAll } from "vitest";
import { env } from "cloudflare:test";
import { upsertProfile, getProfile, isBanned } from "../db";

describe("Database", () => {
  beforeAll(async () => {
    await env.DB.exec("CREATE TABLE IF NOT EXISTS profiles (github_id INTEGER PRIMARY KEY, username TEXT NOT NULL, avatar_url TEXT NOT NULL DEFAULT '', bio TEXT NOT NULL DEFAULT '', public_repos INTEGER NOT NULL DEFAULT 0, github_created_at TEXT NOT NULL DEFAULT '', top_languages TEXT NOT NULL DEFAULT '', top_repo TEXT NOT NULL DEFAULT '', top_repo_stars INTEGER NOT NULL DEFAULT 0, contributions INTEGER NOT NULL DEFAULT 0, contribution_graph TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT (datetime('now')), last_seen TEXT NOT NULL DEFAULT (datetime('now')))");
    await env.DB.exec("CREATE TABLE IF NOT EXISTS bans (github_id INTEGER PRIMARY KEY, ip_hash TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '', banned_at TEXT NOT NULL DEFAULT (datetime('now')), expires_at TEXT, FOREIGN KEY (github_id) REFERENCES profiles(github_id))");
  });

  describe("upsertProfile", () => {
    it("should insert a new profile", async () => {
      await upsertProfile(env.DB, 100, "newuser", "https://avatar/new", "", 0, "", "", "", 0, 0, "");

      const profile = await getProfile(env.DB, 100);
      expect(profile).not.toBeNull();
      expect(profile!.username).toBe("newuser");
      expect(profile!.avatar_url).toBe("https://avatar/new");
      expect(profile!.github_id).toBe(100);
    });

    it("should update an existing profile", async () => {
      await upsertProfile(env.DB, 200, "oldname", "https://avatar/old", "", 0, "", "", "", 0, 0, "");
      await upsertProfile(env.DB, 200, "newname", "https://avatar/updated", "bio", 5, "2020-01-01", "Go,TS", "myrepo", 10, 100, "0011");

      const profile = await getProfile(env.DB, 200);
      expect(profile).not.toBeNull();
      expect(profile!.username).toBe("newname");
      expect(profile!.avatar_url).toBe("https://avatar/updated");
    });
  });

  describe("getProfile", () => {
    it("should return null for non-existent profile", async () => {
      const profile = await getProfile(env.DB, 999999);
      expect(profile).toBeNull();
    });
  });

  describe("isBanned", () => {
    it("should return false for non-banned user", async () => {
      await upsertProfile(env.DB, 300, "freeuser", "", "", 0, "", "", "", 0, 0, "");
      const banned = await isBanned(env.DB, 300);
      expect(banned).toBe(false);
    });

    it("should return true for actively banned user (no expiry)", async () => {
      await upsertProfile(env.DB, 400, "banned-forever", "", "", 0, "", "", "", 0, 0, "");
      await env.DB
        .prepare("INSERT INTO bans (github_id, reason) VALUES (?, ?)")
        .bind(400, "spamming")
        .run();

      const banned = await isBanned(env.DB, 400);
      expect(banned).toBe(true);
    });

    it("should return true for ban with future expiry", async () => {
      await upsertProfile(env.DB, 500, "temp-banned", "", "", 0, "", "", "", 0, 0, "");
      await env.DB
        .prepare("INSERT INTO bans (github_id, reason, expires_at) VALUES (?, ?, datetime('now', '+1 hour'))")
        .bind(500, "temp ban")
        .run();

      const banned = await isBanned(env.DB, 500);
      expect(banned).toBe(true);
    });

    it("should return false for expired ban", async () => {
      await upsertProfile(env.DB, 600, "expired-ban", "", "", 0, "", "", "", 0, 0, "");
      await env.DB
        .prepare("INSERT INTO bans (github_id, reason, expires_at) VALUES (?, ?, datetime('now', '-1 hour'))")
        .bind(600, "expired")
        .run();

      const banned = await isBanned(env.DB, 600);
      expect(banned).toBe(false);
    });
  });
});
