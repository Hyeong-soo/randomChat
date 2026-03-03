import type { Env } from "./types";
import { handleLogin, handleCallback, handleMe, handleLogout, verifySession } from "./auth";
import { isBanned, updateLastSeen } from "./db";

export { Matchmaker } from "./matchmaker";
export { ChatRoom } from "./chatroom";

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    const path = url.pathname;

    try {
      // Health check
      if (path === "/health" && request.method === "GET") {
        return new Response(JSON.stringify({ status: "ok" }), {
          headers: { "Content-Type": "application/json" },
        });
      }

      // Auth routes
      if (path === "/auth/login" && request.method === "GET") {
        return handleLogin(request, env);
      }
      if (path === "/auth/callback" && request.method === "GET") {
        return handleCallback(request, env);
      }
      if (path === "/auth/me" && request.method === "GET") {
        return handleMe(request, env);
      }
      if (path === "/auth/logout" && request.method === "POST") {
        return handleLogout(request, env);
      }

      // WebSocket routes
      if (path === "/ws" && request.method === "GET") {
        return handleWebSocket(request, url, env);
      }

      return new Response(JSON.stringify({ error: "Not found" }), {
        status: 404,
        headers: { "Content-Type": "application/json" },
      });
    } catch {
      return new Response(JSON.stringify({ error: "Internal server error" }), {
        status: 500,
        headers: { "Content-Type": "application/json" },
      });
    }
  },
} satisfies ExportedHandler<Env>;

async function handleWebSocket(
  request: Request,
  url: URL,
  env: Env,
): Promise<Response> {
  const upgradeHeader = request.headers.get("Upgrade");
  if (upgradeHeader !== "websocket") {
    return new Response(JSON.stringify({ error: "Expected WebSocket" }), {
      status: 426,
      headers: { "Content-Type": "application/json" },
    });
  }

  // Read token from Authorization header (preferred) or Sec-WebSocket-Protocol
  let token: string | null = null;
  const authHeader = request.headers.get("Authorization");
  if (authHeader?.startsWith("Bearer ")) {
    token = authHeader.slice(7);
  }
  if (!token) {
    // Fallback: Sec-WebSocket-Protocol "token.<value>"
    const proto = request.headers.get("Sec-WebSocket-Protocol") || "";
    const match = proto.split(",").map((s) => s.trim()).find((s) => s.startsWith("token."));
    if (match) {
      token = match.slice(6);
    }
  }
  if (!token) {
    return new Response(JSON.stringify({ error: "Missing token" }), {
      status: 401,
      headers: { "Content-Type": "application/json" },
    });
  }

  // Verify session
  const session = await verifySession(token, env);
  if (!session) {
    return new Response(JSON.stringify({ error: "Invalid or expired session" }), {
      status: 401,
      headers: { "Content-Type": "application/json" },
    });
  }

  // Check ban
  const banned = await isBanned(env.DB, session.github_id);
  if (banned) {
    return new Response(JSON.stringify({ error: "Account is banned" }), {
      status: 403,
      headers: { "Content-Type": "application/json" },
    });
  }

  // Update last seen
  await updateLastSeen(env.DB, session.github_id);

  // Validate github_id
  const githubId = session.github_id;
  if (isNaN(githubId) || githubId <= 0) {
    return new Response(JSON.stringify({ error: "Invalid GitHub ID" }), {
      status: 400,
      headers: { "Content-Type": "application/json" },
    });
  }

  // Forward session info via headers to DO
  const headers = new Headers(request.headers);
  headers.set("X-GitHub-ID", String(githubId));
  headers.set("X-Username", session.username);
  headers.set("X-Avatar-URL", session.avatar_url);
  headers.set("X-Bio", session.bio || "");
  headers.set("X-Public-Repos", String(session.public_repos || 0));
  headers.set("X-GitHub-Created-At", session.github_created_at || "");
  headers.set("X-Top-Languages", session.top_languages || "");
  headers.set("X-Top-Repo", session.top_repo || "");
  headers.set("X-Top-Repo-Stars", String(session.top_repo_stars || 0));
  headers.set("X-Contributions", String(session.contributions || 0));
  headers.set("X-Contribution-Graph", session.contribution_graph || "");
  const doRequest = new Request(request.url, {
    method: request.method,
    headers,
  });

  const roomId = url.searchParams.get("room");

  if (roomId) {
    // Route to specific ChatRoom DO
    const id = env.CHATROOM.idFromName(roomId);
    const stub = env.CHATROOM.get(id);
    return stub.fetch(doRequest);
  }

  // Route to global Matchmaker DO
  const id = env.MATCHMAKER.idFromName("global");
  const stub = env.MATCHMAKER.get(id);
  return stub.fetch(doRequest);
}
