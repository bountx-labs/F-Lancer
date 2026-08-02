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

func (t *Telegram) Send(text string) error {
	escaped := escapeMarkdownV2(text)
	msg := telegramMessage{
		ChatID:    t.chatID,
		Text:      escaped,
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
	blocks := []string{
		fmt.Sprintf("*New Job Match*\n\n%s", escapeMarkdownV2(link)),
		fmt.Sprintf("*Proposal*\n\n%s", escapeMarkdownV2(proposal)),
		fmt.Sprintf("*Executive Guide*\n\n%s", escapeMarkdownV2(guide)),
	}

	for i, block := range blocks {
		if err := t.Send(block); err != nil {
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