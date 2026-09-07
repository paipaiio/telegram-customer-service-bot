package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"forumdesk/internal/bot"
)

type fileData struct {
	Conversations   map[string]bot.Conversation `json:"conversations"`
	MessageLinks    []bot.MessageLink           `json:"message_links,omitempty"`
	WebMessages     []bot.WebMessage            `json:"web_messages,omitempty"`
	NextWebSequence int64                       `json:"next_web_sequence,omitempty"`
	StaffVisibility map[string][]int64          `json:"staff_visibility,omitempty"`
}

type FileStore struct {
	mu   sync.RWMutex
	path string
	data fileData
}

func OpenFile(path string) (*FileStore, error) {
	s := &FileStore{path: path, data: fileData{Conversations: map[string]bot.Conversation{}, StaffVisibility: map[string][]int64{}}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("read data file: %w", err)
	}
	if len(raw) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("decode data file: %w", err)
	}
	if s.data.Conversations == nil {
		s.data.Conversations = map[string]bot.Conversation{}
	}
	if s.data.StaffVisibility == nil {
		s.data.StaffVisibility = map[string][]int64{}
	}
	return s, nil
}

func (s *FileStore) ConversationByUser(_ context.Context, userID int64) (bot.Conversation, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.data.Conversations[strconv.FormatInt(userID, 10)]
	return c, ok, nil
}

func (s *FileStore) ConversationByTopic(_ context.Context, topicID int64) (bot.Conversation, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.data.Conversations {
		if int64(c.TopicID) == topicID {
			return c, true, nil
		}
	}
	return bot.Conversation{}, false, nil
}

func (s *FileStore) ConversationByPublicID(_ context.Context, publicID string) (bot.Conversation, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.data.Conversations {
		if c.PublicID == publicID {
			return c, true, nil
		}
	}
	return bot.Conversation{}, false, nil
}

func (s *FileStore) CreateConversation(_ context.Context, c bot.Conversation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strconv.FormatInt(c.UserID, 10)
	if _, exists := s.data.Conversations[key]; exists {
		return fmt.Errorf("user %d already has a conversation", c.UserID)
	}
	for _, existing := range s.data.Conversations {
		if existing.TopicID == c.TopicID {
			return fmt.Errorf("topic %d already mapped", c.TopicID)
		}
	}
	s.data.Conversations[key] = c
	if err := s.persist(); err != nil {
		delete(s.data.Conversations, key)
		return err
	}
	return nil
}

func (s *FileStore) SaveMessageLink(_ context.Context, link bot.MessageLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.MessageLinks = append(s.data.MessageLinks, link)
	if err := s.persist(); err != nil {
		s.data.MessageLinks = s.data.MessageLinks[:len(s.data.MessageLinks)-1]
		return err
	}
	return nil
}

func (s *FileStore) MessageLinkByUser(_ context.Context, userID int64, messageID int) (bot.MessageLink, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.data.MessageLinks) - 1; i >= 0; i-- {
		link := s.data.MessageLinks[i]
		if link.UserID == userID && link.UserMessageID == messageID {
			return link, true, nil
		}
	}
	return bot.MessageLink{}, false, nil
}

func (s *FileStore) MessageLinkByTopic(_ context.Context, messageID int) (bot.MessageLink, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.data.MessageLinks) - 1; i >= 0; i-- {
		link := s.data.MessageLinks[i]
		if link.TopicMessageID == messageID {
			return link, true, nil
		}
	}
	return bot.MessageLink{}, false, nil
}

func (s *FileStore) DeleteMessageLink(_ context.Context, target bot.MessageLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, link := range s.data.MessageLinks {
		if link.UserID != target.UserID {
			continue
		}
		userMatch := target.UserMessageID != 0 && link.UserMessageID == target.UserMessageID
		topicMatch := target.TopicMessageID != 0 && link.TopicMessageID == target.TopicMessageID
		if !userMatch && !topicMatch {
			continue
		}
		removed := link
		s.data.MessageLinks = append(s.data.MessageLinks[:i], s.data.MessageLinks[i+1:]...)
		if err := s.persist(); err != nil {
			s.data.MessageLinks = append(s.data.MessageLinks, bot.MessageLink{})
			copy(s.data.MessageLinks[i+1:], s.data.MessageLinks[i:])
			s.data.MessageLinks[i] = removed
			return err
		}
		return nil
	}
	return nil
}

func (s *FileStore) SetConversationProtection(_ context.Context, userID int64, enabled bool) (bot.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strconv.FormatInt(userID, 10)
	c, found := s.data.Conversations[key]
	if !found {
		return bot.Conversation{}, fmt.Errorf("user %d has no conversation", userID)
	}
	previous := c.ProtectContent
	c.ProtectContent = enabled
	s.data.Conversations[key] = c
	if err := s.persist(); err != nil {
		c.ProtectContent = previous
		s.data.Conversations[key] = c
		return bot.Conversation{}, err
	}
	return c, nil
}

