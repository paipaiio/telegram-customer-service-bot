package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testClient(t *testing.T, result any) (*Client, *map[string]any, *string) {
	t.Helper()
	gotPayload := map[string]any{}
	gotPath := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
	}))
	t.Cleanup(server.Close)
	return &Client{baseURL: server.URL + "/", http: server.Client()}, &gotPayload, &gotPath
}

func TestCreateForumTopic(t *testing.T) {
	c, payload, path := testClient(t, map[string]any{"message_thread_id": 42, "name": "Ada"})
	id, err := c.CreateForumTopic(context.Background(), -1001, "Ada")
	if err != nil || id != 42 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	if *path != "/createForumTopic" || (*payload)["chat_id"] != float64(-1001) {
		t.Fatalf("path=%s payload=%v", *path, *payload)
	}
}

func TestCopyMessageIncludesTopicOnlyWhenProvided(t *testing.T) {
	c, payload, _ := testClient(t, map[string]any{"message_id": 99})
	id, err := c.CopyMessage(context.Background(), -1001, 7, 8, CopyOptions{ThreadID: 42, ReplyToMessageID: 41, ProtectContent: true})
	if err != nil || id != 99 || (*payload)["message_thread_id"] != float64(42) {
		t.Fatalf("id=%d payload=%v err=%v", id, *payload, err)
	}
	if (*payload)["protect_content"] != true || (*payload)["reply_parameters"].(map[string]any)["message_id"] != float64(41) {
		t.Fatalf("payload=%v", *payload)
	}

	c, payload, _ = testClient(t, map[string]any{"message_id": 100})
	_, err = c.CopyMessage(context.Background(), 7, -1001, 9, CopyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := (*payload)["message_thread_id"]; ok {
		t.Fatalf("unexpected thread: %v", *payload)
	}
	if _, ok := (*payload)["protect_content"]; ok {
		t.Fatalf("unexpected protection: %v", *payload)
	}
}

func TestGetUpdatesAndProfilePhoto(t *testing.T) {
	c, payload, path := testClient(t, []map[string]any{{"update_id": 5, "message": map[string]any{"message_id": 1, "chat": map[string]any{"id": 7, "type": "private"}}}})
	updates, err := c.GetUpdates(context.Background(), 5)
	if err != nil || len(updates) != 1 || updates[0].UpdateID != 5 {
		t.Fatalf("updates=%v err=%v", updates, err)
	}
	if *path != "/getUpdates" || (*payload)["offset"] != float64(5) {
		t.Fatalf("path=%s payload=%v", *path, *payload)
	}
	allowed := (*payload)["allowed_updates"].([]any)
	if len(allowed) != 2 || allowed[1] != "edited_message" {
		t.Fatalf("allowed_updates=%v", allowed)
	}

	c, _, _ = testClient(t, map[string]any{"total_count": 1, "photos": [][]map[string]any{{{"file_id": "small"}, {"file_id": "large"}}}})
	fileID, ok, err := c.FirstProfilePhoto(context.Background(), 7)
	if err != nil || !ok || fileID != "large" {
		t.Fatalf("file=%q ok=%v err=%v", fileID, ok, err)
	}
}

func TestEditAndDeleteMessages(t *testing.T) {
	c, payload, path := testClient(t, true)
	if err := c.EditMessageText(context.Background(), 7, 8, "changed"); err != nil {
		t.Fatal(err)
	}
	if *path != "/editMessageText" || (*payload)["text"] != "changed" {
		t.Fatalf("path=%s payload=%v", *path, *payload)
	}

	c, payload, path = testClient(t, true)
	if err := c.EditMessageCaption(context.Background(), 7, 8, "caption"); err != nil {
		t.Fatal(err)
	}
	if *path != "/editMessageCaption" || (*payload)["caption"] != "caption" {
		t.Fatalf("path=%s payload=%v", *path, *payload)
	}

	c, payload, path = testClient(t, true)
	if err := c.DeleteMessage(context.Background(), 7, 8); err != nil {
		t.Fatal(err)
	}
	if *path != "/deleteMessage" || (*payload)["message_id"] != float64(8) {
		t.Fatalf("path=%s payload=%v", *path, *payload)
	}
}

func TestSendReplyMessage(t *testing.T) {
	c, payload, path := testClient(t, map[string]any{"message_id": 13, "chat": map[string]any{"id": -1001, "type": "supergroup"}})
	id, err := c.SendReplyMessage(context.Background(), -1001, "修改后", 42, 99)
	if err != nil || id != 13 || *path != "/sendMessage" {
		t.Fatalf("id=%d path=%s err=%v", id, *path, err)
	}
	reply := (*payload)["reply_parameters"].(map[string]any)
	if reply["message_id"] != float64(99) || (*payload)["message_thread_id"] != float64(42) {
		t.Fatalf("payload=%v", *payload)
	}
}

func TestSendMessageAndPhoto(t *testing.T) {
	c, payload, path := testClient(t, map[string]any{"message_id": 11, "chat": map[string]any{"id": -1001, "type": "supergroup"}})
	id, err := c.SendMessage(context.Background(), -1001, "card", 42)
	if err != nil || id != 11 || *path != "/sendMessage" || (*payload)["message_thread_id"] != float64(42) {
		t.Fatalf("id=%d path=%s payload=%v err=%v", id, *path, *payload, err)
	}

	c, payload, path = testClient(t, map[string]any{"message_id": 12, "chat": map[string]any{"id": -1001, "type": "supergroup"}})
	id, err = c.SendPhotoByFileID(context.Background(), -1001, "file", "caption", 42)
	if err != nil || id != 12 || *path != "/sendPhoto" || (*payload)["photo"] != "file" {
		t.Fatalf("id=%d path=%s payload=%v err=%v", id, *path, *payload, err)
	}
}

func TestAdminCommandMenu(t *testing.T) {
	c, payload, path := testClient(t, true)
	if err := c.SetAdminCommands(context.Background(), -1001); err != nil {
		t.Fatal(err)
	}
	if *path != "/setMyCommands" {
		t.Fatalf("path=%s", *path)
	}
	scope := (*payload)["scope"].(map[string]any)
	commands := (*payload)["commands"].([]any)
	if scope["type"] != "chat_administrators" || scope["chat_id"] != float64(-1001) || len(commands) != 10 {
		t.Fatalf("payload=%v", *payload)
	}
	if commands[3].(map[string]any)["command"] != "clear_user" {
		t.Fatalf("commands=%v", commands)
	}
}

func TestIsChatAdministrator(t *testing.T) {
	c, payload, path := testClient(t, map[string]any{"status": "administrator"})
	admin, err := c.IsChatAdministrator(context.Background(), -1001, 7001)
	if err != nil || !admin || *path != "/getChatMember" || (*payload)["user_id"] != float64(7001) {
		t.Fatalf("admin=%v path=%s payload=%v err=%v", admin, *path, *payload, err)
	}
	c, _, _ = testClient(t, map[string]any{"status": "member"})
	admin, err = c.IsChatAdministrator(context.Background(), -1001, 7001)
	if err != nil || admin {
		t.Fatalf("admin=%v err=%v", admin, err)
	}
}

func TestPrivateCommandMenu(t *testing.T) {
	c, payload, path := testClient(t, true)
	if err := c.SetPrivateCommands(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *path != "/setMyCommands" {
		t.Fatalf("path=%s", *path)
	}
	scope := (*payload)["scope"].(map[string]any)
	commands := (*payload)["commands"].([]any)
	if scope["type"] != "all_private_chats" || len(commands) != 2 {
		t.Fatalf("payload=%v", *payload)
	}
}

func TestDeleteMessagesAndForumTopic(t *testing.T) {
	c, payload, path := testClient(t, true)
	if err := c.DeleteMessages(context.Background(), 7, []int{1, 2}); err != nil {
		t.Fatal(err)
	}
	if *path != "/deleteMessages" || len((*payload)["message_ids"].([]any)) != 2 {
		t.Fatalf("path=%s payload=%v", *path, *payload)
	}
	c, payload, path = testClient(t, true)
	if err := c.DeleteForumTopic(context.Background(), -1001, 42); err != nil {
		t.Fatal(err)
	}
	if *path != "/deleteForumTopic" || (*payload)["message_thread_id"] != float64(42) {
		t.Fatalf("path=%s payload=%v", *path, *payload)
	}
}

func TestAPIFailureReturnsDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"description":"bad request"}`))
	}))
	defer server.Close()
	c := &Client{baseURL: server.URL + "/", http: server.Client()}
	if _, err := c.CreateForumTopic(context.Background(), -1, "Ada"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewClientAndNoProfilePhoto(t *testing.T) {
	c := NewClient("secret")
	if c.baseURL != "https://api.telegram.org/botsecret/" {
		t.Fatalf("base URL=%q", c.baseURL)
	}

	c, _, _ = testClient(t, map[string]any{"total_count": 0, "photos": []any{}})
	fileID, ok, err := c.FirstProfilePhoto(context.Background(), 7)
	if err != nil || ok || fileID != "" {
		t.Fatalf("file=%q ok=%v err=%v", fileID, ok, err)
	}
}

func TestCallRejectsMalformedResponseAndUnencodablePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not-json")) }))
	defer server.Close()
	c := &Client{baseURL: server.URL + "/", http: server.Client()}
	if _, err := c.CreateForumTopic(context.Background(), -1, "Ada"); err == nil {
		t.Fatal("expected decode error")
	}
	if err := c.call(context.Background(), "test", make(chan int), nil); err == nil {
		t.Fatal("expected encode error")
	}
}
