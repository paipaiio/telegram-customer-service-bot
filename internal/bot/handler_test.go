package bot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"forumdesk/internal/telegram"
)

type fakeStore struct {
	byUser              map[int64]Conversation
	byTopic             map[int64]Conversation
	saved               []MessageLink
	webMessages         []WebMessage
	staffVisibility     map[int64]map[int64]bool
	createErr           error
	deletedConversation int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{byUser: map[int64]Conversation{}, byTopic: map[int64]Conversation{}, staffVisibility: map[int64]map[int64]bool{}}
}

func (s *fakeStore) ConversationByUser(_ context.Context, id int64) (Conversation, bool, error) {
	c, ok := s.byUser[id]
	return c, ok, nil
}
func (s *fakeStore) ConversationByTopic(_ context.Context, id int64) (Conversation, bool, error) {
	c, ok := s.byTopic[id]
	return c, ok, nil
}
func (s *fakeStore) CreateConversation(_ context.Context, c Conversation) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.byUser[c.UserID], s.byTopic[int64(c.TopicID)] = c, c
	return nil
}
func (s *fakeStore) SaveMessageLink(_ context.Context, l MessageLink) error {
	s.saved = append(s.saved, l)
	return nil
}
func (s *fakeStore) MessageLinkByUser(_ context.Context, userID int64, messageID int) (MessageLink, bool, error) {
	for _, link := range s.saved {
		if link.UserID == userID && link.UserMessageID == messageID {
			return link, true, nil
		}
	}
	return MessageLink{}, false, nil
}
func (s *fakeStore) MessageLinkByTopic(_ context.Context, messageID int) (MessageLink, bool, error) {
	for _, link := range s.saved {
		if link.TopicMessageID == messageID {
			return link, true, nil
		}
	}
	return MessageLink{}, false, nil
}
func (s *fakeStore) DeleteMessageLink(_ context.Context, target MessageLink) error {
	for i, link := range s.saved {
		if link.UserID == target.UserID && ((target.UserMessageID != 0 && link.UserMessageID == target.UserMessageID) || (target.TopicMessageID != 0 && link.TopicMessageID == target.TopicMessageID)) {
			s.saved = append(s.saved[:i], s.saved[i+1:]...)
			break
		}
	}
	return nil
}
func (s *fakeStore) SetConversationProtection(_ context.Context, userID int64, enabled bool) (Conversation, error) {
	c, ok := s.byUser[userID]
	if !ok {
		return Conversation{}, errors.New("conversation not found")
	}
	c.ProtectContent = enabled
	s.byUser[userID], s.byTopic[int64(c.TopicID)] = c, c
	return c, nil
}
func (s *fakeStore) MessageLinksByUser(_ context.Context, userID int64) ([]MessageLink, error) {
	var links []MessageLink
	for _, link := range s.saved {
		if link.UserID == userID {
			links = append(links, link)
		}
	}
	return links, nil
}
func (s *fakeStore) DeleteConversation(_ context.Context, userID int64) error {
	c := s.byUser[userID]
	delete(s.byUser, userID)
	delete(s.byTopic, int64(c.TopicID))
	s.deletedConversation = userID
	kept := s.saved[:0]
	for _, link := range s.saved {
		if link.UserID != userID {
			kept = append(kept, link)
		}
	}
	s.saved = kept
	return nil
}
func (s *fakeStore) ReplaceMessageLinksByUser(_ context.Context, userID int64, links []MessageLink) error {
	kept := s.saved[:0]
	for _, link := range s.saved {
		if link.UserID != userID {
			kept = append(kept, link)
		}
	}
	s.saved = append(kept, links...)
	return nil
}
func (s *fakeStore) SetStaffVisibility(_ context.Context, userID, staffID int64, visible bool) error {
	if s.staffVisibility[userID] == nil {
		s.staffVisibility[userID] = map[int64]bool{}
	}
	s.staffVisibility[userID][staffID] = visible
	return nil
}
func (s *fakeStore) StaffVisible(_ context.Context, userID, staffID int64) (bool, error) {
	return s.staffVisibility[userID][staffID], nil
}
func (s *fakeStore) VisibleStaff(_ context.Context, userID int64) ([]int64, error) {
	var result []int64
	for staffID, visible := range s.staffVisibility[userID] {
		if visible {
			result = append(result, staffID)
		}
	}
	return result, nil
}
func (s *fakeStore) AddWebMessage(_ context.Context, message WebMessage) (WebMessage, error) {
	message.Sequence = int64(len(s.webMessages) + 1)
	s.webMessages = append(s.webMessages, message)
	return message, nil
}
func (s *fakeStore) WebMessageByID(_ context.Context, id string) (WebMessage, bool, error) {
	for _, message := range s.webMessages {
		if message.ID == id {
			return message, true, nil
		}
	}
	return WebMessage{}, false, nil
}
func (s *fakeStore) SetWebMessageVisible(_ context.Context, id string, visible bool) error {
	for i := range s.webMessages {
		if s.webMessages[i].ID == id {
			s.webMessages[i].Visible = visible
		}
	}
	return nil
}

