import {
  describe,
  it,
  expect,
  beforeEach,
} from "vitest";
import { env, runInDurableObject, runDurableObjectAlarm } from "cloudflare:test";
import type { ServerMessage } from "../types";

function createMatchmakerStub() {
  const id = env.MATCHMAKER.idFromName("test-" + crypto.randomUUID());
  return env.MATCHMAKER.get(id);
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

describe("Matchmaker", () => {
  it("should send waiting status on connect", async () => {
    const stub = createMatchmakerStub();
    const res = await stub.fetch(makeUpgradeRequest(1, "alice"));
    expect(res.status).toBe(101);

    const ws = res.webSocket!;
    ws.accept();
    const msgs = collectMessages(ws);

    // The "waiting" message is sent immediately upon connection
    // Give it a moment to process
    await new Promise((r) => setTimeout(r, 50));

    expect(msgs.length).toBeGreaterThanOrEqual(1);
    expect(msgs[0]).toEqual({ type: "waiting" });

    ws.close();
  });

  it("should match two different users", async () => {
    const stub = createMatchmakerStub();

    const res1 = await stub.fetch(makeUpgradeRequest(1, "alice", "https://avatar/alice"));
    const ws1 = res1.webSocket!;
    ws1.accept();
    const msgs1 = collectMessages(ws1);

    const res2 = await stub.fetch(makeUpgradeRequest(2, "bob", "https://avatar/bob"));
    const ws2 = res2.webSocket!;
    ws2.accept();
    const msgs2 = collectMessages(ws2);

    await new Promise((r) => setTimeout(r, 100));

    // Both should get a matched message
    const matched1 = msgs1.find((m) => m.type === "matched");
    const matched2 = msgs2.find((m) => m.type === "matched");

    expect(matched1).toBeDefined();
    expect(matched2).toBeDefined();

    if (matched1?.type === "matched" && matched2?.type === "matched") {
      // They should share the same room_id
      expect(matched1.room_id).toBe(matched2.room_id);
      // Stranger profiles should match the opposite user
      expect(matched1.stranger.username).toBe("bob");
      expect(matched2.stranger.username).toBe("alice");
    }
  });

  it("should prevent duplicate connection (same github_id)", async () => {
    const stub = createMatchmakerStub();

    const res1 = await stub.fetch(makeUpgradeRequest(42, "alice"));
    expect(res1.status).toBe(101);
    const ws1 = res1.webSocket!;
    ws1.accept();

    // Second connection with same github_id should be rejected (429)
    const res2 = await stub.fetch(makeUpgradeRequest(42, "alice-alt"));
    expect(res2.status).toBe(429);

    ws1.close();
  });

  it("should match 2 of 3 users and leave 1 waiting", async () => {
    const stub = createMatchmakerStub();

    const res1 = await stub.fetch(makeUpgradeRequest(1, "alice"));
    const ws1 = res1.webSocket!;
    ws1.accept();
    const msgs1 = collectMessages(ws1);

    const res2 = await stub.fetch(makeUpgradeRequest(2, "bob"));
    const ws2 = res2.webSocket!;
    ws2.accept();
    const msgs2 = collectMessages(ws2);

    const res3 = await stub.fetch(makeUpgradeRequest(3, "charlie"));
    const ws3 = res3.webSocket!;
    ws3.accept();
    const msgs3 = collectMessages(ws3);

    await new Promise((r) => setTimeout(r, 100));

    // Count how many got matched
    const allMatched = [msgs1, msgs2, msgs3].filter((msgs) =>
      msgs.some((m) => m.type === "matched"),
    );
    expect(allMatched.length).toBe(2);

    // The remaining one should only have waiting
    const notMatched = [msgs1, msgs2, msgs3].filter(
      (msgs) => !msgs.some((m) => m.type === "matched"),
    );
    expect(notMatched.length).toBe(1);
    expect(notMatched[0].some((m) => m.type === "waiting")).toBe(true);

    ws3.close();
  });

  it("should reject non-websocket requests", async () => {
    const stub = createMatchmakerStub();
    const res = await stub.fetch(
      new Request("http://fake/ws", {
        headers: { "X-GitHub-ID": "1", "X-Username": "alice" },
      }),
    );
    expect(res.status).toBe(426);
  });

  it("should reject requests missing session info", async () => {
    const stub = createMatchmakerStub();
    const res = await stub.fetch(
      new Request("http://fake/ws", {
        headers: { Upgrade: "websocket" },
      }),
    );
    expect(res.status).toBe(400);
  });

  it("should clean up stale connections via alarm", async () => {
    const stub = createMatchmakerStub();

    const res = await stub.fetch(makeUpgradeRequest(1, "alice"));
    const ws = res.webSocket!;
    ws.accept();
    collectMessages(ws);

    await new Promise((r) => setTimeout(r, 50));

    // Set all queue entries' joined_at to far in the past via DO storage
    await runInDurableObject(stub, async (instance) => {
      const storage = instance.ctx.storage;
      const entries = await storage.list({ prefix: "queue:" });
      for (const [key, value] of entries) {
        const entry = value as { joined_at: number };
        entry.joined_at = Date.now() - 200_000; // 200 seconds ago
        await storage.put(key, entry);
      }
    });

    // Run alarm
    const alarmRan = await runDurableObjectAlarm(stub);
    expect(alarmRan).toBe(true);

    // Verify queue storage is now empty
    const queueSize = await runInDurableObject(stub, async (instance) => {
      const entries = await instance.ctx.storage.list({ prefix: "queue:" });
      return entries.size;
    });
    expect(queueSize).toBe(0);
  });
});
