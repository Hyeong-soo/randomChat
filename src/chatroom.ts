import type { ClientMessage, ServerMessage } from "./types";
import { DurableObject } from "cloudflare:workers";

// Strip ANSI escape sequences and control characters (except newline/tab)
function sanitizeText(text: string): string {
  // eslint-disable-next-line no-control-regex
  return text.replace(/\x1b\[[0-9;]*[a-zA-Z]|\x1b\].*?(?:\x07|\x1b\\)|\x1b[^[\]]/g, "")
    .replace(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/g, "");
}

const RATE_LIMIT_WINDOW_MS = 5_000;
const RATE_LIMIT_MAX = 10;

export class ChatRoom extends DurableObject {
  private msgTimestamps = new Map<WebSocket, number[]>();
  async fetch(request: Request): Promise<Response> {
    const upgradeHeader = request.headers.get("Upgrade");
    if (upgradeHeader !== "websocket") {
      return new Response("Expected WebSocket", { status: 426 });
    }

    // Check current connections — max 2 per room
    const sockets = this.ctx.getWebSockets();
    if (sockets.length >= 2) {
      return new Response("Room is full", { status: 403 });
    }

    const githubId = request.headers.get("X-GitHub-ID");
    const username = request.headers.get("X-Username");
    const avatarUrl = request.headers.get("X-Avatar-URL");

    if (!githubId || !username) {
      return new Response("Missing session info", { status: 400 });
    }

    // Room authorization: only the first 2 unique github_ids are allowed
    const allowedUsers = (await this.ctx.storage.get<number[]>("allowed_users")) || [];
    const numericId = parseInt(githubId, 10);
    if (allowedUsers.length >= 2 && !allowedUsers.includes(numericId)) {
      return new Response("Not authorized for this room", { status: 403 });
    }
    if (!allowedUsers.includes(numericId)) {
      allowedUsers.push(numericId);
      await this.ctx.storage.put("allowed_users", allowedUsers);
    }

    const pair = new WebSocketPair();
    const [client, server] = [pair[0], pair[1]];

    // Use tags to store session info on the WebSocket
    this.ctx.acceptWebSocket(server, [
      `id:${githubId}`,
      `user:${username}`,
      `avatar:${avatarUrl || ""}`,
    ]);

    return new Response(null, { status: 101, webSocket: client });
  }

  webSocketMessage(
    ws: WebSocket,
    message: string | ArrayBuffer,
  ): void {
    let parsed: ClientMessage;
    try {
      const text = typeof message === "string" ? message : new TextDecoder().decode(message);
      parsed = JSON.parse(text) as ClientMessage;
    } catch {
      const err: ServerMessage = { type: "error", message: "Invalid JSON" };
      ws.send(JSON.stringify(err));
      return;
    }

    const peer = this.getPeer(ws);
    if (!peer) {
      // No peer connected yet — just ignore
      return;
    }

    switch (parsed.type) {
      case "message": {
        if (parsed.text.length > 2000) {
          const err: ServerMessage = {
            type: "error",
            message: "Message too long (max 2000 characters)",
          };
          ws.send(JSON.stringify(err));
          return;
        }
        // Rate limiting
        const now = Date.now();
        const timestamps = this.msgTimestamps.get(ws) || [];
        const recent = timestamps.filter((t) => now - t < RATE_LIMIT_WINDOW_MS);
        if (recent.length >= RATE_LIMIT_MAX) {
          const err: ServerMessage = {
            type: "error",
            message: "Too many messages, slow down",
          };
          ws.send(JSON.stringify(err));
          return;
        }
        recent.push(now);
        this.msgTimestamps.set(ws, recent);
        const relay: ServerMessage = {
          type: "message",
          from: "stranger",
          text: sanitizeText(parsed.text),
          timestamp: new Date().toISOString(),
        };
        peer.send(JSON.stringify(relay));
        break;
      }

      case "skip": {
        const left: ServerMessage = { type: "stranger_left" };
        try {
          peer.send(JSON.stringify(left));
          peer.close(1000, "skipped");
        } catch {
          // peer already closed
        }
        try {
          ws.close(1000, "skipped");
        } catch {
          // ws already closed
        }
        break;
      }

      case "typing": {
        if (parsed.state !== "typing" && parsed.state !== "stopped") {
          return;
        }
        const typing: ServerMessage = {
          type: "typing",
          state: parsed.state,
        };
        peer.send(JSON.stringify(typing));
        break;
      }

      default: {
        const err: ServerMessage = {
          type: "error",
          message: "Unknown message type",
        };
        ws.send(JSON.stringify(err));
      }
    }
  }

  webSocketClose(
    ws: WebSocket,
    _code: number,
    _reason: string,
    _wasClean: boolean,
  ): void {
    const peer = this.getPeer(ws);
    if (peer) {
      const left: ServerMessage = { type: "stranger_left" };
      try {
        peer.send(JSON.stringify(left));
        peer.close(1000, "peer_disconnected");
      } catch {
        // peer already closed
      }
    }
  }

  webSocketError(ws: WebSocket): void {
    const peer = this.getPeer(ws);
    if (peer) {
      const left: ServerMessage = { type: "stranger_left" };
      try {
        peer.send(JSON.stringify(left));
        peer.close(1000, "peer_error");
      } catch {
        // peer already closed
      }
    }
  }

  private getPeer(ws: WebSocket): WebSocket | null {
    const sockets = this.ctx.getWebSockets();
    for (const s of sockets) {
      if (s !== ws) return s;
    }
    return null;
  }
}
