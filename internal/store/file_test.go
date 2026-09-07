package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"forumdesk/internal/bot"
)

func TestFileStorePersistsConversationsAndLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "data.json")
	s, err := OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c := bot.Conversation{UserID: 101, TopicID: 202, DisplayName: "Ada", Username: "ada"}
	if err := s.CreateConversation(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	link := bot.MessageLink{UserID: 101, UserMessageID: 1, TopicMessageID: 2}
	if err := s.SaveMessageLink(context.Background(), link); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := reopened.ConversationByUser(context.Background(), 101)
	if err != nil || !ok || got != c {
		t.Fatalf("got=%#v ok=%v err=%v", got, ok, err)
	}
	got, ok, err = reopened.ConversationByTopic(context.Background(), 202)
	if err != nil || !ok || got != c {
		t.Fatalf("topic lookup got=%#v ok=%v err=%v", got, ok, err)
	}
	gotLink, ok, err := reopened.MessageLinkByUser(context.Background(), 101, 1)
	if err != nil || !ok || gotLink != link {
		t.Fatalf("user link=%#v ok=%v err=%v", gotLink, ok, err)
	}
	gotLink, ok, err = reopened.MessageLinkByTopic(context.Background(), 2)
	if err != nil || !ok || gotLink != link {
		t.Fatalf("topic link=%#v ok=%v err=%v", gotLink, ok, err)
	}
	if err := reopened.DeleteMessageLink(context.Background(), link); err != nil {
		t.Fatal(err)
	}
	again, err := OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := again.MessageLinkByUser(context.Background(), 101, 1); ok {
		t.Fatal("deleted link persisted")
	}
	updated, err := again.SetConversationProtection(context.Background(), 101, true)
	if err != nil || !updated.ProtectContent {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	finalStore, err := OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	finalConversation, ok, err := finalStore.ConversationByUser(context.Background(), 101)
	if err != nil || !ok || !finalConversation.ProtectContent {
		t.Fatalf("conversation=%#v ok=%v err=%v", finalConversation, ok, err)
	}
	links, err := finalStore.MessageLinksByUser(context.Background(), 101)
	if err != nil || len(links) != 0 {
		t.Fatalf("links=%v err=%v", links, err)
	}
	if err := finalStore.DeleteConversation(context.Background(), 101); err != nil {
		t.Fatal(err)
	}
	afterDelete, err := OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := afterDelete.ConversationByUser(context.Background(), 101); ok {
		t.Fatal("conversation still exists")
	}
}

func TestOpenFileRejectsCorruptJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFile(path); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestTopicLookupMissAndDuplicateTopic(t *testing.T) {
	s, err := OpenFile(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.ConversationByTopic(context.Background(), 404); err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if err := s.CreateConversation(context.Background(), bot.Conversation{UserID: 1, TopicID: 2}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateConversation(context.Background(), bot.Conversation{UserID: 3, TopicID: 2}); err == nil {
		t.Fatal("expected duplicate topic error")
	}
}

func TestFileStoreRejectsDuplicateMappings(t *testing.T) {
	s, err := OpenFile(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := bot.Conversation{UserID: 1, TopicID: 2}
	if err := s.CreateConversation(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateConversation(context.Background(), c); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestReplaceMessageLinksByUserPreservesConversationAndOtherUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	s, err := OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	conversation := bot.Conversation{UserID: 101, TopicID: 202, DisplayName: "Ada"}
	if err := s.CreateConversation(context.Background(), conversation); err != nil {
		t.Fatal(err)
	}
	for _, link := range []bot.MessageLink{
		{UserID: 101, UserMessageID: 1, TopicMessageID: 11},
		{UserID: 101, UserMessageID: 2, TopicMessageID: 12},
		{UserID: 303, UserMessageID: 3, TopicMessageID: 13},
	} {
		if err := s.SaveMessageLink(context.Background(), link); err != nil {
			t.Fatal(err)
		}
	}
	replacement := []bot.MessageLink{{UserID: 101, TopicMessageID: 11}, {UserID: 101, TopicMessageID: 12}}
	if err := s.ReplaceMessageLinksByUser(context.Background(), 101, replacement); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceMessageLinksByUser(context.Background(), 101, []bot.MessageLink{{UserID: 999}}); err == nil {
		t.Fatal("expected mismatched user error")
	}
	reopened, err := OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, found, _ := reopened.ConversationByUser(context.Background(), 101); !found || got != conversation {
		t.Fatalf("conversation=%#v found=%v", got, found)
	}
	links, err := reopened.MessageLinksByUser(context.Background(), 101)
	if err != nil || len(links) != 2 || links[0].UserMessageID != 0 || links[1].UserMessageID != 0 {
		t.Fatalf("links=%#v err=%v", links, err)
	}
	other, err := reopened.MessageLinksByUser(context.Background(), 303)
	if err != nil || len(other) != 1 || other[0].UserMessageID != 3 {
		t.Fatalf("other=%#v err=%v", other, err)
	}
}

func TestWebMessagesAndStaffVisibilityPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	s, err := OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	conversation := bot.Conversation{UserID: -101, TopicID: 202, PublicID: "wc_test", Channel: "web", DisplayName: "Ada"}
	if err := s.CreateConversation(context.Background(), conversation); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStaffVisibility(context.Background(), conversation.UserID, 7001, true); err != nil {
		t.Fatal(err)
	}
	first, err := s.AddWebMessage(context.Background(), bot.WebMessage{ID: "wm_1", ConversationID: "wc_test", Text: "hello", Visible: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AddWebMessage(context.Background(), bot.WebMessage{ID: "wm_2", ConversationID: "wc_test", Text: "internal", Visible: false})
	if err != nil {
		t.Fatal(err)
	}
	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("sequences=%d,%d", first.Sequence, second.Sequence)
	}
	if err := s.SetWebMessageVisible(context.Background(), "wm_2", true); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := reopened.ConversationByPublicID(context.Background(), "wc_test")
	if err != nil || !found || got != conversation {
		t.Fatalf("conversation=%#v found=%v err=%v", got, found, err)
	}
	allowed, err := reopened.StaffVisible(context.Background(), conversation.UserID, 7001)
	if err != nil || !allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	visibleStaff, err := reopened.VisibleStaff(context.Background(), conversation.UserID)
	if err != nil || len(visibleStaff) != 1 || visibleStaff[0] != 7001 {
		t.Fatalf("visible staff=%v err=%v", visibleStaff, err)
	}
	if err := reopened.SetStaffVisibility(context.Background(), conversation.UserID, 7001, false); err != nil {
		t.Fatal(err)
	}
	if allowed, err := reopened.StaffVisible(context.Background(), conversation.UserID, 7001); err != nil || allowed {
		t.Fatalf("revoked staff allowed=%v err=%v", allowed, err)
	}
	messages, err := reopened.WebMessagesAfter(context.Background(), "wc_test", 1, 10)
	if err != nil || len(messages) != 1 || messages[0].ID != "wm_2" || !messages[0].Visible {
		t.Fatalf("messages=%#v err=%v", messages, err)
	}
	message, found, err := reopened.WebMessageByID(context.Background(), "wm_2")
	if err != nil || !found || message.ID != "wm_2" {
		t.Fatalf("message=%#v found=%v err=%v", message, found, err)
	}
	if _, found, err := reopened.WebMessageByID(context.Background(), "missing"); err != nil || found {
		t.Fatalf("missing found=%v err=%v", found, err)
	}
	if err := reopened.SetWebMessageVisible(context.Background(), "missing", true); err == nil {
		t.Fatal("expected missing web message error")
	}
	if err := reopened.SetStaffVisibility(context.Background(), 404, 7001, true); err == nil {
		t.Fatal("expected missing conversation error")
	}
}
