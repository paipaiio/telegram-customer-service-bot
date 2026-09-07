# ForumDesk Browser Client Example

[简体中文](web-client.md) · [Run the example](web-client.html) · [Web API documentation](../API_EN.md) · [Project README](../README_EN.md)

`web-client.html` is a build-free browser example for testing conversation connection, cursor polling, message delivery, quoted replies, and `message_hidden` events.

## Usage

1. Create a conversation through `POST /api/v1/conversations` from your application backend.
2. Open `web-client.html`.
3. Enter the API Base URL, Conversation ID, and Visitor Token.
4. Connect, send a message, and reply from the matching Telegram topic.

For production, keep the browser on your application's origin and proxy ForumDesk through your backend over Tailscale or another private network. Keep `WEB_API_KEY` out of browser code.

## Page settings

| Field | Example |
|---|---|
| API Base URL | `http://100.64.0.2:8980` |
| Conversation ID | `wc_...` |
| Visitor Token | `wv_...` |

Click an existing message to set `reply_to_message_id`. When a `message_hidden` event arrives, remove the rendered message identified by `target_message_id`.