type copyCall struct {
	chatID, fromChatID int64
	messageID          int
	options            telegram.CopyOptions
}
type fakeTelegram struct {
	topicID        int
	copyID         int
	copyCalls      []copyCall
	texts          []string
	photos         []string
	profileFile    string
	edits          []editCall
	deletes        []deleteCall
	replies        []replyCall
	deleteErr      map[deleteCall]error
	deleteBatches  []deleteBatchCall
	deleteBatchErr error
	deletedTopics  []int
	nonAdmins      map[int64]bool
}
type editCall struct {
	kind      string
	chatID    int64
	messageID int
	content   string
}
type deleteCall struct {
	chatID    int64
	messageID int
}
type deleteBatchCall struct {
	chatID     int64
	messageIDs []int
}
type replyCall struct {
	chatID              int64
	messageID, threadID int
	text                string
}

func (f *fakeTelegram) CreateForumTopic(context.Context, int64, string) (int, error) {
	return f.topicID, nil
}
func (f *fakeTelegram) CopyMessage(_ context.Context, chatID, fromChatID int64, messageID int, options telegram.CopyOptions) (int, error) {
	f.copyCalls = append(f.copyCalls, copyCall{chatID, fromChatID, messageID, options})
	return f.copyID, nil
}
func (f *fakeTelegram) SendMessage(_ context.Context, _ int64, text string, _ int) (int, error) {
	f.texts = append(f.texts, text)
	return 77, nil
}
func (f *fakeTelegram) SendPhotoByFileID(_ context.Context, _ int64, fileID, _ string, _ int) (int, error) {
	f.photos = append(f.photos, fileID)
	return 78, nil
}
func (f *fakeTelegram) FirstProfilePhoto(context.Context, int64) (string, bool, error) {
	return f.profileFile, f.profileFile != "", nil
}
func (f *fakeTelegram) EditMessageText(_ context.Context, chatID int64, messageID int, text string) error {
	f.edits = append(f.edits, editCall{"text", chatID, messageID, text})
	return nil
}
func (f *fakeTelegram) EditMessageCaption(_ context.Context, chatID int64, messageID int, caption string) error {
	f.edits = append(f.edits, editCall{"caption", chatID, messageID, caption})
	return nil
}
func (f *fakeTelegram) DeleteMessage(_ context.Context, chatID int64, messageID int) error {
	call := deleteCall{chatID, messageID}
	f.deletes = append(f.deletes, call)
	return f.deleteErr[call]
}
func (f *fakeTelegram) SendReplyMessage(_ context.Context, chatID int64, text string, threadID, messageID int) (int, error) {
	f.replies = append(f.replies, replyCall{chatID, messageID, threadID, text})
	return 79, nil
}
func (f *fakeTelegram) DeleteMessages(_ context.Context, chatID int64, messageIDs []int) error {
	f.deleteBatches = append(f.deleteBatches, deleteBatchCall{chatID: chatID, messageIDs: append([]int(nil), messageIDs...)})
	return f.deleteBatchErr
}
func (f *fakeTelegram) DeleteForumTopic(_ context.Context, _ int64, topicID int) error {
	f.deletedTopics = append(f.deletedTopics, topicID)
	return nil
}
func (f *fakeTelegram) IsChatAdministrator(_ context.Context, _ int64, userID int64) (bool, error) {
	return !f.nonAdmins[userID], nil
}

