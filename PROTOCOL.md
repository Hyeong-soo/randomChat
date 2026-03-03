# RandomChat Protocol Specification

## REST API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/auth/login?redirect_port={port}` | Redirect to GitHub OAuth. CLI passes local callback port. |
| GET | `/auth/callback?code={code}&state={state}` | GitHub OAuth callback. Exchanges code, creates session, redirects to CLI. |
| GET | `/auth/me` | Returns current user profile. Header: `Authorization: Bearer {token}` |
| POST | `/auth/logout` | Invalidates session. Header: `Authorization: Bearer {token}` |
| GET | `/ws` | WebSocket upgrade → Matchmaker DO. Header: `Authorization: Bearer {token}` |
| GET | `/ws?room={room_id}` | WebSocket upgrade → ChatRoom DO. Header: `Authorization: Bearer {token}` |
| GET | `/health` | Health check. Returns `{ "status": "ok" }` |

## WebSocket Messages

### Client → Server

```json
// Send chat message
{ "type": "message", "text": "hello" }

// Skip current stranger
{ "type": "skip" }

// Typing indicator
{ "type": "typing", "state": "typing" }
{ "type": "typing", "state": "stopped" }
```

### Server → Client

```json
// Match found — disconnect from Matchmaker, reconnect to /ws?token=x&room={room_id}
{ "type": "matched", "room_id": "room-abc123", "stranger": { "username": "octocat", "avatar_url": "https://..." } }

// Incoming message from stranger
{ "type": "message", "from": "stranger", "text": "hey!", "timestamp": "2026-03-03T12:34:56Z" }

// Stranger left the chat
{ "type": "stranger_left" }

// Waiting in queue
{ "type": "waiting" }

// Error
{ "type": "error", "message": "Rate limit exceeded" }

// Typing indicator from stranger
{ "type": "typing", "state": "typing" }
{ "type": "typing", "state": "stopped" }
```

## Connection Flow

```
1. CLI opens WebSocket to /ws with Authorization: Bearer {token} header
2. Server verifies token via KV, connects to Matchmaker DO
3. Matchmaker sends { "type": "waiting" }
4. When 2 users matched: sends { "type": "matched", "room_id": "xxx", ... }
5. CLI disconnects from Matchmaker
6. CLI opens new WebSocket to /ws?room=xxx with Authorization: Bearer {token} header
7. ChatRoom DO relays messages between two users
8. On /skip: ChatRoom sends { "type": "stranger_left" } to other user
9. Both users return to step 1
```

**Auth fallback**: If the `Authorization` header cannot be set (e.g. browser WebSocket),
the token can be sent via `Sec-WebSocket-Protocol: token.{token}`.

## Session Token

- Stored in KV: `session:{token}` → `{ "github_id": 123, "username": "octocat", "avatar_url": "https://..." }`
- TTL: 30 days
- Token format: UUID v4 (via `crypto.randomUUID()`)
