package mail

import "testing"

func TestParseMailMessagesKeepsUnreadAndDeduplicatesByID(t *testing.T) {
	raw := []byte(`[
		{"id":42,"subject":"first","sender":"a@example.com","date":"2026-06-17T08:52:00+03:00","mailbox":"Inbox","isUnread":true},
		{"id":42,"subject":"first duplicate","sender":"a@example.com","date":"2026-06-17T08:52:00+03:00","mailbox":"All Mail","isUnread":true},
		{"id":43,"subject":"read","sender":"b@example.com","date":"2026-06-17T08:53:00+03:00","mailbox":"Inbox","isUnread":false},
		{"id":44,"subject":"second","sender":"c@example.com","date":"2026-06-17T08:54:00+03:00","mailbox":"Archive","isUnread":true}
	]`)

	messages, err := parseMailMessages(raw)
	if err != nil {
		t.Fatalf("parseMailMessages() error = %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(messages))
	}
	if messages[0].ID != 42 || messages[0].Subject != "first" {
		t.Fatalf("messages[0] = %+v, want first unread message with id 42", messages[0])
	}
	if messages[1].ID != 44 || messages[1].Subject != "second" {
		t.Fatalf("messages[1] = %+v, want second unread message with id 44", messages[1])
	}
}