func TestPrivateMessageCreatesTopicAndCopiesMessage(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{topicID: 321, copyID: 654, profileFile: "avatar-file"}
	h := NewHandler(-10099, store, api)
	update := telegram.Update{UpdateID: 1, Message: &telegram.Message{
		MessageID: 42, Chat: telegram.Chat{ID: 1001, Type: "private"},
		From: &telegram.User{ID: 1001, FirstName: "Ada", Username: "ada"}, Text: "hello",
	}}
	if err := h.Handle(context.Background(), update); err != nil {
		t.Fatal(err)
	}
	c := store.byUser[1001]
	if c.TopicID != 321 || c.DisplayName != "Ada" {
		t.Fatalf("conversation = %#v", c)
	}
	if len(api.copyCalls) != 1 || api.copyCalls[0] != (copyCall{-10099, 1001, 42, telegram.CopyOptions{ThreadID: 321}}) {
		t.Fatalf("copy calls = %#v", api.copyCalls)
	}
	if len(api.photos) != 1 || len(api.texts) != 0 {
		t.Fatalf("profile card not sent: photos=%v texts=%v", api.photos, api.texts)
	}
	if len(store.saved) != 1 || store.saved[0].UserMessageID != 42 || store.saved[0].TopicMessageID != 654 {
		t.Fatalf("links = %#v", store.saved)
	}
}

func TestNewConversationWithoutAvatarSendsTextCard(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{topicID: 321, copyID: 654}
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 42, Chat: telegram.Chat{ID: 1001, Type: "private"}, From: &telegram.User{ID: 1001, FirstName: "Ada"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.photos) != 0 || len(api.texts) != 1 {
		t.Fatalf("photos=%v texts=%v", api.photos, api.texts)
	}
}

func TestExistingPrivateConversationReusesTopic(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{topicID: 999, copyID: 10}
	c := Conversation{UserID: 1001, TopicID: 12, DisplayName: "Ada"}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{MessageID: 8, Chat: telegram.Chat{ID: 1001, Type: "private"}, From: &telegram.User{ID: 1001}}})
	if err != nil {
		t.Fatal(err)
	}
	if api.copyCalls[0].options.ThreadID != 12 || len(api.texts) != 0 || len(api.photos) != 0 {
		t.Fatalf("unexpected calls: %#v", api)
	}
}

func TestTopicMessageCopiesBackToUser(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{copyID: 91}
	c := Conversation{UserID: 1001, TopicID: 12, DisplayName: "Ada"}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 50, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "reply",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.copyCalls) != 1 || api.copyCalls[0] != (copyCall{1001, -10099, 50, telegram.CopyOptions{}}) {
		t.Fatalf("calls = %#v", api.copyCalls)
	}
	if len(store.saved) != 1 || store.saved[0].UserMessageID != 91 || store.saved[0].TopicMessageID != 50 {
		t.Fatalf("links = %#v", store.saved)
	}
}

func TestIgnoresBotMessagesAndOtherChats(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	h := NewHandler(-10099, store, api)
	updates := []telegram.Update{
		{},
		{Message: &telegram.Message{Chat: telegram.Chat{ID: -10022, Type: "supergroup"}}},
		{Message: &telegram.Message{Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{IsBot: true}, MessageThreadID: 4}},
	}
	for _, u := range updates {
		if err := h.Handle(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
	if len(api.copyCalls) != 0 {
		t.Fatalf("copy calls = %#v", api.copyCalls)
	}
}

func TestTopicMessageWithoutMappingReturnsUsefulError(t *testing.T) {
	h := NewHandler(-10099, newFakeStore(), &fakeTelegram{})
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{MessageID: 1, MessageThreadID: 404, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}}})
	if err == nil || !strings.Contains(err.Error(), "topic 404") {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateConversationFailureStopsCopy(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{topicID: 12}
	store.createErr = errors.New("db down")
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{MessageID: 1, Chat: telegram.Chat{ID: 1001, Type: "private"}, From: &telegram.User{ID: 1001}}})
	if err == nil || len(api.copyCalls) != 0 {
		t.Fatalf("err=%v calls=%v", err, api.copyCalls)
	}
}

