package telegram

type Update struct {
	UpdateID      int      `json:"update_id"`
	Message       *Message `json:"message,omitempty"`
	EditedMessage *Message `json:"edited_message,omitempty"`
}

type Message struct {
	MessageID       int      `json:"message_id"`
	MessageThreadID int      `json:"message_thread_id,omitempty"`
	From            *User    `json:"from,omitempty"`
	Chat            Chat     `json:"chat"`
	Text            string   `json:"text,omitempty"`
	Caption         string   `json:"caption,omitempty"`
	ReplyToMessage  *Message `json:"reply_to_message,omitempty"`
}

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type CopyOptions struct {
	ThreadID         int
	ReplyToMessageID int
	ProtectContent   bool
}
