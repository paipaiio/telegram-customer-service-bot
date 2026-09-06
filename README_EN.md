# ForumDesk · Telegram Customer Service Bot

[中文说明 / Chinese README](README.md) | [图文文档 / Visual Docs](README.html)

A self-hosted Telegram customer service bot written in Go. Each user who DMs your bot gets a dedicated forum topic in a private support supergroup. Agents reply inside the topic, and the bot relays messages both ways — with quote mapping, edit sync, two-side deletion, and content protection.

## Features

- **One topic per user** — the first DM auto-creates a dedicated forum topic plus a profile card (with the user's avatar); all later messages reuse the same topic.
- **Two-way relay** — text, photos, voice, video, documents, and other message types are copied in both directions.
- **Quote mapping** — when an agent quotes a message in the topic, the user sees a quote of their own copy, and user quotes map back to the topic.
- **Edit sync** — agent edits update the user's message in place; user edits are appended in the topic with a reference to the original.
- **Two-side deletion & cleanup** — reply with `/delete` to remove a message on both ends; `/clear` wipes the conversation on both sides while keeping the profile card; `/clear_user` clears the user side only.
- **Content protection** — `/protect on` marks all outgoing messages with Telegram's protect-content flag (no forwarding, saving, or screenshots).
- **Scoped command menus** — `setMyCommands` is called on startup; admins and users see different menus.

## How It Works

```
Telegram User  →  ForumDesk Bot  →  Support Group Topic
```

- **User → Agent**: every private message lands in that user's topic.
- **Agent → User**: agents simply reply in the topic; the bot maps the topic back to the user and delivers the message.

## Setup

1. Create a bot via [@BotFather](https://t.me/BotFather) and get the token.
2. Create a private supergroup and enable **Topics**.
3. Add the bot as an administrator with **Manage Topics**, **Send Messages**, and **Delete Messages** permissions.
4. Configure via environment variables:

```bash
cp .env.example .env

# edit .env
BOT_TOKEN=123456789:YOUR_TOKEN
SUPPORT_GROUP_ID=-1001234567890
DATA_FILE=./data/forumdesk.json
```

> Forum topics have no avatar; the bot uses the user's avatar as the first profile-card image to approximate one.

## Run

Local:

```bash
set -a
source .env
set +a
go run ./cmd/forumdesk
```

Docker Compose:

```bash
docker compose up -d --build
docker compose logs -f
```

Quality checks:

```bash
go test ./...
go vet ./...
make coverage
```

## Admin Commands

Used inside a user's topic:

| Command | Description |
|---|---|
| `/protect` [on/off] | View or toggle content protection for this user |
| `/delete`, `/del` | Reply to a message to delete it on both ends |
| `/clear` + `/clear confirm` | Clear messages on both sides, keeping the topic and profile card |
| `/clear_user` + confirm | Clear the user side only; admin topic history is kept |
| `/help` | Show full help |

Users only see `/delete` and `/help` in their private chat menu.

## Project Structure

```
cmd/forumdesk/       entrypoint
internal/app/        long-polling runner
internal/bot/        two-way topic routing
internal/config/     environment-based config
internal/store/      atomic JSON persistence
internal/telegram/   Telegram Bot API client
data/                runtime data (auto-created)
```

## Known Limitations

- The Bot API does not deliver deletion events for normal messages — deleting via the Telegram UI only affects one side. Use `/delete` for two-side removal. Telegram's deletion time limits and admin permission rules still apply.
- Album (media group) batching is not yet preserved; ban/close conversation actions, agent-claim buttons, PostgreSQL multi-instance deployment, and an admin dashboard are on the roadmap.
- Config is injected via environment variables only — the bot token is never written to code or data files.