func TestEditedTopicTextUpdatesUserCopy(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	store.saved = append(store.saved, MessageLink{UserID: 1001, UserMessageID: 91, TopicMessageID: 50})
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{EditedMessage: &telegram.Message{
		MessageID: 50, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "修改后",
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := editCall{"text", 1001, 91, "修改后"}
	if len(api.edits) != 1 || api.edits[0] != want {
		t.Fatalf("edits=%#v want=%#v", api.edits, want)
	}
}

func TestEditedPrivateCaptionRepliesToOriginalTopicMessage(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	store.saved = append(store.saved, MessageLink{UserID: 1001, UserMessageID: 42, TopicMessageID: 654})
	store.byUser[1001] = Conversation{UserID: 1001, TopicID: 12}
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{EditedMessage: &telegram.Message{
		MessageID: 42, Chat: telegram.Chat{ID: 1001, Type: "private"}, From: &telegram.User{ID: 1001}, Caption: "新说明",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.edits) != 0 || len(api.replies) != 1 {
		t.Fatalf("edits=%#v replies=%#v", api.edits, api.replies)
	}
	reply := api.replies[0]
	if reply.chatID != -10099 || reply.threadID != 12 || reply.messageID != 654 || !strings.Contains(reply.text, "新说明") {
		t.Fatalf("reply=%#v", reply)
	}
}

func TestDeleteCommandDeletesBothMessagesAndCommand(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	store.saved = append(store.saved, MessageLink{UserID: 1001, UserMessageID: 91, TopicMessageID: 50})
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 60, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "/delete",
		ReplyToMessage: &telegram.Message{MessageID: 50},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []deleteCall{{1001, 91}, {-10099, 50}, {-10099, 60}}
	if len(api.deletes) != len(want) {
		t.Fatalf("deletes=%#v", api.deletes)
	}
	for i := range want {
		if api.deletes[i] != want[i] {
			t.Fatalf("deletes=%#v want=%#v", api.deletes, want)
		}
	}
	if len(store.saved) != 0 {
		t.Fatalf("link was not removed: %#v", store.saved)
	}
}

func TestDeleteCommandWithoutReplyShowsUsageAndIsNotForwarded(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	c := Conversation{UserID: 1001, TopicID: 12}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 60, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "/delete",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.copyCalls) != 0 || len(api.texts) != 1 || len(store.saved) != 2 {
		t.Fatalf("copies=%v texts=%v links=%v", api.copyCalls, api.texts, store.saved)
	}
}

func TestPrivateDeleteCommandDeletesBothMessages(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	store.saved = append(store.saved, MessageLink{UserID: 1001, UserMessageID: 91, TopicMessageID: 50})
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 92, Chat: telegram.Chat{ID: 1001, Type: "private"}, From: &telegram.User{ID: 1001}, Text: "/del@forumdesk_bot",
		ReplyToMessage: &telegram.Message{MessageID: 91},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []deleteCall{{1001, 91}, {-10099, 50}, {1001, 92}}
	if len(api.deletes) != len(want) {
		t.Fatalf("deletes=%#v", api.deletes)
	}
	for i := range want {
		if api.deletes[i] != want[i] {
			t.Fatalf("deletes=%#v want=%#v", api.deletes, want)
		}
	}
}

func TestDeleteContinuesAndReportsPartialFailure(t *testing.T) {
	failed := deleteCall{-10099, 50}
	store, api := newFakeStore(), &fakeTelegram{deleteErr: map[deleteCall]error{failed: errors.New("missing delete permission")}}
	store.saved = append(store.saved, MessageLink{UserID: 1001, UserMessageID: 91, TopicMessageID: 50})
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 60, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "/delete",
		ReplyToMessage: &telegram.Message{MessageID: 50},
	}})
	if err == nil || len(api.deletes) != 3 || len(api.texts) != 1 {
		t.Fatalf("err=%v deletes=%v texts=%v", err, api.deletes, api.texts)
	}
	if !strings.Contains(api.texts[0], "删除消息") {
		t.Fatalf("notice=%q", api.texts[0])
	}
}

