package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"forumdesk/internal/telegram"
)

type Conversation struct {
	UserID         int64     `json:"user_id"`
	TopicID        int       `json:"topic_id"`
	DisplayName    string    `json:"display_name"`
	Username       string    `json:"username,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
	ProtectContent bool      `json:"protect_content,omitempty"`
}

type MessageLink struct {
	UserID         int64     `json:"user_id"`
	UserMessageID  int       `json:"user_message_id"`
	TopicMessageID int       `json:"topic_message_id"`
	CreatedAt      time.Time `json:"created_at"`
}

type Store interface {
	ConversationByUser(context.Context, int64) (Conversation, bool, error)
	ConversationByTopic(context.Context, int64) (Conversation, bool, error)
	CreateConversation(context.Context, Conversation) error
	SaveMessageLink(context.Context, MessageLink) error
	MessageLinkByUser(context.Context, int64, int) (MessageLink, bool, error)
	MessageLinkByTopic(context.Context, int) (MessageLink, bool, error)
	DeleteMessageLink(context.Context, int64, int) error
	SetConversationProtection(context.Context, int64, bool) (Conversation, error)
	MessageLinksByUser(context.Context, int64) ([]MessageLink, error)
	DeleteConversation(context.Context, int64) error
}

type Telegram interface {
	CreateForumTopic(context.Context, int64, string) (int, error)
	CopyMessage(context.Context, int64, int64, int, telegram.CopyOptions) (int, error)
	SendMessage(context.Context, int64, string, int) (int, error)
	SendPhotoByFileID(context.Context, int64, string, string, int) (int, error)
	FirstProfilePhoto(context.Context, int64) (string, bool, error)
	EditMessageText(context.Context, int64, int, string) error
	EditMessageCaption(context.Context, int64, int, string) error
	DeleteMessage(context.Context, int64, int) error
	SendReplyMessage(context.Context, int64, string, int, int) (int, error)
	DeleteMessages(context.Context, int64, []int) error
	DeleteForumTopic(context.Context, int64, int) error
}

type Handler struct {
	supportGroupID int64
	store          Store
	telegram       Telegram
}

func NewHandler(supportGroupID int64, store Store, telegram Telegram) *Handler {
	return &Handler{supportGroupID: supportGroupID, store: store, telegram: telegram}
}

func (h *Handler) Handle(ctx context.Context, update telegram.Update) error {
	if update.EditedMessage != nil {
		return h.editedMessage(ctx, update.EditedMessage)
	}
	m := update.Message
	if m == nil || (m.From != nil && m.From.IsBot) {
		return nil
	}
	if m.Chat.Type == "private" {
		if isHelpCommand(m.Text) {
			return h.sendPrivateNotice(ctx, m, "可用命令：\n/delete — 回复一条消息后双端删除\n/help — 显示命令说明")
		}
		if matched, _ := clearCommand(m.Text); matched {
			return h.sendPrivateNotice(ctx, m, "整段对话删除仅由管理员操作；你可以回复单条消息后发送 /delete。")
		}
		if isDeleteCommand(m.Text) {
			return h.deletePair(ctx, m)
		}
		return h.fromUser(ctx, m)
	}
	if m.Chat.ID == h.supportGroupID && m.MessageThreadID != 0 {
		if matched, confirmed := clearCommand(m.Text); matched {
			return h.clearConversation(ctx, m, confirmed)
		}
		if isHelpCommand(m.Text) {
			_, err := h.telegram.SendMessage(ctx, h.supportGroupID,
				"管理命令：\n/protect on — 开启后续消息保护\n/protect off — 关闭保护\n/protect — 查看状态\n/delete — 回复消息后双端删除\n/clear — 双向删除整段对话",
				m.MessageThreadID)
			return err
		}
		if matched, action := protectCommand(m.Text); matched {
			return h.setProtection(ctx, m, action)
		}
		if isDeleteCommand(m.Text) {
			return h.deletePair(ctx, m)
		}
		return h.fromStaff(ctx, m)
	}
	return nil
}

func (h *Handler) sendPrivateNotice(ctx context.Context, command *telegram.Message, text string) error {
	responseID, err := h.telegram.SendMessage(ctx, command.Chat.ID, text, 0)
	if err != nil { return err }
	now := time.Now().UTC()
	if err := h.store.SaveMessageLink(ctx, MessageLink{UserID: command.Chat.ID, UserMessageID: command.MessageID, CreatedAt: now}); err != nil { return err }
	return h.store.SaveMessageLink(ctx, MessageLink{UserID: command.Chat.ID, UserMessageID: responseID, CreatedAt: now})
}

func (h *Handler) clearConversation(ctx context.Context, command *telegram.Message, confirmed bool) error {
	c, found, err := h.store.ConversationByTopic(ctx, int64(command.MessageThreadID))
	if err != nil {
		return fmt.Errorf("find conversation to clear: %w", err)
	}
	if !found {
		return fmt.Errorf("topic %d has no user mapping", command.MessageThreadID)
	}
	if !confirmed {
		_, err = h.telegram.SendMessage(ctx, h.supportGroupID,
			"⚠️ 这会双向删除整段对话并移除当前 Topic。发送 /clear confirm 确认。", command.MessageThreadID)
		return err
	}
	links, err := h.store.MessageLinksByUser(ctx, c.UserID)
	if err != nil {
		return fmt.Errorf("list conversation messages: %w", err)
	}
	messageIDs := make([]int, 0, len(links))
	seen := make(map[int]struct{}, len(links))
	for _, link := range links {
		if _, exists := seen[link.UserMessageID]; exists {
			continue
		}
		seen[link.UserMessageID] = struct{}{}
		messageIDs = append(messageIDs, link.UserMessageID)
	}
	for start := 0; start < len(messageIDs); start += 100 {
		end := start + 100
		if end > len(messageIDs) {
			end = len(messageIDs)
		}
		if err := h.telegram.DeleteMessages(ctx, c.UserID, messageIDs[start:end]); err != nil {
			_, _ = h.telegram.SendMessage(ctx, h.supportGroupID, "⚠️ 用户端批量删除失败，Topic 已保留，请检查日志后重试。", command.MessageThreadID)
			return fmt.Errorf("delete user conversation batch: %w", err)
		}
	}
	if err := h.telegram.DeleteForumTopic(ctx, h.supportGroupID, c.TopicID); err != nil {
		_, _ = h.telegram.SendMessage(ctx, h.supportGroupID, "⚠️ Topic 删除失败，请确认 Bot 拥有“删除消息”权限。", command.MessageThreadID)
		return fmt.Errorf("delete forum topic %d: %w", c.TopicID, err)
	}
	if err := h.store.DeleteConversation(ctx, c.UserID); err != nil {
		return fmt.Errorf("delete conversation mapping: %w", err)
	}
	return nil
}

func (h *Handler) setProtection(ctx context.Context, command *telegram.Message, action string) error {
	c, found, err := h.store.ConversationByTopic(ctx, int64(command.MessageThreadID))
	if err != nil {
		return fmt.Errorf("find topic protection settings: %w", err)
	}
	if !found {
		return fmt.Errorf("topic %d has no user mapping", command.MessageThreadID)
	}
	if action == "on" || action == "off" {
		c, err = h.store.SetConversationProtection(ctx, c.UserID, action == "on")
		if err != nil {
			return fmt.Errorf("save content protection: %w", err)
		}
	}
	status := "关闭"
	if c.ProtectContent {
		status = "开启"
	}
	_, err = h.telegram.SendMessage(ctx, h.supportGroupID,
		fmt.Sprintf("🔒 禁止转发/保存：%s\n使用 /protect on 开启，/protect off 关闭\n设置只影响之后发送的消息。", status),
		command.MessageThreadID)
	return err
}

func (h *Handler) editedMessage(ctx context.Context, m *telegram.Message) error {
	if m.From != nil && m.From.IsBot {
		return nil
	}
	var link MessageLink
	var found bool
	var err error
	if m.Chat.Type == "private" {
		link, found, err = h.store.MessageLinkByUser(ctx, m.Chat.ID, m.MessageID)
	} else if m.Chat.ID == h.supportGroupID && m.MessageThreadID != 0 {
		link, found, err = h.store.MessageLinkByTopic(ctx, m.MessageID)
	} else {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find edited message %d mapping: %w", m.MessageID, err)
	}
	if !found {
		return fmt.Errorf("edited message %d has no message mapping", m.MessageID)
	}
	if m.Chat.Type == "private" {
		conversation, exists, conversationErr := h.store.ConversationByUser(ctx, m.Chat.ID)
		if conversationErr != nil {
			return fmt.Errorf("find edited message topic: %w", conversationErr)
		}
		if !exists {
			return fmt.Errorf("edited message user %d has no topic mapping", m.Chat.ID)
		}
		content := m.Text
		if content == "" {
			content = m.Caption
		}
		if content == "" {
			content = "（内容已清空）"
		}
		_, err = h.telegram.SendReplyMessage(ctx, h.supportGroupID, "✏️ 用户修改为：\n"+content, conversation.TopicID, link.TopicMessageID)
		if err != nil {
			return fmt.Errorf("send user edit for message %d: %w", m.MessageID, err)
		}
		return nil
	}
	destinationChatID, destinationMessageID := link.UserID, link.UserMessageID
	if m.Text != "" {
		if err := h.telegram.EditMessageText(ctx, destinationChatID, destinationMessageID, m.Text); err != nil {
			return fmt.Errorf("sync text edit for message %d: %w", m.MessageID, err)
		}
		return nil
	}
	if err := h.telegram.EditMessageCaption(ctx, destinationChatID, destinationMessageID, m.Caption); err != nil {
		return fmt.Errorf("sync caption edit for message %d: %w", m.MessageID, err)
	}
	return nil
}

func (h *Handler) deletePair(ctx context.Context, command *telegram.Message) error {
	if command.ReplyToMessage == nil {
		if command.Chat.Type == "private" {
			return h.sendPrivateNotice(ctx, command, "请回复要双端删除的消息，然后发送 /delete")
		}
		_, err := h.telegram.SendMessage(ctx, command.Chat.ID, "请回复要双端删除的消息，然后发送 /delete", command.MessageThreadID)
		return err
	}
	var link MessageLink
	var found bool
	var err error
	if command.Chat.Type == "private" {
		link, found, err = h.store.MessageLinkByUser(ctx, command.Chat.ID, command.ReplyToMessage.MessageID)
	} else {
		link, found, err = h.store.MessageLinkByTopic(ctx, command.ReplyToMessage.MessageID)
	}
	if err != nil {
		return fmt.Errorf("find message to delete: %w", err)
	}
	if !found {
		return fmt.Errorf("message %d has no message mapping", command.ReplyToMessage.MessageID)
	}
	var failures []error
	if err := h.telegram.DeleteMessage(ctx, link.UserID, link.UserMessageID); err != nil && !messageAlreadyDeleted(err) {
		failures = append(failures, fmt.Errorf("delete user message %d: %w", link.UserMessageID, err))
	}
	if link.TopicMessageID != 0 {
		if err := h.telegram.DeleteMessage(ctx, h.supportGroupID, link.TopicMessageID); err != nil && !messageAlreadyDeleted(err) {
			failures = append(failures, fmt.Errorf("delete topic message %d: %w", link.TopicMessageID, err))
		}
	}
	if err := h.telegram.DeleteMessage(ctx, command.Chat.ID, command.MessageID); err != nil && !messageAlreadyDeleted(err) {
		failures = append(failures, fmt.Errorf("delete command message %d: %w", command.MessageID, err))
	}
	if len(failures) > 0 {
		_, noticeErr := h.telegram.SendMessage(ctx, command.Chat.ID, "⚠️ 删除消息未完成，请确认 Bot 拥有“删除消息”管理员权限，并重试 /delete", command.MessageThreadID)
		if noticeErr != nil {
			failures = append(failures, noticeErr)
		}
		return errors.Join(failures...)
	}
	return h.store.DeleteMessageLink(ctx, link.UserID, link.UserMessageID)
}

func messageAlreadyDeleted(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "message to delete not found")
}

func isDeleteCommand(text string) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}
	command := strings.SplitN(strings.ToLower(fields[0]), "@", 2)[0]
	return command == "/delete" || command == "/del"
}

func protectCommand(text string) (bool, string) {
	fields := strings.Fields(strings.ToLower(text))
	if len(fields) == 0 {
		return false, ""
	}
	if strings.SplitN(fields[0], "@", 2)[0] != "/protect" {
		return false, ""
	}
	if len(fields) == 1 {
		return true, "status"
	}
	if fields[1] == "on" || fields[1] == "off" {
		return true, fields[1]
	}
	return true, "status"
}

func isHelpCommand(text string) bool {
	fields := strings.Fields(strings.ToLower(text))
	return len(fields) > 0 && strings.SplitN(fields[0], "@", 2)[0] == "/help"
}

func clearCommand(text string) (bool, bool) {
	fields := strings.Fields(strings.ToLower(text))
	if len(fields) == 0 || strings.SplitN(fields[0], "@", 2)[0] != "/clear" {
		return false, false
	}
	return true, len(fields) > 1 && fields[1] == "confirm"
}

func (h *Handler) fromUser(ctx context.Context, m *telegram.Message) error {
	userID := m.Chat.ID
	conversation, found, err := h.store.ConversationByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("find conversation for user %d: %w", userID, err)
	}
	if !found {
		conversation, err = h.startConversation(ctx, m)
		if err != nil {
			return err
		}
	}
	options := telegram.CopyOptions{ThreadID: conversation.TopicID, ProtectContent: conversation.ProtectContent}
	if m.ReplyToMessage != nil {
		link, linkFound, linkErr := h.store.MessageLinkByUser(ctx, userID, m.ReplyToMessage.MessageID)
		if linkErr != nil {
			return fmt.Errorf("find replied user message: %w", linkErr)
		}
		if linkFound {
			options.ReplyToMessageID = link.TopicMessageID
		}
	}
	topicMessageID, err := h.telegram.CopyMessage(ctx, h.supportGroupID, userID, m.MessageID, options)
	if err != nil {
		return fmt.Errorf("copy user message %d to topic %d: %w", m.MessageID, conversation.TopicID, err)
	}
	return h.store.SaveMessageLink(ctx, MessageLink{UserID: userID, UserMessageID: m.MessageID, TopicMessageID: topicMessageID, CreatedAt: time.Now().UTC()})
}

func (h *Handler) startConversation(ctx context.Context, m *telegram.Message) (Conversation, error) {
	displayName, username := displayName(m.From), ""
	if m.From != nil {
		username = m.From.Username
	}
	topicName := displayName
	if username != "" {
		topicName += " · @" + username
	}
	if len([]rune(topicName)) > 128 {
		topicName = string([]rune(topicName)[:128])
	}
	topicID, err := h.telegram.CreateForumTopic(ctx, h.supportGroupID, topicName)
	if err != nil {
		return Conversation{}, fmt.Errorf("create topic for user %d: %w", m.Chat.ID, err)
	}
	c := Conversation{UserID: m.Chat.ID, TopicID: topicID, DisplayName: displayName, Username: username, CreatedAt: time.Now().UTC()}
	if err := h.store.CreateConversation(ctx, c); err != nil {
		return Conversation{}, fmt.Errorf("save topic %d mapping: %w", topicID, err)
	}
	card := fmt.Sprintf("👤 用户资料\n名称：%s\n用户名：%s\n用户 ID：%d\n\n管理命令：\n/protect on — 禁止后续消息转发/保存\n/protect off — 关闭保护\n回复消息发送 /delete — 双端删除单条消息\n/clear — 双向删除整段对话", displayName, usernameLabel(username), m.Chat.ID)
	if fileID, ok, photoErr := h.telegram.FirstProfilePhoto(ctx, m.Chat.ID); photoErr == nil && ok {
		if _, photoErr = h.telegram.SendPhotoByFileID(ctx, h.supportGroupID, fileID, card, topicID); photoErr == nil {
			return c, nil
		}
	}
	_, _ = h.telegram.SendMessage(ctx, h.supportGroupID, card, topicID)
	return c, nil
}

func (h *Handler) fromStaff(ctx context.Context, m *telegram.Message) error {
	c, found, err := h.store.ConversationByTopic(ctx, int64(m.MessageThreadID))
	if err != nil {
		return fmt.Errorf("find topic %d: %w", m.MessageThreadID, err)
	}
	if !found {
		return fmt.Errorf("topic %d has no user mapping", m.MessageThreadID)
	}
	options := telegram.CopyOptions{ProtectContent: c.ProtectContent}
	if m.ReplyToMessage != nil {
		link, linkFound, linkErr := h.store.MessageLinkByTopic(ctx, m.ReplyToMessage.MessageID)
		if linkErr != nil {
			return fmt.Errorf("find replied topic message: %w", linkErr)
		}
		if linkFound && link.UserID == c.UserID {
			options.ReplyToMessageID = link.UserMessageID
		}
	}
	userMessageID, err := h.telegram.CopyMessage(ctx, c.UserID, h.supportGroupID, m.MessageID, options)
	if err != nil {
		return fmt.Errorf("copy topic message %d to user %d: %w", m.MessageID, c.UserID, err)
	}
	return h.store.SaveMessageLink(ctx, MessageLink{UserID: c.UserID, UserMessageID: userMessageID, TopicMessageID: m.MessageID, CreatedAt: time.Now().UTC()})
}

func displayName(u *telegram.User) string {
	if u == nil {
		return "访客"
	}
	name := strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
	if name == "" {
		return fmt.Sprintf("访客-%d", u.ID)
	}
	return name
}

func usernameLabel(username string) string {
	if username == "" {
		return "未设置"
	}
	return "@" + username
}