func (s *FileStore) MessageLinksByUser(_ context.Context, userID int64) ([]bot.MessageLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	links := make([]bot.MessageLink, 0)
	for _, link := range s.data.MessageLinks {
		if link.UserID == userID {
			links = append(links, link)
		}
	}
	return links, nil
}

func (s *FileStore) AddWebMessage(_ context.Context, message bot.WebMessage) (bot.WebMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.data.WebMessages {
		if existing.ID == message.ID {
			return bot.WebMessage{}, fmt.Errorf("web message %s already exists", message.ID)
		}
	}
	previousSequence := s.data.NextWebSequence
	s.data.NextWebSequence++
	message.Sequence = s.data.NextWebSequence
	s.data.WebMessages = append(s.data.WebMessages, message)
	if err := s.persist(); err != nil {
		s.data.WebMessages = s.data.WebMessages[:len(s.data.WebMessages)-1]
		s.data.NextWebSequence = previousSequence
		return bot.WebMessage{}, err
	}
	return message, nil
}

func (s *FileStore) WebMessagesAfter(_ context.Context, conversationID string, after int64, limit int) ([]bot.WebMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	messages := make([]bot.WebMessage, 0, limit)
	for _, message := range s.data.WebMessages {
		if message.ConversationID != conversationID || message.Sequence <= after || !message.Visible {
			continue
		}
		messages = append(messages, message)
		if len(messages) == limit {
			break
		}
	}
	return messages, nil
}

func (s *FileStore) WebMessageByID(_ context.Context, messageID string) (bot.WebMessage, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, message := range s.data.WebMessages {
		if message.ID == messageID {
			return message, true, nil
		}
	}
	return bot.WebMessage{}, false, nil
}

func (s *FileStore) SetWebMessageVisible(_ context.Context, messageID string, visible bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.WebMessages {
		if s.data.WebMessages[i].ID != messageID {
			continue
		}
		previous := s.data.WebMessages[i].Visible
		s.data.WebMessages[i].Visible = visible
		if err := s.persist(); err != nil {
			s.data.WebMessages[i].Visible = previous
			return err
		}
		return nil
	}
	return fmt.Errorf("web message %s not found", messageID)
}

func (s *FileStore) SetStaffVisibility(_ context.Context, userID, staffID int64, visible bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strconv.FormatInt(userID, 10)
	if _, found := s.data.Conversations[key]; !found {
		return fmt.Errorf("user %d has no conversation", userID)
	}
	previous := append([]int64(nil), s.data.StaffVisibility[key]...)
	staffIDs := append([]int64(nil), previous...)
	index := -1
	for i, id := range staffIDs {
		if id == staffID {
			index = i
			break
		}
	}
	if visible && index == -1 {
		staffIDs = append(staffIDs, staffID)
	}
	if !visible && index >= 0 {
		staffIDs = append(staffIDs[:index], staffIDs[index+1:]...)
	}
	s.data.StaffVisibility[key] = staffIDs
	if err := s.persist(); err != nil {
		s.data.StaffVisibility[key] = previous
		return err
	}
	return nil
}

func (s *FileStore) StaffVisible(_ context.Context, userID, staffID int64) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range s.data.StaffVisibility[strconv.FormatInt(userID, 10)] {
		if id == staffID {
			return true, nil
		}
	}
	return false, nil
}

func (s *FileStore) VisibleStaff(_ context.Context, userID int64) ([]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]int64(nil), s.data.StaffVisibility[strconv.FormatInt(userID, 10)]...), nil
}

func (s *FileStore) ReplaceMessageLinksByUser(_ context.Context, userID int64, links []bot.MessageLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, link := range links {
		if link.UserID != userID {
			return fmt.Errorf("message link user %d does not match conversation user %d", link.UserID, userID)
		}
	}
	previous := append([]bot.MessageLink(nil), s.data.MessageLinks...)
	kept := make([]bot.MessageLink, 0, len(s.data.MessageLinks)+len(links))
	for _, link := range s.data.MessageLinks {
		if link.UserID != userID {
			kept = append(kept, link)
		}
	}
	s.data.MessageLinks = append(kept, links...)
	if err := s.persist(); err != nil {
		s.data.MessageLinks = previous
		return err
	}
	return nil
}

func (s *FileStore) DeleteConversation(_ context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strconv.FormatInt(userID, 10)
	c, found := s.data.Conversations[key]
	if !found {
		return fmt.Errorf("user %d has no conversation", userID)
	}
	previousLinks := append([]bot.MessageLink(nil), s.data.MessageLinks...)
	delete(s.data.Conversations, key)
	kept := s.data.MessageLinks[:0]
	for _, link := range s.data.MessageLinks {
		if link.UserID != userID {
			kept = append(kept, link)
		}
	}
	s.data.MessageLinks = kept
	if err := s.persist(); err != nil {
		s.data.Conversations[key] = c
		s.data.MessageLinks = previousLinks
		return err
	}
	return nil
}

func (s *FileStore) persist() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode data: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write data: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace data: %w", err)
	}
	return nil
}