func TestStaffReplyPreservesQuoteAndProtectionForUser(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{copyID: 91}
	c := Conversation{UserID: 1001, TopicID: 12, ProtectContent: true}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	store.saved = append(store.saved, MessageLink{UserID: 1001, UserMessageID: 80, TopicMessageID: 40})
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 50, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "reply",
		ReplyToMessage: &telegram.Message{MessageID: 40},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := telegram.CopyOptions{ReplyToMessageID: 80, ProtectContent: true}
	if len(api.copyCalls) != 1 || api.copyCalls[0].options != want {
		t.Fatalf("calls=%#v want=%#v", api.copyCalls, want)
	}
}

func TestUserReplyPreservesQuoteInTopic(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{copyID: 92}
	c := Conversation{UserID: 1001, TopicID: 12, ProtectContent: true}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	store.saved = append(store.saved, MessageLink{UserID: 1001, UserMessageID: 80, TopicMessageID: 40})
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 81, Chat: telegram.Chat{ID: 1001, Type: "private"}, From: &telegram.User{ID: 1001}, Text: "reply",
		ReplyToMessage: &telegram.Message{MessageID: 80},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := telegram.CopyOptions{ThreadID: 12, ReplyToMessageID: 40, ProtectContent: true}
	if len(api.copyCalls) != 1 || api.copyCalls[0].options != want {
		t.Fatalf("calls=%#v want=%#v", api.copyCalls, want)
	}
}

func TestProtectCommandEnablesContentProtectionForTopic(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	c := Conversation{UserID: 1001, TopicID: 12}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 60, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "/protect on",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !store.byUser[1001].ProtectContent || len(api.copyCalls) != 0 || len(api.texts) != 1 || len(store.saved) != 2 {
		t.Fatalf("conversation=%#v copies=%v texts=%v links=%v", store.byUser[1001], api.copyCalls, api.texts, store.saved)
	}
	if store.saved[0].TopicMessageID != 60 || store.saved[1].TopicMessageID != 77 {
		t.Fatalf("links=%#v", store.saved)
	}
}

func TestHelpCommandShowsAdminCommandsWithoutForwarding(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	c := Conversation{UserID: 1001, TopicID: 12}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 61, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "/help",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.copyCalls) != 0 || len(api.texts) != 1 || !strings.Contains(api.texts[0], "/protect") || len(store.saved) != 2 {
		t.Fatalf("copies=%v texts=%v links=%v", api.copyCalls, api.texts, store.saved)
	}
}

func TestClearConversationRequiresConfirmation(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	c := Conversation{UserID: 1001, TopicID: 12}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 70, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "/clear",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.texts) != 1 || len(api.deletedTopics) != 0 || store.deletedConversation != 0 {
		t.Fatalf("texts=%v topics=%v deleted=%d", api.texts, api.deletedTopics, store.deletedConversation)
	}
}

