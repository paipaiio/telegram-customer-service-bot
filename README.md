# ForumDesk · Telegram 客服私聊中转 Bot / Telegram Customer Service Bot

[English README](README_EN.md) | [图文文档 / Visual Docs](README.html)

[![License: CC BY-NC-SA 4.0](https://img.shields.io/badge/License-CC%20BY--NC--SA%204.0-lightgrey.svg)](https://creativecommons.org/licenses/by-nc-sa/4.0/)

**许可证 / License**: [CC BY-NC-SA 4.0](LICENSE) — 禁止商用。允许使用、修改、分发，但必须署名、非商用、衍生作品采用相同许可证。Commercial use is prohibited; you may use, modify, and distribute with attribution, non-commercially, under the same license.

一个自托管的 Telegram 客服机器人：把每位私聊用户放进私有超级群的独立 Topic（话题），客服在 Topic 里回复，Bot 自动双向中转。

A self-hosted Telegram customer service bot: each private chat user gets a dedicated forum topic in a private supergroup. Agents reply inside the topic, and the bot relays messages both ways.

---

## 功能特性 / Features

- **单用户单 Topic / One topic per user** — 用户首次私聊自动创建专属 Topic 和资料卡；之后所有消息复用同一 Topic。First message auto-creates a dedicated topic with a profile card; later messages reuse it.
- **双向消息中转 / Two-way relay** — 文本、图片、语音、视频、文件等消息双向复制。Relays text, photos, voice, video, documents, etc. in both directions.
- **双向引用映射 / Quote mapping** — 客服在 Topic 引用历史消息，用户端引用对应副本；用户引用也映射回 Topic。Replies/quotes are mapped across both sides.
- **双向编辑同步 / Edit sync** — 客服编辑直接修改用户端消息；用户编辑会在 Topic 引用原消息并追加新内容。Agent edits update the user's message in place; user edits are appended with a quote.
- **双端删除与批量清理 / Two-side deletion & cleanup** — 回复消息发送 `/delete` 删除两端消息；`/clear` 清空双方对话并保留资料卡；`/clear_user` 仅清空用户端。`/delete` removes a message on both ends; `/clear` wipes both sides keeping the profile card; `/clear_user` clears the user side only.
- **内容保护 / Content protection** — `/protect on` 后发出的消息禁止转发、保存与截图。`/protect on` marks outgoing messages with Telegram content protection.
- **命令菜单 / Command menu** — 启动时自动注册 `setMyCommands`，管理员与用户看到不同菜单。Auto-registers scoped command menus on startup.

## 工作流程 / How It Works

```
Telegram 用户 User  →  ForumDesk Bot  →  客服群 Topic / Support Group Topic
```

- 用户 → 客服：所有私聊消息进入该用户的 Topic。User → Agent: every DM lands in the user's topic.
- 客服 → 用户：管理员在 Topic 直接回复，Bot 按 Topic 映射发回用户。Agent → User: agents reply in the topic; the bot forwards to the mapped user.

## 首次配置 / Setup

1. 通过 @BotFather 创建 Bot 并取得 Token。Create a bot via @BotFather and get the token.
2. 创建私有超级群，开启「话题 / Topics」。Create a private supergroup and enable Topics.
3. 把 Bot 设为管理员，授予管理话题、发送消息、删除消息权限。Add the bot as admin with Manage Topics, Send Messages, Delete Messages.
4. 复制配置并填入 Token 与群 ID。Copy the config and fill in token and group ID:

```bash
cp .env.example .env

# 编辑 .env / edit .env
BOT_TOKEN=123456789:YOUR_TOKEN
SUPPORT_GROUP_ID=-1001234567890
DATA_FILE=./data/forumdesk.json
```

> Topic 没有独立头像，基础版会把用户头像作为 Topic 的第一张资料卡图片。
> Topics have no avatar; the bot uses the user's avatar as the first profile-card image instead.

## 启动 / Run

本机运行 / Local:

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

质量检查 / Checks:

```bash
go test ./...
go vet ./...
make coverage
```

## 管理员命令 / Admin Commands

在 Topic 内使用 / Used inside a topic:

| 命令 Command | 说明 Description |
|---|---|
| `/protect` [on/off] | 查看或设置内容保护 / View or toggle content protection |
| `/delete`, `/del` | 回复消息后双端删除 / Delete a replied message on both ends |
| `/clear` + `/clear confirm` | 清空双方消息并保留资料卡 / Clear both sides, keep profile card |
| `/clear_user` + confirm | 仅清空用户端 / Clear user side only |
| `/help` | 显示完整说明 / Show full help |

用户私聊只显示 `/delete` 和 `/help`。Users only see `/delete` and `/help`.

## 项目结构 / Project Structure

```
cmd/forumdesk/       程序入口 / entrypoint
internal/app/        长轮询运行器 / long-polling runner
internal/bot/        Topic 双向路由 / two-way topic routing
internal/config/     环境变量配置 / env config
internal/store/      JSON 原子持久化 / atomic JSON persistence
internal/telegram/   Telegram Bot API 客户端 / Bot API client
data/                运行数据（自动创建）/ runtime data (auto-created)
```

## 已知边界 / Known Limitations

- 普通 Bot API 不推送常规消息的删除事件，直接点 Telegram 的「删除」只影响当前一端，请用 `/delete` 触发双端删除。
  The Bot API doesn't deliver deletion events for normal messages — use `/delete` for two-side removal.
- 相册批量消息暂未保持分组；封禁、会话关闭、PostgreSQL 多实例等在路线图中。
  Album grouping, ban/close actions, and PostgreSQL multi-instance support are on the roadmap.
- 配置仅通过环境变量注入，Bot Token 不写入代码或数据文件。
  Config is injected via environment variables only; the bot token is never written to code or data files.
