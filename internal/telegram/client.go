package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		baseURL: "https://api.telegram.org/bot" + token + "/",
		http:    &http.Client{Timeout: 70 * time.Second},
	}
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
}

func (c *Client) call(ctx context.Context, method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+method, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call %s: %w", method, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read %s response: %w", method, err)
	}
	var envelope apiResponse
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if resp.StatusCode != http.StatusOK || !envelope.OK {
		return fmt.Errorf("telegram %s failed (%d): %s", method, resp.StatusCode, envelope.Description)
	}
	if out != nil && len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
	}
	return nil
}

func (c *Client) GetUpdates(ctx context.Context, offset int) ([]Update, error) {
	var updates []Update
	err := c.call(ctx, "getUpdates", map[string]any{
		"offset": offset, "timeout": 55, "allowed_updates": []string{"message", "edited_message"},
	}, &updates)
	return updates, err
}

func (c *Client) EditMessageText(ctx context.Context, chatID int64, messageID int, text string) error {
	return c.call(ctx, "editMessageText", map[string]any{"chat_id": chatID, "message_id": messageID, "text": text}, nil)
}

func (c *Client) EditMessageCaption(ctx context.Context, chatID int64, messageID int, caption string) error {
	return c.call(ctx, "editMessageCaption", map[string]any{"chat_id": chatID, "message_id": messageID, "caption": caption}, nil)
}

func (c *Client) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	return c.call(ctx, "deleteMessage", map[string]any{"chat_id": chatID, "message_id": messageID}, nil)
}

func (c *Client) DeleteMessages(ctx context.Context, chatID int64, messageIDs []int) error {
	return c.call(ctx, "deleteMessages", map[string]any{"chat_id": chatID, "message_ids": messageIDs}, nil)
}

func (c *Client) DeleteForumTopic(ctx context.Context, chatID int64, topicID int) error {
	return c.call(ctx, "deleteForumTopic", map[string]any{"chat_id": chatID, "message_thread_id": topicID}, nil)
}

func (c *Client) IsChatAdministrator(ctx context.Context, chatID, userID int64) (bool, error) {
	var result struct {
		Status string `json:"status"`
	}
	if err := c.call(ctx, "getChatMember", map[string]any{"chat_id": chatID, "user_id": userID}, &result); err != nil {
		return false, err
	}
	return result.Status == "administrator" || result.Status == "creator", nil
}

func (c *Client) CreateForumTopic(ctx context.Context, chatID int64, name string) (int, error) {
	var result struct {
		MessageThreadID int `json:"message_thread_id"`
	}
	err := c.call(ctx, "createForumTopic", map[string]any{"chat_id": chatID, "name": name}, &result)
	return result.MessageThreadID, err
}

func (c *Client) CopyMessage(ctx context.Context, chatID, fromChatID int64, messageID int, options CopyOptions) (int, error) {
	payload := map[string]any{"chat_id": chatID, "from_chat_id": fromChatID, "message_id": messageID}
	if options.ThreadID != 0 {
		payload["message_thread_id"] = options.ThreadID
	}
	if options.ReplyToMessageID != 0 {
		payload["reply_parameters"] = map[string]any{"message_id": options.ReplyToMessageID}
	}
	if options.ProtectContent {
		payload["protect_content"] = true
	}
	var result struct {
		MessageID int `json:"message_id"`
	}
	err := c.call(ctx, "copyMessage", payload, &result)
	return result.MessageID, err
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, threadID int) (int, error) {
	payload := map[string]any{"chat_id": chatID, "text": text}
	if threadID != 0 {
		payload["message_thread_id"] = threadID
	}
	var result Message
	err := c.call(ctx, "sendMessage", payload, &result)
	return result.MessageID, err
}

func (c *Client) SetAdminCommands(ctx context.Context, chatID int64) error {
	commands := []map[string]string{
		{"command": "protect", "description": "内容保护状态或开关：/protect on|off"},
		{"command": "delete", "description": "回复一条消息后双端删除"},
		{"command": "clear", "description": "清空双方消息，保留 Topic 和资料卡"},
		{"command": "clear_user", "description": "仅清空用户端，保留管理端历史"},
		{"command": "allow_staff", "description": "回复成员消息，允许其发言对客可见"},
		{"command": "deny_staff", "description": "回复成员消息，恢复仅内部可见"},
		{"command": "show", "description": "回复消息，单独设为对客可见"},
		{"command": "hide", "description": "回复消息，单独设为仅内部可见"},
		{"command": "staff", "description": "查看当前 Topic 已授权成员"},
		{"command": "help", "description": "显示管理员命令说明"},
	}
	return c.call(ctx, "setMyCommands", map[string]any{
		"commands": commands,
		"scope":    map[string]any{"type": "chat_administrators", "chat_id": chatID},
	}, nil)
}

func (c *Client) SetPrivateCommands(ctx context.Context) error {
	commands := []map[string]string{
		{"command": "delete", "description": "回复一条消息后双端删除"},
		{"command": "help", "description": "显示可用命令"},
	}
	return c.call(ctx, "setMyCommands", map[string]any{
		"commands": commands,
		"scope":    map[string]string{"type": "all_private_chats"},
	}, nil)
}

func (c *Client) SendReplyMessage(ctx context.Context, chatID int64, text string, threadID, replyToMessageID int) (int, error) {
	payload := map[string]any{
		"chat_id":          chatID,
		"text":             text,
		"reply_parameters": map[string]any{"message_id": replyToMessageID},
	}
	if threadID != 0 {
		payload["message_thread_id"] = threadID
	}
	var result Message
	err := c.call(ctx, "sendMessage", payload, &result)
	return result.MessageID, err
}

func (c *Client) SendPhotoByFileID(ctx context.Context, chatID int64, fileID, caption string, threadID int) (int, error) {
	payload := map[string]any{"chat_id": chatID, "photo": fileID}
	if caption != "" {
		payload["caption"] = caption
	}
	if threadID != 0 {
		payload["message_thread_id"] = threadID
	}
	var result Message
	err := c.call(ctx, "sendPhoto", payload, &result)
	return result.MessageID, err
}

func (c *Client) FirstProfilePhoto(ctx context.Context, userID int64) (string, bool, error) {
	var result struct {
		TotalCount int `json:"total_count"`
		Photos     [][]struct {
			FileID string `json:"file_id"`
		} `json:"photos"`
	}
	err := c.call(ctx, "getUserProfilePhotos", map[string]any{"user_id": userID, "limit": 1}, &result)
	if err != nil {
		return "", false, err
	}
	if result.TotalCount == 0 || len(result.Photos) == 0 || len(result.Photos[0]) == 0 {
		return "", false, nil
	}
	photo := result.Photos[0][len(result.Photos[0])-1].FileID
	return photo, strings.TrimSpace(photo) != "", nil
}