func TestClearConversationDeletesBothSidesButPreservesTopicAndProfileCard(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	c := Conversation{UserID: 1001, TopicID: 12}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	for i := 1; i <= 101; i++ {
		store.saved = append(store.saved, MessageLink{UserID: 1001, UserMessageID: i, TopicMessageID: 1000 + i})
	}
	store.saved = append(store.saved,
		MessageLink{UserID: 1001, TopicMessageID: 2001},
		MessageLink{UserID: 1001, TopicMessageID: 2002},
	)
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 70, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "/clear confirm",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.deleteBatches) != 4 || len(api.deleteBatches[0].messageIDs) != 100 || len(api.deleteBatches[1].messageIDs) != 1 || len(api.deleteBatches[2].messageIDs) != 100 || len(api.deleteBatches[3].messageIDs) != 4 {
		t.Fatalf("batches=%v", api.deleteBatches)
	}
	if api.deleteBatches[0].chatID != 1001 || api.deleteBatches[2].chatID != -10099 {
		t.Fatalf("batch chats=%v", api.deleteBatches)
	}
	for _, batch := range api.deleteBatches[2:] {
		for _, messageID := range batch.messageIDs {
			if messageID == 900 {
				t.Fatal("untracked profile card was deleted")
			}
		}
	}
	if len(api.deletedTopics) != 0 || store.deletedConversation != 0 || len(store.saved) != 0 {
		t.Fatalf("topics=%v deleted=%d links=%d", api.deletedTopics, store.deletedConversation, len(store.saved))
	}
	if _, found := store.byTopic[12]; !found {
		t.Fatal("topic conversation mapping was removed")
	}
}

func TestClearConversationKeepsTopicWhenUserBatchDeleteFails(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{deleteBatchErr: errors.New("batch failed")}
	c := Conversation{UserID: 1001, TopicID: 12}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	store.saved = append(store.saved, MessageLink{UserID: 1001, UserMessageID: 1, TopicMessageID: 1001})
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 70, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "/clear confirm",
	}})
	if err == nil || !strings.Contains(err.Error(), "delete user conversation batch") {
		t.Fatalf("err=%v", err)
	}
	if len(api.deletedTopics) != 0 || store.deletedConversation != 0 || len(api.texts) != 1 {
		t.Fatalf("topics=%v deleted=%d notices=%v", api.deletedTopics, store.deletedConversation, api.texts)
	}
}

func TestPrivateClearIsRejectedAndPrivateHelpListsDelete(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	h := NewHandler(-10099, store, api)
	for _, command := range []string{"/clear confirm", "/help"} {
		err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
			MessageID: 80, Chat: telegram.Chat{ID: 1001, Type: "private"}, From: &telegram.User{ID: 1001}, Text: command,
		}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(api.copyCalls) != 0 || len(api.texts) != 2 || !strings.Contains(api.texts[1], "/delete") || strings.Contains(api.texts[1], "/clear") {
		t.Fatalf("copies=%v texts=%v", api.copyCalls, api.texts)
	}
}

func TestUnapprovedStaffMessageStaysInternal(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{copyID: 91, nonAdmins: map[int64]bool{7: true}}
	c := Conversation{UserID: 1001, TopicID: 12}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 50, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7, FirstName: "Tech"}, Text: "internal diagnosis",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.copyCalls) != 0 || len(store.saved) != 1 || !store.saved[0].Hidden || store.saved[0].TopicMessageID != 50 {
		t.Fatalf("copies=%v links=%#v", api.copyCalls, store.saved)
	}
}

func TestAdminAllowsStaffForCurrentTopic(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{copyID: 91, nonAdmins: map[int64]bool{7: true}}
	c := Conversation{UserID: 1001, TopicID: 12}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	h := NewHandler(-10099, store, api)
	allow := telegram.Update{Message: &telegram.Message{
		MessageID: 60, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 9}, Text: "/allow_staff",
		ReplyToMessage: &telegram.Message{MessageID: 50, From: &telegram.User{ID: 7, FirstName: "Tech"}},
	}}
	if err := h.Handle(context.Background(), allow); err != nil {
		t.Fatal(err)
	}
	if !store.staffVisibility[1001][7] || len(api.copyCalls) != 0 {
		t.Fatalf("visibility=%v copies=%v", store.staffVisibility, api.copyCalls)
	}
	if err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 61, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7, FirstName: "Tech"}, Text: "customer-visible answer",
	}}); err != nil {
		t.Fatal(err)
	}
	if len(api.copyCalls) != 1 || api.copyCalls[0].messageID != 61 {
		t.Fatalf("copies=%#v", api.copyCalls)
	}
}

