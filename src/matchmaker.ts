import type { ServerMessage, StrangerProfile } from "./types";
import { DurableObject } from "cloudflare:workers";

const STALE_TIMEOUT_MS = 120_000; // 120 seconds
const ALARM_INTERVAL_MS = 30_000; // 30 seconds

interface QueueEntry {
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
  joined_at: number;
}

export class Matchmaker extends DurableObject {
  async fetch(request: Request): Promise<Response> {
    const upgradeHeader = request.headers.get("Upgrade");
    if (upgradeHeader !== "websocket") {
      return new Response("Expected WebSocket", { status: 426 });
    }

    const githubId = request.headers.get("X-GitHub-ID");
    const username = request.headers.get("X-Username");
    const avatarUrl = request.headers.get("X-Avatar-URL");
    const bio = request.headers.get("X-Bio") || "";
    const publicRepos = parseInt(request.headers.get("X-Public-Repos") || "0", 10);
    const githubCreatedAt = request.headers.get("X-GitHub-Created-At") || "";
    const topLanguages = request.headers.get("X-Top-Languages") || "";
    const topRepo = request.headers.get("X-Top-Repo") || "";
    const topRepoStars = parseInt(request.headers.get("X-Top-Repo-Stars") || "0", 10);
    const contributions = parseInt(request.headers.get("X-Contributions") || "0", 10);
    const contributionGraph = request.headers.get("X-Contribution-Graph") || "";

    if (!githubId || !username) {
      return new Response("Missing session info", { status: 400 });
    }

    // Rate limit: prevent duplicate connections from same user
    const numericGithubId = parseInt(githubId, 10);
    for (const ws of this.ctx.getWebSockets()) {
      const tags = this.ctx.getTags(ws);
      if (tags.length === 0) continue;
      const entry = await this.ctx.storage.get<QueueEntry>(`queue:${tags[0]}`);
      if (entry && entry.github_id === numericGithubId) {
        return new Response("Already in queue", { status: 429 });
      }
    }

    const pair = new WebSocketPair();
    const [client, server] = [pair[0], pair[1]];

    const connId = crypto.randomUUID();
    const entry: QueueEntry = {
      github_id: parseInt(githubId, 10),
      username,
      avatar_url: avatarUrl || "",
      bio,
      public_repos: publicRepos,
      github_created_at: githubCreatedAt,
      top_languages: topLanguages,
      top_repo: topRepo,
      top_repo_stars: topRepoStars,
      contributions,
      contribution_graph: contributionGraph,
      joined_at: Date.now(),
    };

    // Tag with short ID, store full entry in DO storage
    this.ctx.acceptWebSocket(server, [connId]);
    await this.ctx.storage.put(`queue:${connId}`, entry);

    // Send waiting status
    const waitMsg: ServerMessage = { type: "waiting" };
    server.send(JSON.stringify(waitMsg));

    // Attempt to match
    await this.tryMatch();

    // Schedule alarm for stale connection cleanup
    const currentAlarm = await this.ctx.storage.getAlarm();
    if (currentAlarm === null) {
      await this.ctx.storage.setAlarm(Date.now() + ALARM_INTERVAL_MS);
    }

    return new Response(null, { status: 101, webSocket: client });
  }

  private async getQueue(): Promise<Map<WebSocket, QueueEntry>> {
    const queue = new Map<WebSocket, QueueEntry>();
    for (const ws of this.ctx.getWebSockets()) {
      const tags = this.ctx.getTags(ws);
      if (tags.length === 0) continue;
      const connId = tags[0];
      const entry = await this.ctx.storage.get<QueueEntry>(`queue:${connId}`);
      if (entry) {
        queue.set(ws, entry);
      }
    }
    return queue;
  }

  private async removeFromQueue(ws: WebSocket): Promise<void> {
    const tags = this.ctx.getTags(ws);
    if (tags.length > 0) {
      await this.ctx.storage.delete(`queue:${tags[0]}`);
    }
  }

  private async tryMatch(): Promise<void> {
    const queue = await this.getQueue();
    const entries = Array.from(queue.entries());

    for (let i = 0; i < entries.length; i++) {
      for (let j = i + 1; j < entries.length; j++) {
        const [ws1, user1] = entries[i];
        const [ws2, user2] = entries[j];

        if (user1.github_id === user2.github_id) {
          continue;
        }

        // Remove both from queue
        await this.removeFromQueue(ws1);
        await this.removeFromQueue(ws2);

        const roomId = crypto.randomUUID();

        const toProfile = (u: QueueEntry): StrangerProfile => ({
          username: u.username,
          avatar_url: u.avatar_url,
          bio: u.bio,
          public_repos: u.public_repos,
          github_created_at: u.github_created_at,
          top_languages: u.top_languages,
          top_repo: u.top_repo,
          top_repo_stars: u.top_repo_stars,
          contributions: u.contributions,
          contribution_graph: u.contribution_graph,
        });

        const msg1: ServerMessage = {
          type: "matched",
          room_id: roomId,
          stranger: toProfile(user2),
        };
        const msg2: ServerMessage = {
          type: "matched",
          room_id: roomId,
          stranger: toProfile(user1),
        };

        try {
          ws1.send(JSON.stringify(msg1));
          ws1.close(1000, "matched");
        } catch {
          // ws1 already closed
        }

        try {
          ws2.send(JSON.stringify(msg2));
          ws2.close(1000, "matched");
        } catch {
          // ws2 already closed
        }

        // Restart matching with remaining connections
        return this.tryMatch();
      }
    }
  }

  async webSocketClose(
    ws: WebSocket,
    _code: number,
    _reason: string,
    _wasClean: boolean,
  ): Promise<void> {
    await this.removeFromQueue(ws);
  }

  async webSocketError(ws: WebSocket): Promise<void> {
    await this.removeFromQueue(ws);
  }

  webSocketMessage(
    _ws: WebSocket,
    _message: string | ArrayBuffer,
  ): void {
    // Matchmaker doesn't expect any client messages after connect.
  }

  async alarm(): Promise<void> {
    const now = Date.now();
    const queue = await this.getQueue();

    for (const [ws, entry] of queue) {
      if (now - entry.joined_at >= STALE_TIMEOUT_MS) {
        try {
          const waitMsg: ServerMessage = { type: "waiting" };
          ws.send(JSON.stringify(waitMsg));
          ws.close(1000, "stale");
        } catch {
          // already closed
        }
        await this.removeFromQueue(ws);
      }
    }

    if (this.ctx.getWebSockets().length > 0) {
      const current = await this.ctx.storage.getAlarm();
      if (current === null) {
        await this.ctx.storage.setAlarm(Date.now() + ALARM_INTERVAL_MS);
      }
    }
  }
}
