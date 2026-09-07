# ForumDesk 网页客户端示例

[English](web-client_EN.md) · [运行示例](web-client.html) · [Web API 文档](../API.md) · [项目首页](../README.md)

`web-client.html` 是一个无构建步骤的浏览器示例，用于验证会话连接、增量轮询、消息发送、引用回复和 `message_hidden` 事件。

## 使用方法

1. 由业务后端调用 `POST /api/v1/conversations` 创建会话。
2. 打开 `web-client.html`。
3. 填写 API Base URL、Conversation ID 和 Visitor Token。
4. 点击“连接”，发送消息并在 Telegram Topic 中回复。

生产环境推荐让浏览器请求业务系统自己的同域后端，再由业务后端通过 Tailscale 等私有网络调用 ForumDesk。不要把 `WEB_API_KEY` 写入网页。

## 页面配置

| 输入项 | 示例 |
|---|---|
| API Base URL | `http://100.64.0.2:8980` |
| Conversation ID | `wc_...` |
| Visitor Token | `wv_...` |

点击已有消息即可建立引用，发送请求时会附带 `reply_to_message_id`。收到 `message_hidden` 后，页面会移除 `target_message_id` 对应的消息。
