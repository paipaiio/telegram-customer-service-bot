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
	if err := reopened.DeleteMessageLink(context.Background(), 101, 1); err != nil {
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
