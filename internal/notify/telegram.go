package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Telegram struct {
	botToken string
	chatID   string
	client   *http.Client
}

type telegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

func NewTelegram(botToken, chatID string) *Telegram {
	return &Telegram{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Send escapes raw text for MarkdownV2 and delivers it.
func (t *Telegram) Send(text string) error {
	return t.post(escapeMarkdownV2(text))
}

// post delivers text that is already valid MarkdownV2 (headers kept intact).
func (t *Telegram) post(markdownText string) error {
	msg := telegramMessage{
		ChatID:    t.chatID,
		Text:      markdownText,
		ParseMode: "MarkdownV2",
	}

	data, _ := json.Marshal(msg)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)

	resp, err := t.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("telegram send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fallback: plain text with escape backslashes stripped so it stays readable.
		msg.Text = strings.ReplaceAll(markdownText, "\\", "")
		msg.ParseMode = ""
		data, _ = json.Marshal(msg)
		resp2, err2 := t.client.Post(url, "application/json", bytes.NewReader(data))
		if err2 != nil {
			return fmt.Errorf("telegram retry failed: %w", err2)
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != http.StatusOK {
			return fmt.Errorf("telegram HTTP %d", resp2.StatusCode)
		}
	}

	return nil
}

func (t *Telegram) SendJobAlert(link, proposal, guide string) error {
	// Headers use MarkdownV2 bold; content is escaped exactly once.
	blocks := []string{
		"*New Job Match*\n\n" + escapeMarkdownV2(link),
		"*Proposal*\n\n" + escapeMarkdownV2(proposal),
		"*Executive Guide*\n\n" + escapeMarkdownV2(guide),
	}

	for i, block := range blocks {
		if err := t.post(block); err != nil {
			return fmt.Errorf("send block %d: %w", i, err)
		}
	}

	return nil
}

func (t *Telegram) SendAlert(text string) error {
	return t.Send(text)
}

func escapeMarkdownV2(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}