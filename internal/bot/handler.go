package bot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"forumdesk/internal/telegram"
)

type Conversation struct {
	UserID         int64     `json:"user_id"`
	TopicID        int       `json:"topic_id"`
	PublicID       string    `json:"public_id,omitempty"`
	Channel        string    `json:"channel,omitempty"`
	VisitorToken   string    `json:"visitor_token_hash,omitempty"`
	VisitorEmail   string    `json:"visitor_email,omitempty"`
	ExternalID     string    `json:"external_id,omitempty"`
	PageURL        string    `json:"page_url,omitempty"`
	DisplayName    string    `json:"display_name"`
	Username       string    `json:"username,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
	ProtectContent bool      `json:"protect_content,omitempty"`
}

type MessageLink struct {
	UserID         int64     `json:"user_id"`
	UserMessageID  int       `json:"user_message_id"`
	TopicMessageID int       `json:"topic_message_id"`
	WebMessageID   string    `json:"web_message_id,omitempty"`
	Hidden         bool      `json:"hidden,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type WebMessage struct {
	ID               string    `json:"id"`
	ConversationID   string    `json:"conversation_id"`
	ReplyToMessageID string    `json:"reply_to_message_id,omitempty"`
	Sequence         int64     `json:"sequence"`
	Direction        string    `json:"direction"`
	Event            string    `json:"event,omitempty"`
	TargetMessageID  string    `json:"target_message_id,omitempty"`
	Text             string    `json:"text"`
	StaffUserID      int64     `json:"staff_user_id,omitempty"`
	StaffName        string    `json:"staff_name,omitempty"`
	TopicMessageID   int       `json:"-"`
	Visible          bool      `json:"visible"`
	CreatedAt        time.Time `json:"created_at"`
}