func TestAdminShowsThenHidesOneStaffMessage(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{copyID: 91, nonAdmins: map[int64]bool{7: true}}
	c := Conversation{UserID: 1001, TopicID: 12}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	store.saved = []MessageLink{{UserID: 1001, TopicMessageID: 50, Hidden: true}}
	h := NewHandler(-10099, store, api)
	original := &telegram.Message{MessageID: 50, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7, FirstName: "Tech"}, Text: "approved answer"}
	if err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 60, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 9}, Text: "/show", ReplyToMessage: original,
	}}); err != nil {
		t.Fatal(err)
	}
	link, found, _ := store.MessageLinkByTopic(context.Background(), 50)
	if !found || link.Hidden || link.UserMessageID != 91 {
		t.Fatalf("shown link=%#v found=%v", link, found)
	}
	if err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 62, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 9}, Text: "/hide", ReplyToMessage: original,
	}}); err != nil {
		t.Fatal(err)
	}
	link, found, _ = store.MessageLinkByTopic(context.Background(), 50)
	if !found || !link.Hidden || link.UserMessageID != 0 {
		t.Fatalf("hidden link=%#v found=%v", link, found)
	}
	if len(api.deletes) == 0 || api.deletes[0] != (deleteCall{1001, 91}) {
		t.Fatalf("deletes=%#v", api.deletes)
	}
}

func TestWebConversationPublishesAdminReplyToWebQueue(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	c := Conversation{UserID: -1001, TopicID: 12, PublicID: "wc_1", Channel: "web", DisplayName: "Ada"}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	h := NewHandler(-10099, store, api)
	if err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 50, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 9, FirstName: "Alice"}, Text: "hello from support",
	}}); err != nil {
		t.Fatal(err)
	}
	if len(api.copyCalls) != 0 || len(store.webMessages) != 1 || store.webMessages[0].Text != "hello from support" || !store.webMessages[0].Visible {
		t.Fatalf("copies=%v web=%#v", api.copyCalls, store.webMessages)
	}
	if len(store.saved) != 1 || store.saved[0].WebMessageID == "" {
		t.Fatalf("links=%#v", store.saved)
	}
}

func TestWebStaffReplyKeepsQuotedMessageMapping(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	c := Conversation{UserID: -1001, TopicID: 12, PublicID: "wc_1", Channel: "web", DisplayName: "Ada"}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	store.saved = []MessageLink{{UserID: c.UserID, TopicMessageID: 40, WebMessageID: "wm_customer"}}
	h := NewHandler(-10099, store, api)
	if err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 50, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"},
		From: &telegram.User{ID: 9, FirstName: "Alice"}, Text: "quoted answer",
		ReplyToMessage: &telegram.Message{MessageID: 40},
	}}); err != nil {
		t.Fatal(err)
	}
	if len(store.webMessages) != 1 || store.webMessages[0].ReplyToMessageID != "wm_customer" {
		t.Fatalf("web=%#v", store.webMessages)
	}
}

func TestAdminDeniesAndListsStaffWhileMemberCannotControl(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{nonAdmins: map[int64]bool{7: true}}
	c := Conversation{UserID: 1001, TopicID: 12}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	store.staffVisibility[1001] = map[int64]bool{7: true}
	h := NewHandler(-10099, store, api)

	unauthorized := telegram.Update{Message: &telegram.Message{
		MessageID: 60, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "/deny_staff",
		ReplyToMessage: &telegram.Message{MessageID: 50, From: &telegram.User{ID: 8, FirstName: "Sales"}},
	}}
	if err := h.Handle(context.Background(), unauthorized); err != nil {
		t.Fatal(err)
	}
	if store.staffVisibility[1001][8] {
		t.Fatal("non-admin changed staff visibility")
	}

	deny := unauthorized
	deny.Message = &telegram.Message{
		MessageID: 61, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 9}, Text: "/deny_staff",
		ReplyToMessage: &telegram.Message{MessageID: 50, From: &telegram.User{ID: 7, FirstName: "Tech"}},
	}
	if err := h.Handle(context.Background(), deny); err != nil {
		t.Fatal(err)
	}
	if store.staffVisibility[1001][7] {
		t.Fatal("staff remained visible")
	}
	if err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 62, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 9}, Text: "/staff",
	}}); err != nil {
		t.Fatal(err)
	}
	if len(api.texts) != 3 || !strings.Contains(api.texts[2], "没有已授权") {
		t.Fatalf("texts=%v", api.texts)
	}
}

