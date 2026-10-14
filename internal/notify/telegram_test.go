package notify

import (
	"strings"
	"testing"
)

func TestEscapeMarkdownV2EscapesSpecials(t *testing.T) {
	got := escapeMarkdownV2("hello_world *bold* [x](y)")
	for _, want := range []string{"\\_", "\\*", "\\[", "\\]", "\\(", "\\)"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in escaped output %q", want, got)
		}
	}
}

func TestEscapeMarkdownV2LeavesPlainText(t *testing.T) {
	if got := escapeMarkdownV2("plain text here"); got != "plain text here" {
		t.Errorf("plain text should be unchanged, got %q", got)
	}
}

func TestEscapeMarkdownV2IdempotenceNotAssumed(t *testing.T) {
	once := escapeMarkdownV2("a-b")
	if !strings.Contains(once, "\\-") {
		t.Errorf("dash should be escaped, got %q", once)
	}
}

func TestNewTelegramKeepsCredentials(t *testing.T) {
	tg := NewTelegram("token123", "chat456")
	if tg.botToken != "token123" || tg.chatID != "chat456" {
		t.Errorf("credentials not stored: %q %q", tg.botToken, tg.chatID)
	}
	if tg.client == nil {
		t.Error("http client should be initialized")
	}
}
