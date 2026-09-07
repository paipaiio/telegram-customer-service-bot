package webapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"forumdesk/internal/bot"
)

const maxRequestBody = 16 << 10

type Store interface {
	ConversationByPublicID(context.Context, string) (bot.Conversation, bool, error)
	CreateConversation(context.Context, bot.Conversation) error
	AddWebMessage(context.Context, bot.WebMessage) (bot.WebMessage, error)
	WebMessagesAfter(context.Context, string, int64, int) ([]bot.WebMessage, error)
	WebMessageByID(context.Context, string) (bot.WebMessage, bool, error)
	SaveMessageLink(context.Context, bot.MessageLink) error
}

type Telegram interface {
	CreateForumTopic(context.Context, int64, string) (int, error)
	SendMessage(context.Context, int64, string, int) (int, error)
	SendReplyMessage(context.Context, int64, string, int, int) (int, error)
}

type Config struct {
	APIKey             string
	SupportGroupID     int64
	AllowedOrigins     []string
	RateLimitPerMinute int
}

type handler struct {
	config   Config
	store    Store
	telegram Telegram
	limiter  *rateLimiter
}

type rateWindow struct {
	started time.Time
	count   int
}

type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	windows map[string]rateWindow
}

func New(config Config, store Store, telegram Telegram) http.Handler {
	if config.RateLimitPerMinute <= 0 {
		config.RateLimitPerMinute = 60
	}
	h := &handler{
		config: config, store: store, telegram: telegram,
		limiter: &rateLimiter{limit: config.RateLimitPerMinute, windows: map[string]rateWindow{}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("POST /api/v1/conversations", h.createConversation)
	mux.HandleFunc("POST /api/v1/conversations/{conversation_id}/messages", h.createMessage)
	mux.HandleFunc("GET /api/v1/conversations/{conversation_id}/messages", h.listMessages)
	return h.middleware(mux)
}

func (h *handler) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		if origin := r.Header.Get("Origin"); origin != "" && h.originAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
}

func (h *handler) createConversation(w http.ResponseWriter, r *http.Request) {
	if !secureEqual(bearerToken(r), h.config.APIKey) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid integration API key")
		return
	}
	if !h.limiter.allow("integration:" + bearerToken(r)) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Rate limit exceeded")
		return
	}
	var input struct {
		VisitorName string `json:"visitor_name"`
		Email       string `json:"email"`
		ExternalID  string `json:"external_id"`
		PageURL     string `json:"page_url"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	input.VisitorName = strings.TrimSpace(input.VisitorName)
	input.Email = strings.TrimSpace(input.Email)
	input.ExternalID = strings.TrimSpace(input.ExternalID)
	input.PageURL = strings.TrimSpace(input.PageURL)
	if input.VisitorName == "" || len([]rune(input.VisitorName)) > 128 || len(input.Email) > 254 || len(input.ExternalID) > 128 || !validPageURL(input.PageURL) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "visitor_name or metadata is invalid")
		return
	}
	publicID, err := randomValue("wc_", 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	visitorToken, err := randomValue("wv_", 32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	topicName := "🌐 " + input.VisitorName + " · " + publicID
	if len([]rune(topicName)) > 128 {
		topicName = string([]rune(topicName)[:128])
	}
	topicID, err := h.telegram.CreateForumTopic(r.Context(), h.config.SupportGroupID, topicName)
	if err != nil {
		writeError(w, http.StatusBadGateway, "telegram_error", "Telegram topic creation failed")
		return
	}
	now := time.Now().UTC()
	c := bot.Conversation{
		UserID: internalID(publicID), TopicID: topicID, PublicID: publicID, Channel: "web",
		VisitorToken: tokenHash(visitorToken), VisitorEmail: input.Email, ExternalID: input.ExternalID,
		PageURL: input.PageURL, DisplayName: input.VisitorName, CreatedAt: now,
	}
	if err := h.store.CreateConversation(r.Context(), c); err != nil {
		writeError(w, http.StatusConflict, "conversation_conflict", "Conversation could not be created")
		return
	}
	card := fmt.Sprintf("🌐 网页客户资料\n名称：%s\n邮箱：%s\n外部 ID：%s\n来源页：%s\n会话 ID：%s", input.VisitorName, valueOrDash(input.Email), valueOrDash(input.ExternalID), valueOrDash(input.PageURL), publicID)
	_, _ = h.telegram.SendMessage(r.Context(), h.config.SupportGroupID, card, topicID)
	w.Header().Set("Location", "/api/v1/conversations/"+publicID)
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"id": publicID, "visitor_token": visitorToken, "status": "open", "created_at": now,
	}})
}

func (h *handler) createMessage(w http.ResponseWriter, r *http.Request) {
	c, ok := h.authorizeVisitor(w, r)
	if !ok {
		return
	}
	if !h.limiter.allow("visitor:" + c.PublicID) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Rate limit exceeded")
		return
	}
	var input struct {
		Text             string `json:"text"`
		ReplyToMessageID string `json:"reply_to_message_id"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	input.Text = strings.TrimSpace(input.Text)
	input.ReplyToMessageID = strings.TrimSpace(input.ReplyToMessageID)
	if input.Text == "" || len([]rune(input.Text)) > 4000 {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "text must contain 1 to 4000 characters")
		return
	}
	replyTopicMessageID := 0
	if input.ReplyToMessageID != "" {
		target, found, err := h.store.WebMessageByID(r.Context(), input.ReplyToMessageID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage_error", "Reply target could not be loaded")
			return
		}
		if !found || target.ConversationID != c.PublicID || !target.Visible || target.TopicMessageID == 0 {
			writeError(w, http.StatusUnprocessableEntity, "invalid_reply_target", "reply_to_message_id is not a visible message in this conversation")
			return
		}
		replyTopicMessageID = target.TopicMessageID
	}
	var (
		topicMessageID int
		err            error
	)
	if replyTopicMessageID != 0 {
		topicMessageID, err = h.telegram.SendReplyMessage(r.Context(), h.config.SupportGroupID, "🌐 客户：\n"+input.Text, c.TopicID, replyTopicMessageID)
	} else {
		topicMessageID, err = h.telegram.SendMessage(r.Context(), h.config.SupportGroupID, "🌐 客户：\n"+input.Text, c.TopicID)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "telegram_error", "Message delivery to Telegram failed")
		return
	}
	messageID, err := randomValue("wm_", 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal error")
		return
	}
	message, err := h.store.AddWebMessage(r.Context(), bot.WebMessage{
		ID: messageID, ConversationID: c.PublicID, Direction: "customer", Text: input.Text,
		ReplyToMessageID: input.ReplyToMessageID,
		TopicMessageID:   topicMessageID, Visible: true, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Message could not be stored")
		return
	}
	if err := h.store.SaveMessageLink(r.Context(), bot.MessageLink{UserID: c.UserID, TopicMessageID: topicMessageID, WebMessageID: message.ID, CreatedAt: message.CreatedAt}); err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Message mapping could not be stored")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": message})
}

