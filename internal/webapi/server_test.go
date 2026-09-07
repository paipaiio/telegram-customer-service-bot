package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"forumdesk/internal/bot"
)

type memoryStore struct {
	mu            sync.Mutex
	conversations map[string]bot.Conversation
	messages      []bot.WebMessage
	links         []bot.MessageLink
	lookupErr     error
	createErr     error
	addErr        error
	listErr       error
	linkErr       error
}

func newMemoryStore() *memoryStore {
	return &memoryStore{conversations: map[string]bot.Conversation{}}
}

func (s *memoryStore) ConversationByPublicID(_ context.Context, id string) (bot.Conversation, bool, error) {
	if s.lookupErr != nil {
		return bot.Conversation{}, false, s.lookupErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.conversations[id]
	return c, ok, nil
}

func (s *memoryStore) CreateConversation(_ context.Context, c bot.Conversation) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conversations[c.PublicID] = c
	return nil
}

func (s *memoryStore) AddWebMessage(_ context.Context, m bot.WebMessage) (bot.WebMessage, error) {
	if s.addErr != nil {
		return bot.WebMessage{}, s.addErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m.Sequence = int64(len(s.messages) + 1)
	s.messages = append(s.messages, m)
	return m, nil
}

func (s *memoryStore) WebMessagesAfter(_ context.Context, conversationID string, after int64, limit int) ([]bot.WebMessage, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []bot.WebMessage
	for _, message := range s.messages {
		if message.ConversationID == conversationID && message.Sequence > after && message.Visible {
			result = append(result, message)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func (s *memoryStore) WebMessageByID(_ context.Context, id string) (bot.WebMessage, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, message := range s.messages {
		if message.ID == id {
			return message, true, nil
		}
	}
	return bot.WebMessage{}, false, nil
}

func (s *memoryStore) SaveMessageLink(_ context.Context, link bot.MessageLink) error {
	if s.linkErr != nil {
		return s.linkErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links = append(s.links, link)
	return nil
}

type fakeTelegram struct {
	topicID int
	sent    []string
	replies []struct {
		text              string
		threadID, replyTo int
	}
	topicErr error
	sendErr  error
}

func (f *fakeTelegram) CreateForumTopic(context.Context, int64, string) (int, error) {
	if f.topicErr != nil {
		return 0, f.topicErr
	}
	return f.topicID, nil
}

func (f *fakeTelegram) SendMessage(_ context.Context, _ int64, text string, _ int) (int, error) {
	if f.sendErr != nil {
		return 0, f.sendErr
	}
	f.sent = append(f.sent, text)
	return 88 + len(f.sent), nil
}

func (f *fakeTelegram) SendReplyMessage(_ context.Context, _ int64, text string, threadID, replyTo int) (int, error) {
	if f.sendErr != nil {
		return 0, f.sendErr
	}
	f.replies = append(f.replies, struct {
		text              string
		threadID, replyTo int
	}{text, threadID, replyTo})
	return 188 + len(f.replies), nil
}

func TestUpstreamAndStorageFailuresReturnStableErrors(t *testing.T) {
	makeCreate := func(store *memoryStore, telegram *fakeTelegram) *httptest.ResponseRecorder {
		h := New(Config{APIKey: "key", SupportGroupID: -1001}, store, telegram)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/conversations", strings.NewReader(`{"visitor_name":"Ada"}`))
		r.Header.Set("Authorization", "Bearer key")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	w := makeCreate(newMemoryStore(), &fakeTelegram{topicErr: errors.New("telegram down")})
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "telegram_error") {
		t.Fatalf("topic failure status=%d body=%s", w.Code, w.Body.String())
	}
	w = makeCreate(&memoryStore{conversations: map[string]bot.Conversation{}, createErr: errors.New("duplicate")}, &fakeTelegram{topicID: 42})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "conversation_conflict") {
		t.Fatalf("create failure status=%d body=%s", w.Code, w.Body.String())
	}

	conversation := bot.Conversation{PublicID: "wc_test", Channel: "web", VisitorToken: tokenHash("visitor-secret"), TopicID: 42}
	for name, store := range map[string]*memoryStore{
		"lookup": {conversations: map[string]bot.Conversation{"wc_test": conversation}, lookupErr: errors.New("read")},
		"list":   {conversations: map[string]bot.Conversation{"wc_test": conversation}, listErr: errors.New("read")},
	} {
		h := New(Config{APIKey: "key", SupportGroupID: -1001}, store, &fakeTelegram{topicID: 42})
		r := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/wc_test/messages", nil)
		r.Header.Set("Authorization", "Bearer visitor-secret")
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "storage_error") {
			t.Fatalf("%s failure status=%d body=%s", name, w.Code, w.Body.String())
		}
	}

	for name, fixture := range map[string]struct {
		store    *memoryStore
		telegram *fakeTelegram
	}{
		"telegram": {newMemoryStore(), &fakeTelegram{sendErr: errors.New("send")}},
		"message":  {&memoryStore{conversations: map[string]bot.Conversation{}, addErr: errors.New("write")}, &fakeTelegram{}},
		"mapping":  {&memoryStore{conversations: map[string]bot.Conversation{}, linkErr: errors.New("write")}, &fakeTelegram{}},
	} {
		store, telegram := fixture.store, fixture.telegram
		store.conversations["wc_test"] = conversation
		h := New(Config{APIKey: "key", SupportGroupID: -1001}, store, telegram)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/wc_test/messages", strings.NewReader(`{"text":"hello"}`))
		r.Header.Set("Authorization", "Bearer visitor-secret")
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if name == "telegram" && w.Code != http.StatusBadGateway {
			t.Fatalf("%s failure status=%d body=%s", name, w.Code, w.Body.String())
		}
		if name != "telegram" && w.Code != http.StatusInternalServerError {
			t.Fatalf("%s failure status=%d body=%s", name, w.Code, w.Body.String())
		}
	}
}

func TestConversationAndMessageLifecycle(t *testing.T) {
	store, telegram := newMemoryStore(), &fakeTelegram{topicID: 42}
	h := New(Config{APIKey: "integration-secret", SupportGroupID: -1001, RateLimitPerMinute: 60}, store, telegram)

	unauthorized := httptest.NewRecorder()
	h.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/v1/conversations", strings.NewReader(`{"visitor_name":"Ada"}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.Code)
	}

	create := httptest.NewRequest(http.MethodPost, "/api/v1/conversations", strings.NewReader(`{"visitor_name":"Ada","external_id":"customer-7","page_url":"https://example.test/pricing"}`))
	create.Header.Set("Authorization", "Bearer integration-secret")
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	h.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var createBody struct {
		Data struct {
			ID           string `json:"id"`
			VisitorToken string `json:"visitor_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createBody); err != nil {
		t.Fatal(err)
	}
	if createBody.Data.ID == "" || createBody.Data.VisitorToken == "" || len(telegram.sent) != 1 {
		t.Fatalf("body=%s sent=%v", created.Body.String(), telegram.sent)
	}

	path := "/api/v1/conversations/" + createBody.Data.ID + "/messages"
	send := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"text":"I need pricing help"}`))
	send.Header.Set("Authorization", "Bearer "+createBody.Data.VisitorToken)
	send.Header.Set("Content-Type", "application/json")
	sent := httptest.NewRecorder()
	h.ServeHTTP(sent, send)
	if sent.Code != http.StatusCreated || len(telegram.sent) != 2 || len(store.links) != 1 {
		t.Fatalf("send status=%d body=%s telegram=%v links=%v", sent.Code, sent.Body.String(), telegram.sent, store.links)
	}
	var sentBody struct {
		Data bot.WebMessage `json:"data"`
	}
	if err := json.Unmarshal(sent.Body.Bytes(), &sentBody); err != nil {
		t.Fatal(err)
	}
	reply := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"text":"Here is more context","reply_to_message_id":"`+sentBody.Data.ID+`"}`))
	reply.Header.Set("Authorization", "Bearer "+createBody.Data.VisitorToken)
	replied := httptest.NewRecorder()
	h.ServeHTTP(replied, reply)
	if replied.Code != http.StatusCreated || len(telegram.replies) != 1 || telegram.replies[0].replyTo != store.messages[0].TopicMessageID {
		t.Fatalf("reply status=%d body=%s replies=%#v", replied.Code, replied.Body.String(), telegram.replies)
	}

	list := httptest.NewRequest(http.MethodGet, path+"?after=0&limit=20", nil)
	list.Header.Set("Authorization", "Bearer "+createBody.Data.VisitorToken)
	listed := httptest.NewRecorder()
	h.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "I need pricing help") {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
}

func TestReplyRejectsUnknownOrHiddenTarget(t *testing.T) {
	store, telegram := newMemoryStore(), &fakeTelegram{topicID: 42}
	store.conversations["wc_test"] = bot.Conversation{PublicID: "wc_test", Channel: "web", VisitorToken: tokenHash("visitor-secret"), TopicID: 42}
	store.messages = []bot.WebMessage{{ID: "wm_hidden", ConversationID: "wc_test", TopicMessageID: 50, Visible: false}}
	h := New(Config{APIKey: "key", SupportGroupID: -1001}, store, telegram)
	for _, target := range []string{"missing", "wm_hidden"} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/wc_test/messages", strings.NewReader(`{"text":"reply","reply_to_message_id":"`+target+`"}`))
		r.Header.Set("Authorization", "Bearer visitor-secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnprocessableEntity || len(telegram.replies) != 0 {
			t.Fatalf("target=%s status=%d body=%s", target, w.Code, w.Body.String())
		}
	}
}

func TestMessageValidationCORSAndSecurityHeaders(t *testing.T) {
	store, telegram := newMemoryStore(), &fakeTelegram{topicID: 42}
	h := New(Config{APIKey: "key", SupportGroupID: -1001, AllowedOrigins: []string{"https://site.test"}, RateLimitPerMinute: 60}, store, telegram)

	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/conversations", nil)
	preflight.Header.Set("Origin", "https://site.test")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, preflight)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "https://site.test" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("status=%d headers=%v", response.Code, response.Header())
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations", strings.NewReader(`{"visitor_name":""}`))
	request.Header.Set("Authorization", "Bearer key")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "validation_error") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHealthVisitorAuthorizationAndQueryValidation(t *testing.T) {
	store, telegram := newMemoryStore(), &fakeTelegram{topicID: 42}
	store.conversations["wc_test"] = bot.Conversation{PublicID: "wc_test", Channel: "web", VisitorToken: tokenHash("visitor-secret")}
	h := New(Config{APIKey: "integration-secret", SupportGroupID: -1001}, store, telegram)

	health := httptest.NewRecorder()
	h.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health=%d", health.Code)
	}

	unauthorized := httptest.NewRecorder()
	h.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/conversations/wc_test/messages", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized=%d", unauthorized.Code)
	}

	for _, path := range []string{
		"/api/v1/conversations/wc_test/messages?after=-1",
		"/api/v1/conversations/wc_test/messages?limit=101",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer visitor-secret")
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}

	empty := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/wc_test/messages", strings.NewReader(`{"text":" "}`))
	empty.Header.Set("Authorization", "Bearer visitor-secret")
	emptyResponse := httptest.NewRecorder()
	h.ServeHTTP(emptyResponse, empty)
	if emptyResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty status=%d body=%s", emptyResponse.Code, emptyResponse.Body.String())
	}
}

func TestConversationValidationAndRateLimit(t *testing.T) {
	store, telegram := newMemoryStore(), &fakeTelegram{topicID: 42}
	h := New(Config{APIKey: "integration-secret", SupportGroupID: -1001, RateLimitPerMinute: 1}, store, telegram)

	invalid := httptest.NewRequest(http.MethodPost, "/api/v1/conversations", strings.NewReader(`{"visitor_name":"Ada","page_url":"file:///tmp/x"}`))
	invalid.Header.Set("Authorization", "Bearer integration-secret")
	invalidResponse := httptest.NewRecorder()
	h.ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid status=%d body=%s", invalidResponse.Code, invalidResponse.Body.String())
	}

	limited := httptest.NewRequest(http.MethodPost, "/api/v1/conversations", strings.NewReader(`{"visitor_name":"Ada"}`))
	limited.Header.Set("Authorization", "Bearer integration-secret")
	limitedResponse := httptest.NewRecorder()
	h.ServeHTTP(limitedResponse, limited)
	if limitedResponse.Code != http.StatusTooManyRequests || limitedResponse.Header().Get("Retry-After") != "60" {
		t.Fatalf("limited status=%d headers=%v body=%s", limitedResponse.Code, limitedResponse.Header(), limitedResponse.Body.String())
	}
}