type Store interface {
	ConversationByUser(context.Context, int64) (Conversation, bool, error)
	ConversationByTopic(context.Context, int64) (Conversation, bool, error)
	CreateConversation(context.Context, Conversation) error
	SaveMessageLink(context.Context, MessageLink) error
	MessageLinkByUser(context.Context, int64, int) (MessageLink, bool, error)
	MessageLinkByTopic(context.Context, int) (MessageLink, bool, error)
	DeleteMessageLink(context.Context, MessageLink) error
	SetConversationProtection(context.Context, int64, bool) (Conversation, error)
	MessageLinksByUser(context.Context, int64) ([]MessageLink, error)
	ReplaceMessageLinksByUser(context.Context, int64, []MessageLink) error
	SetStaffVisibility(context.Context, int64, int64, bool) error
	StaffVisible(context.Context, int64, int64) (bool, error)
	VisibleStaff(context.Context, int64) ([]int64, error)
	AddWebMessage(context.Context, WebMessage) (WebMessage, error)
	WebMessageByID(context.Context, string) (WebMessage, bool, error)
	SetWebMessageVisible(context.Context, string, bool) error
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
	IsChatAdministrator(context.Context, int64, int64) (bool, error)
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
		if matched, _ := clearUserCommand(m.Text); matched {
			return h.sendPrivateNotice(ctx, m, "整段对话删除仅由管理员操作；你可以回复单条消息后发送 /delete。")
		}
		if isDeleteCommand(m.Text) {
			return h.deletePair(ctx, m)
		}
		return h.fromUser(ctx, m)
	}
	if m.Chat.ID == h.supportGroupID && m.MessageThreadID != 0 {
		if staffAction, matched := staffControlCommand(m.Text); matched {
			return h.controlStaffVisibility(ctx, m, staffAction)
		}
		if visibilityAction, matched := messageVisibilityCommand(m.Text); matched {
			return h.controlMessageVisibility(ctx, m, visibilityAction)
		}
		if matched, confirmed := clearCommand(m.Text); matched {
			return h.clearConversation(ctx, m, confirmed)
		}
		if matched, confirmed := clearUserCommand(m.Text); matched {
			return h.clearUserConversation(ctx, m, confirmed)
		}
		if isHelpCommand(m.Text) {
			return h.sendTopicNotice(ctx, m,
				"管理命令：\n/protect on|off — 后续消息保护\n/delete — 回复后双端删除\n/clear — 清空双方，保留 Topic 和资料卡\n/clear_user — 仅清空客户端\n/allow_staff — 回复成员消息并授权自动外发\n/deny_staff — 取消成员自动外发\n/staff — 查看已授权成员\n/show — 回复后单条对客可见\n/hide — 回复后单条仅内部可见",
			)
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
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := h.store.SaveMessageLink(ctx, MessageLink{UserID: command.Chat.ID, UserMessageID: command.MessageID, CreatedAt: now}); err != nil {
		return err
	}
	return h.store.SaveMessageLink(ctx, MessageLink{UserID: command.Chat.ID, UserMessageID: responseID, CreatedAt: now})
}

func (h *Handler) sendTopicNotice(ctx context.Context, command *telegram.Message, text string) error {
	c, found, err := h.store.ConversationByTopic(ctx, int64(command.MessageThreadID))
	if err != nil {
		return fmt.Errorf("find topic notice conversation: %w", err)
	}
	if !found {
		return fmt.Errorf("topic %d has no user mapping", command.MessageThreadID)
	}
	return h.sendTopicNoticeForUser(ctx, c.UserID, command, text)
}

func (h *Handler) sendTopicNoticeForUser(ctx context.Context, userID int64, command *telegram.Message, text string) error {
	responseID, err := h.telegram.SendMessage(ctx, h.supportGroupID, text, command.MessageThreadID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := h.store.SaveMessageLink(ctx, MessageLink{UserID: userID, TopicMessageID: command.MessageID, Hidden: true, CreatedAt: now}); err != nil {
		return err
	}
	return h.store.SaveMessageLink(ctx, MessageLink{UserID: userID, TopicMessageID: responseID, Hidden: true, CreatedAt: now})
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
		return h.sendTopicNoticeForUser(ctx, c.UserID, command,
			"⚠️ 这会清空双方的对话消息，保留当前 Topic 和首条用户资料卡。发送 /clear confirm 确认。")
	}
	links, err := h.store.MessageLinksByUser(ctx, c.UserID)
	if err != nil {
		return fmt.Errorf("list conversation messages: %w", err)
	}
	userMessageIDs := make([]int, 0, len(links))
	topicMessageIDs := make([]int, 0, len(links)+1)
	for _, link := range links {
		if link.UserMessageID != 0 {
			userMessageIDs = append(userMessageIDs, link.UserMessageID)
		}
		if link.TopicMessageID != 0 {
			topicMessageIDs = append(topicMessageIDs, link.TopicMessageID)
		}
	}
	topicMessageIDs = append(topicMessageIDs, command.MessageID)
	if err := h.deleteMessageIDs(ctx, c.UserID, userMessageIDs); err != nil {
		_ = h.sendTopicNoticeForUser(ctx, c.UserID, command, "⚠️ 用户端批量删除失败，Topic 已保留，请检查日志后重试。")
		return fmt.Errorf("delete user conversation batch: %w", err)
	}
	if err := h.deleteMessageIDs(ctx, h.supportGroupID, topicMessageIDs); err != nil {
		_ = h.sendTopicNoticeForUser(ctx, c.UserID, command, "⚠️ 管理端批量删除失败，Topic 和资料卡已保留，请检查权限后重试。")
		return fmt.Errorf("delete topic conversation batch: %w", err)
	}
	if err := h.store.ReplaceMessageLinksByUser(ctx, c.UserID, nil); err != nil {
		return fmt.Errorf("clear conversation message mappings: %w", err)
	}
	return nil
}

func (h *Handler) clearUserConversation(ctx context.Context, command *telegram.Message, confirmed bool) error {
	c, found, err := h.store.ConversationByTopic(ctx, int64(command.MessageThreadID))
	if err != nil {
		return fmt.Errorf("find conversation to clear user side: %w", err)
	}
	if !found {
		return fmt.Errorf("topic %d has no user mapping", command.MessageThreadID)
	}
	if !confirmed {
		return h.sendTopicNoticeForUser(ctx, c.UserID, command,
			"⚠️ 这会仅清空用户端消息，管理端 Topic 历史完整保留。发送 /clear_user confirm 确认。")
	}
	links, err := h.store.MessageLinksByUser(ctx, c.UserID)
	if err != nil {
		return fmt.Errorf("list user-side messages: %w", err)
	}
	userMessageIDs := make([]int, 0, len(links))
	retained := make([]MessageLink, 0, len(links))
	for _, link := range links {
		if link.UserMessageID != 0 {
			userMessageIDs = append(userMessageIDs, link.UserMessageID)
		}
		if link.TopicMessageID != 0 {
			link.UserMessageID = 0
			retained = append(retained, link)
		}
	}
	if err := h.deleteMessageIDs(ctx, c.UserID, userMessageIDs); err != nil {
		_ = h.sendTopicNoticeForUser(ctx, c.UserID, command, "⚠️ 用户端批量删除失败，管理端历史已保留，请检查日志后重试。")
		return fmt.Errorf("delete user-only conversation batch: %w", err)
	}
	if err := h.store.ReplaceMessageLinksByUser(ctx, c.UserID, retained); err != nil {
		return fmt.Errorf("clear user-side message mappings: %w", err)
	}
	return h.sendTopicNoticeForUser(ctx, c.UserID, command, "✅ 用户端消息已清空，管理端 Topic 历史已保留。")
}

func (h *Handler) deleteMessageIDs(ctx context.Context, chatID int64, messageIDs []int) error {
	unique := make([]int, 0, len(messageIDs))
	seen := make(map[int]struct{}, len(messageIDs))
	for _, messageID := range messageIDs {
		if messageID == 0 {
			continue
		}
		if _, exists := seen[messageID]; exists {
			continue
		}
		seen[messageID] = struct{}{}
		unique = append(unique, messageID)
	}
	for start := 0; start < len(unique); start += 100 {
		end := start + 100
		if end > len(unique) {
			end = len(unique)
		}
		if err := h.telegram.DeleteMessages(ctx, chatID, unique[start:end]); err != nil {
			return err
		}
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
	return h.sendTopicNoticeForUser(ctx, c.UserID, command,
		fmt.Sprintf("🔒 禁止转发/保存：%s\n使用 /protect on 开启，/protect off 关闭\n设置只影响之后发送的消息。", status),
	)
}

func (h *Handler) controlStaffVisibility(ctx context.Context, command *telegram.Message, action string) error {
	c, found, err := h.store.ConversationByTopic(ctx, int64(command.MessageThreadID))
	if err != nil {
		return fmt.Errorf("find staff visibility conversation: %w", err)
	}
	if !found {
		return fmt.Errorf("topic %d has no user mapping", command.MessageThreadID)
	}
	admin, err := h.commandFromAdmin(ctx, command)
	if err != nil {
		return err
	}
	if !admin {
		return h.sendTopicNoticeForUser(ctx, c.UserID, command, "⛔ 只有管理员可以修改客户可见范围。")
	}
	if action == "list" {
		staffIDs, err := h.store.VisibleStaff(ctx, c.UserID)
		if err != nil {
			return fmt.Errorf("list visible staff: %w", err)
		}
		if len(staffIDs) == 0 {
			return h.sendTopicNoticeForUser(ctx, c.UserID, command, "👥 当前没有已授权自动对客可见的成员。")
		}
		parts := make([]string, 0, len(staffIDs))
		for _, staffID := range staffIDs {
			parts = append(parts, strconv.FormatInt(staffID, 10))
		}
		return h.sendTopicNoticeForUser(ctx, c.UserID, command, "👥 已授权成员 ID："+strings.Join(parts, ", "))
	}
	if command.ReplyToMessage == nil || command.ReplyToMessage.From == nil || command.ReplyToMessage.From.IsBot {
		return h.sendTopicNoticeForUser(ctx, c.UserID, command, "请回复目标成员的一条消息，再发送 /allow_staff 或 /deny_staff。")
	}
	staff := command.ReplyToMessage.From
	visible := action == "allow"
	if err := h.store.SetStaffVisibility(ctx, c.UserID, staff.ID, visible); err != nil {
		return fmt.Errorf("save staff visibility: %w", err)
	}
	state := "已允许自动对客可见"
	if !visible {
		state = "已转为仅内部可见"
	}
	return h.sendTopicNoticeForUser(ctx, c.UserID, command, fmt.Sprintf("👥 %s（ID %d）%s。", displayName(staff), staff.ID, state))
}

func (h *Handler) controlMessageVisibility(ctx context.Context, command *telegram.Message, action string) error {
	c, found, err := h.store.ConversationByTopic(ctx, int64(command.MessageThreadID))
	if err != nil {
		return fmt.Errorf("find message visibility conversation: %w", err)
	}
	if !found {
		return fmt.Errorf("topic %d has no user mapping", command.MessageThreadID)
	}
	admin, err := h.commandFromAdmin(ctx, command)
	if err != nil {
		return err
	}
	if !admin {
		return h.sendTopicNoticeForUser(ctx, c.UserID, command, "⛔ 只有管理员可以修改单条消息的客户可见性。")
	}
	if command.ReplyToMessage == nil {
		return h.sendTopicNoticeForUser(ctx, c.UserID, command, "请回复目标消息，再发送 /show 或 /hide。")
	}
	link, linkFound, err := h.store.MessageLinkByTopic(ctx, command.ReplyToMessage.MessageID)
	if err != nil {
		return fmt.Errorf("find visibility message mapping: %w", err)
	}
	if action == "show" {
		if linkFound && !link.Hidden {
			return h.sendTopicNoticeForUser(ctx, c.UserID, command, "✅ 该消息已对客户可见。")
		}
		if err := h.publishStaffMessage(ctx, c, command.ReplyToMessage, link, linkFound); err != nil {
			return err
		}
		return h.sendTopicNoticeForUser(ctx, c.UserID, command, "✅ 该消息已单独设为客户可见。")
	}
	if !linkFound || link.Hidden {
		return h.sendTopicNoticeForUser(ctx, c.UserID, command, "👁 该消息当前只在管理端可见。")
	}
	if c.Channel == "web" {
		if err := h.store.SetWebMessageVisible(ctx, link.WebMessageID, false); err != nil {
			return fmt.Errorf("hide web message: %w", err)
		}
		eventID, err := newWebMessageID()
		if err != nil {
			return err
		}
		_, err = h.store.AddWebMessage(ctx, WebMessage{ID: eventID, ConversationID: c.PublicID, Direction: "system", Event: "message_hidden", TargetMessageID: link.WebMessageID, Visible: true, CreatedAt: time.Now().UTC()})
		if err != nil {
			return fmt.Errorf("append web hide event: %w", err)
		}
	} else if link.UserMessageID != 0 {
		if err := h.telegram.DeleteMessage(ctx, c.UserID, link.UserMessageID); err != nil && !messageAlreadyDeleted(err) {
			return fmt.Errorf("hide telegram message %d: %w", link.UserMessageID, err)
		}
	}
	if err := h.store.DeleteMessageLink(ctx, link); err != nil {
		return err
	}
	link.UserMessageID = 0
	link.Hidden = true
	if err := h.store.SaveMessageLink(ctx, link); err != nil {
		return err
	}
	return h.sendTopicNoticeForUser(ctx, c.UserID, command, "👁 该消息已转为仅管理端可见。")
}

func (h *Handler) commandFromAdmin(ctx context.Context, command *telegram.Message) (bool, error) {
	if command.From == nil {
		return false, nil
	}
	admin, err := h.telegram.IsChatAdministrator(ctx, h.supportGroupID, command.From.ID)
	if err != nil {
		return false, fmt.Errorf("check administrator %d: %w", command.From.ID, err)
	}
	return admin, nil
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
	if destinationMessageID == 0 {
		return nil
	}
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
		return h.sendTopicNotice(ctx, command, "请回复要双端删除的消息，然后发送 /delete")
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
	if link.UserMessageID != 0 {
		if err := h.telegram.DeleteMessage(ctx, link.UserID, link.UserMessageID); err != nil && !messageAlreadyDeleted(err) {
			failures = append(failures, fmt.Errorf("delete user message %d: %w", link.UserMessageID, err))
		}
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
	return h.store.DeleteMessageLink(ctx, link)
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

func clearUserCommand(text string) (bool, bool) {
	fields := strings.Fields(strings.ToLower(text))
	if len(fields) == 0 || strings.SplitN(fields[0], "@", 2)[0] != "/clear_user" {
		return false, false
	}
	return true, len(fields) > 1 && fields[1] == "confirm"
}

func staffControlCommand(text string) (string, bool) {
	fields := strings.Fields(strings.ToLower(text))
	if len(fields) == 0 {
		return "", false
	}
	switch strings.SplitN(fields[0], "@", 2)[0] {
	case "/allow_staff":
		return "allow", true
	case "/deny_staff":
		return "deny", true
	case "/staff":
		return "list", true
	default:
		return "", false
	}
}

func messageVisibilityCommand(text string) (string, bool) {
	fields := strings.Fields(strings.ToLower(text))
	if len(fields) == 0 {
		return "", false
	}
	switch strings.SplitN(fields[0], "@", 2)[0] {
	case "/show":
		return "show", true
	case "/hide":
		return "hide", true
	default:
		return "", false
	}
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
	card := fmt.Sprintf("👤 用户资料\n名称：%s\n用户名：%s\n用户 ID：%d\n\n使用 /help 查看管理命令\n普通成员发言默认仅内部可见，管理员可按成员或单条消息授权外发。", displayName, usernameLabel(username), m.Chat.ID)
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
	admin := false
	if m.From != nil {
		admin, err = h.telegram.IsChatAdministrator(ctx, h.supportGroupID, m.From.ID)
		if err != nil {
			return fmt.Errorf("check staff administrator status: %w", err)
		}
	}
	visible := admin
	if !visible && m.From != nil {
		visible, err = h.store.StaffVisible(ctx, c.UserID, m.From.ID)
		if err != nil {
			return fmt.Errorf("check staff visibility: %w", err)
		}
	}
	if !visible {
		return h.store.SaveMessageLink(ctx, MessageLink{UserID: c.UserID, TopicMessageID: m.MessageID, Hidden: true, CreatedAt: time.Now().UTC()})
	}
	return h.publishStaffMessage(ctx, c, m, MessageLink{}, false)
}

func (h *Handler) publishStaffMessage(ctx context.Context, c Conversation, m *telegram.Message, previous MessageLink, replace bool) error {
	var link MessageLink
	createdAt := time.Now().UTC()
	if c.Channel == "web" {
		messageID, err := newWebMessageID()
		if err != nil {
			return err
		}
		content := strings.TrimSpace(m.Text)
		if content == "" {
			content = strings.TrimSpace(m.Caption)
		}
		if content == "" {
			content = "[非文本消息]"
		}
		staffID, staffName := int64(0), "客服"
		if m.From != nil {
			staffID, staffName = m.From.ID, displayName(m.From)
		}
		replyToMessageID := ""
		if m.ReplyToMessage != nil {
			replyLink, found, lookupErr := h.store.MessageLinkByTopic(ctx, m.ReplyToMessage.MessageID)
			if lookupErr != nil {
				return fmt.Errorf("find replied web message: %w", lookupErr)
			}
			if found && replyLink.UserID == c.UserID {
				replyToMessageID = replyLink.WebMessageID
			}
		}
		message, err := h.store.AddWebMessage(ctx, WebMessage{
			ID: messageID, ConversationID: c.PublicID, Direction: "staff", Text: content,
			ReplyToMessageID: replyToMessageID,
			StaffUserID:      staffID, StaffName: staffName, TopicMessageID: m.MessageID,
			Visible: true, CreatedAt: createdAt,
		})
		if err != nil {
			return fmt.Errorf("append web staff message: %w", err)
		}
		link = MessageLink{UserID: c.UserID, TopicMessageID: m.MessageID, WebMessageID: message.ID, CreatedAt: createdAt}
	} else {
		options := telegram.CopyOptions{ProtectContent: c.ProtectContent}
		if m.ReplyToMessage != nil {
			replyLink, linkFound, linkErr := h.store.MessageLinkByTopic(ctx, m.ReplyToMessage.MessageID)
			if linkErr != nil {
				return fmt.Errorf("find replied topic message: %w", linkErr)
			}
			if linkFound && replyLink.UserID == c.UserID {
				options.ReplyToMessageID = replyLink.UserMessageID
			}
		}
		userMessageID, err := h.telegram.CopyMessage(ctx, c.UserID, h.supportGroupID, m.MessageID, options)
		if err != nil {
			return fmt.Errorf("copy topic message %d to user %d: %w", m.MessageID, c.UserID, err)
		}
		link = MessageLink{UserID: c.UserID, UserMessageID: userMessageID, TopicMessageID: m.MessageID, CreatedAt: createdAt}
	}
	if replace {
		if err := h.store.DeleteMessageLink(ctx, previous); err != nil {
			return fmt.Errorf("replace hidden message mapping: %w", err)
		}
	}
	return h.store.SaveMessageLink(ctx, link)
}

func newWebMessageID() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate web message id: %w", err)
	}
	return "wm_" + hex.EncodeToString(raw), nil
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