func (h *handler) listMessages(w http.ResponseWriter, r *http.Request) {
	c, ok := h.authorizeVisitor(w, r)
	if !ok {
		return
	}
	after, err := strconv.ParseInt(defaultValue(r.URL.Query().Get("after"), "0"), 10, 64)
	if err != nil || after < 0 {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "after must be a non-negative integer")
		return
	}
	limit, err := strconv.Atoi(defaultValue(r.URL.Query().Get("limit"), "50"))
	if err != nil || limit < 1 || limit > 100 {
		writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100")
		return
	}
	messages, err := h.store.WebMessagesAfter(r.Context(), c.PublicID, after, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Messages could not be loaded")
		return
	}
	next := after
	if len(messages) > 0 {
		next = messages[len(messages)-1].Sequence
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": messages, "meta": map[string]any{"next_cursor": next}})
}

func (h *handler) authorizeVisitor(w http.ResponseWriter, r *http.Request) (bot.Conversation, bool) {
	id := r.PathValue("conversation_id")
	c, found, err := h.store.ConversationByPublicID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Conversation could not be loaded")
		return bot.Conversation{}, false
	}
	if !found || c.Channel != "web" || !secureEqual(tokenHash(bearerToken(r)), c.VisitorToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid conversation credentials")
		return bot.Conversation{}, false
	}
	return c, true
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	w, found := l.windows[key]
	if !found || now.Sub(w.started) >= time.Minute {
		l.windows[key] = rateWindow{started: now, count: 1}
		return true
	}
	if w.count >= l.limit {
		return false
	}
	w.count++
	l.windows[key] = w
	return true
}

func (h *handler) originAllowed(origin string) bool {
	for _, allowed := range h.config.AllowedOrigins {
		if subtle.ConstantTimeCompare([]byte(origin), []byte(allowed)) == 1 {
			return true
		}
	}
	return false
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON request body")
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) < 8 || !strings.EqualFold(value[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(value[7:])
}

func tokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func secureEqual(a, b string) bool {
	return a != "" && b != "" && len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func randomValue(prefix string, size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}

func internalID(publicID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(publicID))
	value := int64(h.Sum64() & ((1 << 63) - 1))
	if value == 0 {
		value = 1
	}
	return -value
}

func validPageURL(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := url.ParseRequestURI(value)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != ""
}

func valueOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
