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

// post delivers text that is already valid MarkdownV2 (headers kept intact),
// retrying transient failures (network, 429, 5xx) with exponential backoff.
func (t *Telegram) post(markdownText string) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
		}
		transient, err := t.sendOnce(markdownText)
		if err == nil {
			return nil
		}
		lastErr = err
		if !transient {
			break
		}
	}
	return lastErr
}

// sendOnce performs a single sendMessage attempt. It returns transient=true
// for network errors, 429 and 5xx so the caller can retry; other HTTP errors
// (e.g. 400 bad MarkdownV2) trigger a plain-text fallback instead.
func (t *Telegram) sendOnce(markdownText string) (bool, error) {
	msg := telegramMessage{
		ChatID:    t.chatID,
		Text:      markdownText,
		ParseMode: "MarkdownV2",
	}

	data, _ := json.Marshal(msg)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)

	resp, err := t.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return true, fmt.Errorf("telegram send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
		return true, fmt.Errorf("telegram HTTP %d", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		// Fallback: plain text with escape backslashes stripped so it stays readable.
		msg.Text = strings.ReplaceAll(markdownText, "\\", "")
		msg.ParseMode = ""
		data, _ = json.Marshal(msg)
		resp2, err2 := t.client.Post(url, "application/json", bytes.NewReader(data))
		if err2 != nil {
			return true, fmt.Errorf("telegram retry failed: %w", err2)
		}
		defer resp2.Body.Close()
		if resp2.StatusCode == http.StatusTooManyRequests || resp2.StatusCode >= http.StatusInternalServerError {
			return true, fmt.Errorf("telegram HTTP %d", resp2.StatusCode)
		}
		if resp2.StatusCode != http.StatusOK {
			return false, fmt.Errorf("telegram HTTP %d", resp2.StatusCode)
		}
	}

	return false, nil
}

func (t *Telegram) SendJobAlert(link, proposal, guide string) error {
	// Headers use MarkdownV2 bold; content is escaped exactly once.
	// All three blocks go in a single message so a partial send can never
	// leave the job unmarked with some blocks already delivered.
	msg := "*New Job Match*\n\n" + escapeMarkdownV2(link) +
		"\n\n*Proposal*\n\n" + escapeMarkdownV2(proposal) +
		"\n\n*Executive Guide*\n\n" + escapeMarkdownV2(guide)
	return t.post(msg)
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
	)
	return replacer.Replace(text)
}
