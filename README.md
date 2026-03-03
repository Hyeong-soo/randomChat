# RandomChat

Anonymous terminal chat with random strangers. Authenticate with GitHub, get matched with someone, and start chatting — all from your terminal.

<!-- TODO: add screenshot/demo GIF -->

## Quick Start

```bash
# One-line install (macOS / Linux)
curl -sSL https://raw.githubusercontent.com/Hyeong-soo/randomChat/main/install.sh | sh

# Then just run:
randomchat
```

A browser window opens for GitHub login on first run. After that, you're dropped into a matchmaking queue and paired with a stranger.

### Other install methods

```bash
# Go install
go install github.com/Hyeong-soo/randomChat/cmd/randomchat@latest

# Or download a binary from GitHub Releases
# https://github.com/Hyeong-soo/randomChat/releases
```

## Features

- **Random matching** — paired with a random online user via WebSocket
- **GitHub profile card** — see your stranger's avatar, bio, top repo, languages, and contribution graph
- **Color avatars** — half-block Unicode rendering with 24-bit true color
- **Contribution heatmap** — 12-week activity graph rendered in the terminal
- **Typing indicators** — see when the other person is typing
- **Chat commands** — `/skip`, `/quit`, `/help`, `/status`
- **Scroll** — PgUp / PgDown to scroll through chat history

## Keybindings

| Key | Action |
|-----|--------|
| `Enter` | Send message / retry connection |
| `Esc` | Skip to next stranger |
| `PgUp` / `PgDown` | Scroll chat history |
| `Ctrl+C` | Quit |

## Self-Hosting

RandomChat has two parts: a **Cloudflare Workers** backend and a **Go CLI** client.

### 1. Create a GitHub OAuth App

1. Go to [GitHub Developer Settings](https://github.com/settings/developers) → **OAuth Apps** → **New OAuth App**
2. Set **Authorization callback URL** to `https://randomchat.YOUR_SUBDOMAIN.workers.dev/auth/callback`
3. Note the **Client ID** and generate a **Client Secret**

### 2. Deploy the Worker

```bash
# Clone and install dependencies
git clone https://github.com/Hyeong-soo/randomChat.git
cd randomchat
npm install

# Configure
cp wrangler.toml.example wrangler.toml
# Edit wrangler.toml: fill in GITHUB_CLIENT_ID, CALLBACK_URL, database_id, KV id

# Create D1 database and KV namespace
npx wrangler d1 create randomchat-db
npx wrangler kv namespace create SESSIONS

# Apply schema
npx wrangler d1 execute randomchat-db --file=schema.sql --remote

# Set secret
npx wrangler secret put GITHUB_CLIENT_SECRET

# Deploy
npx wrangler deploy
```

### 3. Point the CLI to your server

```bash
export RANDOMCHAT_SERVER="https://randomchat.YOUR_SUBDOMAIN.workers.dev"
randomchat
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `RANDOMCHAT_SERVER` | `https://randomchat.hyeongsoo.workers.dev` | Backend server URL |
| `RANDOMCHAT_CONFIG` | `~/.config/randomchat` | Config/session directory |

## Architecture

```
┌─────────────┐         WebSocket          ┌──────────────────────┐
│  Go CLI     │◄──────────────────────────►│  Cloudflare Workers  │
│  (Bubble Tea│         HTTPS              │                      │
│   TUI)      │◄──────────────────────────►│  ├─ Matchmaker (DO)  │
└─────────────┘                            │  ├─ ChatRoom (DO)    │
                                           │  ├─ D1 (profiles)    │
                                           │  └─ KV (sessions)    │
                                           └──────────────────────┘
```

- **Matchmaker** — Durable Object that queues users and pairs them
- **ChatRoom** — Durable Object that relays messages between two matched users
- **D1** — SQLite database for user profiles, bans, reports
- **KV** — Session token storage with TTL

## Development

```bash
# Build CLI
make build

# Run all tests (Go + Cloudflare Workers)
make test

# Run worker locally
npm run dev

# Lint
make lint
```

## Protocol

See [PROTOCOL.md](PROTOCOL.md) for the full WebSocket message specification.

## License

[MIT](LICENSE)