func TestAdminHidesPublishedWebMessageAndAppendsTombstone(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	c := Conversation{UserID: -1001, TopicID: 12, PublicID: "wc_1", Channel: "web", DisplayName: "Ada"}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	store.webMessages = []WebMessage{{ID: "wm_1", ConversationID: "wc_1", Direction: "staff", Text: "visible", Visible: true}}
	store.saved = []MessageLink{{UserID: -1001, TopicMessageID: 50, WebMessageID: "wm_1"}}
	h := NewHandler(-10099, store, api)
	original := &telegram.Message{MessageID: 50, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "visible"}
	if err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 60, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 9}, Text: "/hide", ReplyToMessage: original,
	}}); err != nil {
		t.Fatal(err)
	}
	if store.webMessages[0].Visible || len(store.webMessages) != 2 || store.webMessages[1].Event != "message_hidden" || store.webMessages[1].TargetMessageID != "wm_1" {
		t.Fatalf("web messages=%#v", store.webMessages)
	}
}

func TestClearUserDeletesOnlyPrivateCopiesAndKeepsTopicHistory(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	c := Conversation{UserID: 1001, TopicID: 12}
	store.byUser[c.UserID], store.byTopic[int64(c.TopicID)] = c, c
	store.saved = []MessageLink{
		{UserID: 1001, UserMessageID: 80, TopicMessageID: 40},
		{UserID: 1001, UserMessageID: 81, TopicMessageID: 41},
	}
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 70, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "/clear_user confirm",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.deleteBatches) != 1 || api.deleteBatches[0].chatID != 1001 || len(api.deleteBatches[0].messageIDs) != 2 {
		t.Fatalf("batches=%v", api.deleteBatches)
	}
	if len(api.deletedTopics) != 0 || len(store.saved) != 4 {
		t.Fatalf("topics=%v links=%v", api.deletedTopics, store.saved)
	}
	for _, link := range store.saved[:2] {
		if link.UserMessageID != 0 || link.TopicMessageID == 0 {
			t.Fatalf("link=%#v", link)
		}
	}
	if store.saved[2].TopicMessageID != 70 || store.saved[3].TopicMessageID != 77 {
		t.Fatalf("command links=%#v", store.saved[2:])
	}
}

func TestDeleteAfterUserOnlyClearRemovesTheRepliedTopicMapping(t *testing.T) {
	store, api := newFakeStore(), &fakeTelegram{}
	store.saved = []MessageLink{
		{UserID: 1001, TopicMessageID: 40},
		{UserID: 1001, TopicMessageID: 41},
	}
	h := NewHandler(-10099, store, api)
	err := h.Handle(context.Background(), telegram.Update{Message: &telegram.Message{
		MessageID: 70, MessageThreadID: 12, Chat: telegram.Chat{ID: -10099, Type: "supergroup"}, From: &telegram.User{ID: 7}, Text: "/delete",
		ReplyToMessage: &telegram.Message{MessageID: 41},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.saved) != 1 || store.saved[0].TopicMessageID != 40 {
		t.Fatalf("links=%#v", store.saved)
	}
	want := []deleteCall{{-10099, 41}, {-10099, 70}}
	if len(api.deletes) != len(want) || api.deletes[0] != want[0] || api.deletes[1] != want[1] {
		t.Fatalf("deletes=%#v want=%#v", api.deletes, want)
	}
}
