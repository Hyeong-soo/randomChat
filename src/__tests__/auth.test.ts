import { describe, it, expect, beforeAll, beforeEach, afterEach } from "vitest";
import { env, SELF, fetchMock } from "cloudflare:test";

describe("Auth", () => {
  beforeAll(async () => {
    await env.DB.exec("CREATE TABLE IF NOT EXISTS profiles (github_id INTEGER PRIMARY KEY, username TEXT NOT NULL, avatar_url TEXT NOT NULL DEFAULT '', bio TEXT NOT NULL DEFAULT '', public_repos INTEGER NOT NULL DEFAULT 0, github_created_at TEXT NOT NULL DEFAULT '', top_languages TEXT NOT NULL DEFAULT '', top_repo TEXT NOT NULL DEFAULT '', top_repo_stars INTEGER NOT NULL DEFAULT 0, contributions INTEGER NOT NULL DEFAULT 0, contribution_graph TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT (datetime('now')), last_seen TEXT NOT NULL DEFAULT (datetime('now')))");
  });

  beforeEach(() => {
    fetchMock.activate();
    fetchMock.disableNetConnect();
  });

  afterEach(() => {
    fetchMock.deactivate();
  });

  describe("handleLogin", () => {
    it("should return 400 if redirect_port is missing", async () => {
      const res = await SELF.fetch("https://fake/auth/login");
      expect(res.status).toBe(400);
      const body = await res.json() as { error: string };
      expect(body.error).toContain("redirect_port");
    });

    it("should redirect to GitHub with state containing redirect_port", async () => {
      const res = await SELF.fetch(
        "https://fake/auth/login?redirect_port=9876",
        { redirect: "manual" },
      );
      expect(res.status).toBe(302);

      const location = res.headers.get("Location")!;
      expect(location).toContain("github.com/login/oauth/authorize");
      expect(location).toContain("client_id=");

      // Extract state from URL
      const url = new URL(location);
      const state = url.searchParams.get("state")!;
      expect(state).toContain(":9876");

      // Verify state was stored in KV
      const stateRandom = state.split(":")[0];
      const stored = await env.SESSIONS.get(`oauth_state:${stateRandom}`);
      expect(stored).toBe("1");
    });
  });

  describe("handleCallback", () => {
    it("should return 400 if code or state is missing", async () => {
      const res = await SELF.fetch("https://fake/auth/callback");
      expect(res.status).toBe(400);
    });

    it("should return 400 for invalid state format", async () => {
      const res = await SELF.fetch(
        "https://fake/auth/callback?code=abc&state=no-colon",
      );
      expect(res.status).toBe(400);
    });

    it("should return 400 for expired/invalid CSRF state", async () => {
      const res = await SELF.fetch(
        "https://fake/auth/callback?code=abc&state=nonexistent:8080",
      );
      expect(res.status).toBe(400);
      const body = await res.json() as { error: string };
      expect(body.error).toContain("OAuth state");
    });

    it("should complete OAuth flow with valid state", async () => {
      // Store a valid state in KV
      const stateRandom = "abcdef1234567890abcdef1234567890";
      await env.SESSIONS.put(`oauth_state:${stateRandom}`, "1", {
        expirationTtl: 300,
      });

      // Mock GitHub token exchange
      fetchMock
        .get("https://github.com")
        .intercept({ path: "/login/oauth/access_token", method: "POST" })
        .reply(200, JSON.stringify({ access_token: "gho_test_token" }), {
          headers: { "Content-Type": "application/json" },
        });

      // Mock GitHub user API
      const ghApi = fetchMock.get("https://api.github.com");
      ghApi
        .intercept({ path: "/user" })
        .reply(
          200,
          JSON.stringify({
            id: 12345,
            login: "testuser",
            avatar_url: "https://avatar/test",
            bio: "test bio",
            public_repos: 5,
            created_at: "2020-01-01T00:00:00Z",
          }),
          { headers: { "Content-Type": "application/json" } },
        );

      // Mock repos API
      ghApi
        .intercept({ path: /\/user\/repos/ })
        .reply(200, JSON.stringify([]), {
          headers: { "Content-Type": "application/json" },
        });

      // Mock GraphQL API (contributions)
      ghApi
        .intercept({ path: "/graphql", method: "POST" })
        .reply(200, JSON.stringify({ data: { user: { contributionsCollection: { contributionCalendar: { totalContributions: 0, weeks: [] } } } } }), {
          headers: { "Content-Type": "application/json" },
        });

      // Mock token revoke
      ghApi
        .intercept({ path: /\/applications\//, method: "DELETE" })
        .reply(204);

      const res = await SELF.fetch(
        `https://fake/auth/callback?code=valid_code&state=${stateRandom}:8080`,
        { redirect: "manual" },
      );

      expect(res.status).toBe(302);
      const location = res.headers.get("Location")!;
      expect(location).toContain("localhost:8080/callback?token=");

      // State should be consumed (deleted)
      const consumed = await env.SESSIONS.get(
        `oauth_state:${stateRandom}`,
      );
      expect(consumed).toBeNull();

      // Session should be created
      const token = new URL(location).searchParams.get("token")!;
      const sessionRaw = await env.SESSIONS.get(`session:${token}`);
      expect(sessionRaw).not.toBeNull();
      const session = JSON.parse(sessionRaw!) as { github_id: number; username: string };
      expect(session.github_id).toBe(12345);
      expect(session.username).toBe("testuser");
    });
  });

  describe("handleMe", () => {
    it("should return 401 without token", async () => {
      const res = await SELF.fetch("https://fake/auth/me");
      expect(res.status).toBe(401);
    });

    it("should return session data with valid token", async () => {
      const token = crypto.randomUUID();
      await env.SESSIONS.put(
        `session:${token}`,
        JSON.stringify({
          github_id: 1,
          username: "testuser",
          avatar_url: "https://avatar/test",
        }),
      );

      const res = await SELF.fetch("https://fake/auth/me", {
        headers: { Authorization: `Bearer ${token}` },
      });
      expect(res.status).toBe(200);
      const body = await res.json() as { username: string };
      expect(body.username).toBe("testuser");
    });
  });

  describe("handleLogout", () => {
    it("should delete session on logout", async () => {
      const token = crypto.randomUUID();
      await env.SESSIONS.put(
        `session:${token}`,
        JSON.stringify({
          github_id: 1,
          username: "testuser",
          avatar_url: "",
        }),
      );

      const res = await SELF.fetch("https://fake/auth/logout", {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      });
      expect(res.status).toBe(200);

      const deleted = await env.SESSIONS.get(`session:${token}`);
      expect(deleted).toBeNull();
    });

    it("should return 401 without token", async () => {
      const res = await SELF.fetch("https://fake/auth/logout", {
        method: "POST",
      });
      expect(res.status).toBe(401);
    });
  });
});
