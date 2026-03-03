import { describe, it, expect } from "vitest";
import { env } from "cloudflare:test";
import type { ServerMessage } from "../types";

function createChatRoomStub() {
  const id = env.CHATROOM.idFromName("room-" + crypto.randomUUID());
  return env.CHATROOM.get(id);
}

function makeUpgradeRequest(
  githubId: number,
  username: string,
  avatarUrl = "",
): Request {
  return new Request("http://fake/ws", {
    headers: {
      Upgrade: "websocket",
      "X-GitHub-ID": String(githubId),
      "X-Username": username,
      "X-Avatar-URL": avatarUrl,
    },
  });
}

function collectMessages(ws: WebSocket): ServerMessage[] {
  const msgs: ServerMessage[] = [];
  ws.addEventListener("message", (e) => {
    msgs.push(JSON.parse(e.data as string) as ServerMessage);
  });
  return msgs;
}

function safeClose(ws: WebSocket): void {
  try {
    ws.close(1000, "test-cleanup");
  } catch {
    // already closed
  }
}

describe("ChatRoom", () => {
  it("should relay messages between two users", async () => {
    const stub = createChatRoomStub();

    const res1 = await stub.fetch(makeUpgradeRequest(1, "alice"));
    const ws1 = res1.webSocket!;
    ws1.accept();
    const msgs1 = collectMessages(ws1);

    const res2 = await stub.fetch(makeUpgradeRequest(2, "bob"));
    const ws2 = res2.webSocket!;
    ws2.accept();
    const msgs2 = collectMessages(ws2);

    // Alice sends a message
    ws1.send(JSON.stringify({ type: "message", text: "hello from alice" }));
    await new Promise((r) => setTimeout(r, 100));

    // Bob should receive it
    const relayed = msgs2.find(
      (m) => m.type === "message" && m.from === "stranger",
    );
    expect(relayed).toBeDefined();
    if (relayed?.type === "message") {
      expect(relayed.text).toBe("hello from alice");
      expect(relayed.timestamp).toBeDefined();
    }

    // Alice should NOT receive her own message
    const selfMsg = msgs1.find(
      (m) => m.type === "message" && m.from === "stranger",
    );
    expect(selfMsg).toBeUndefined();

    safeClose(ws1);
    safeClose(ws2);
    await new Promise((r) => setTimeout(r, 50));
  });

  it("should notify peer on skip", async () => {
    const stub = createChatRoomStub();

    const res1 = await stub.fetch(makeUpgradeRequest(1, "alice"));
    const ws1 = res1.webSocket!;
    ws1.accept();

    const res2 = await stub.fetch(makeUpgradeRequest(2, "bob"));
    const ws2 = res2.webSocket!;
    ws2.accept();
    const msgs2 = collectMessages(ws2);

    // Alice skips
    ws1.send(JSON.stringify({ type: "skip" }));
    await new Promise((r) => setTimeout(r, 100));

    // Bob should get stranger_left
    const left = msgs2.find((m) => m.type === "stranger_left");
    expect(left).toBeDefined();

    // Both websockets are closed by skip handler
    await new Promise((r) => setTimeout(r, 50));
  });

  it("should reject a 3rd connection (room full)", async () => {
    const stub = createChatRoomStub();

    const res1 = await stub.fetch(makeUpgradeRequest(1, "alice"));
    expect(res1.status).toBe(101);
    const ws1 = res1.webSocket!;
    ws1.accept();

    const res2 = await stub.fetch(makeUpgradeRequest(2, "bob"));
    expect(res2.status).toBe(101);
    const ws2 = res2.webSocket!;
    ws2.accept();

    const res3 = await stub.fetch(makeUpgradeRequest(3, "charlie"));
    expect(res3.status).toBe(403);

    safeClose(ws1);
    safeClose(ws2);
    await new Promise((r) => setTimeout(r, 50));
  });

  it("should reject messages exceeding 2000 characters", async () => {
    const stub = createChatRoomStub();

    const res1 = await stub.fetch(makeUpgradeRequest(1, "alice"));
    const ws1 = res1.webSocket!;
    ws1.accept();
    const msgs1 = collectMessages(ws1);

    const res2 = await stub.fetch(makeUpgradeRequest(2, "bob"));
    const ws2 = res2.webSocket!;
    ws2.accept();
    const msgs2 = collectMessages(ws2);

    // Alice sends an oversized message
    const longText = "x".repeat(2001);
    ws1.send(JSON.stringify({ type: "message", text: longText }));
    await new Promise((r) => setTimeout(r, 100));

    // Alice should get an error
    const err = msgs1.find((m) => m.type === "error");
    expect(err).toBeDefined();
    if (err?.type === "error") {
      expect(err.message).toContain("2000");
    }

    // Bob should NOT receive the oversized message
    const relayed = msgs2.find((m) => m.type === "message");
    expect(relayed).toBeUndefined();

    safeClose(ws1);
    safeClose(ws2);
    await new Promise((r) => setTimeout(r, 50));
  });

  it("should accept messages exactly at 2000 characters", async () => {
    const stub = createChatRoomStub();

    const res1 = await stub.fetch(makeUpgradeRequest(1, "alice"));
    const ws1 = res1.webSocket!;
    ws1.accept();
    const msgs1 = collectMessages(ws1);

    const res2 = await stub.fetch(makeUpgradeRequest(2, "bob"));
    const ws2 = res2.webSocket!;
    ws2.accept();
    const msgs2 = collectMessages(ws2);

    const exactText = "y".repeat(2000);
    ws1.send(JSON.stringify({ type: "message", text: exactText }));
    await new Promise((r) => setTimeout(r, 100));

    // No error for Alice
    const err = msgs1.find((m) => m.type === "error");
    expect(err).toBeUndefined();

    // Bob should receive it
    const relayed = msgs2.find(
      (m) => m.type === "message" && m.from === "stranger",
    );
    expect(relayed).toBeDefined();

    safeClose(ws1);
    safeClose(ws2);
    await new Promise((r) => setTimeout(r, 50));
  });

  it("should relay valid typing states", async () => {
    const stub = createChatRoomStub();

    const res1 = await stub.fetch(makeUpgradeRequest(1, "alice"));
    const ws1 = res1.webSocket!;
    ws1.accept();

    const res2 = await stub.fetch(makeUpgradeRequest(2, "bob"));
    const ws2 = res2.webSocket!;
    ws2.accept();
    const msgs2 = collectMessages(ws2);

    ws1.send(JSON.stringify({ type: "typing", state: "typing" }));
    await new Promise((r) => setTimeout(r, 100));

    const typing = msgs2.find(
      (m) => m.type === "typing" && m.state === "typing",
    );
    expect(typing).toBeDefined();

    safeClose(ws1);
    safeClose(ws2);
    await new Promise((r) => setTimeout(r, 50));
  });

  it("should return error for invalid JSON", async () => {
    const stub = createChatRoomStub();

    const res1 = await stub.fetch(makeUpgradeRequest(1, "alice"));
    const ws1 = res1.webSocket!;
    ws1.accept();
    const msgs1 = collectMessages(ws1);

    ws1.send("not json");
    await new Promise((r) => setTimeout(r, 100));

    const err = msgs1.find((m) => m.type === "error");
    expect(err).toBeDefined();

    safeClose(ws1);
    await new Promise((r) => setTimeout(r, 50));
  });

  it("should notify peer on disconnect", async () => {
    const stub = createChatRoomStub();

    const res1 = await stub.fetch(makeUpgradeRequest(1, "alice"));
    const ws1 = res1.webSocket!;
    ws1.accept();

    const res2 = await stub.fetch(makeUpgradeRequest(2, "bob"));
    const ws2 = res2.webSocket!;
    ws2.accept();
    const msgs2 = collectMessages(ws2);

    // Alice disconnects
    ws1.close(1000, "bye");
    await new Promise((r) => setTimeout(r, 100));

    const left = msgs2.find((m) => m.type === "stranger_left");
    expect(left).toBeDefined();

    // ws2 gets closed by the close handler
    await new Promise((r) => setTimeout(r, 50));
  });
});
