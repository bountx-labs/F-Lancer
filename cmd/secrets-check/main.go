package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// secrets-check verifies that every configured credential is valid by hitting a
// lightweight, non-consuming endpoint for each service. It does not print secret
// values; it only reports set/valid/invalid status. Any failure exits non-zero
// so the workflow step surfaces red.
func main() {
	client := &http.Client{Timeout: 15 * time.Second}

	type check struct {
		name string
		run  func() error
	}

	checks := []check{
		{"TELEGRAM_BOT_TOKEN", func() error { return checkTelegram(client, os.Getenv("TELEGRAM_BOT_TOKEN")) }},
		{"TELEGRAM_CHAT_ID", func() error {
			return checkChatID(client, os.Getenv("TELEGRAM_BOT_TOKEN"), os.Getenv("TELEGRAM_CHAT_ID"))
		}},
		{"GEMINI_API_KEY", func() error { return checkGemini(client, os.Getenv("GEMINI_API_KEY")) }},
		{"KILO_API_KEY", func() error {
			return checkOpenAICompat(client, firstNonEmpty(os.Getenv("KILO_GATEWAY_API_KEY"), os.Getenv("KILO_API_KEY")), envOr(os.Getenv("KILO_GATEWAY_BASE_URL"), "https://api.kilo.ai/api/gateway"))
		}},
	}

	failed := false
	for _, c := range checks {
		fmt.Printf("%-22s ", c.name)
		if err := c.run(); err != nil {
			fmt.Printf("INVALID: %v\n", err)
			failed = true
		} else {
			fmt.Println("OK")
		}
	}

	if failed {
		os.Exit(1)
	}
	fmt.Println("ALL SECRETS VALID")
}

func checkTelegram(client *http.Client, token string) error {
	if token == "" {
		return fmt.Errorf("not set")
	}
	return expectOK(client, "https://api.telegram.org/bot"+token+"/getMe")
}

func checkChatID(client *http.Client, token, chatID string) error {
	if chatID == "" {
		return fmt.Errorf("not set")
	}
	if token == "" {
		return fmt.Errorf("token missing, cannot verify chat id")
	}
	return expectOK(client, "https://api.telegram.org/bot"+token+"/getChat?chat_id="+chatID)
}

func checkGemini(client *http.Client, key string) error {
	if key == "" {
		return fmt.Errorf("not set")
	}
	return expectOK(client, "https://generativelanguage.googleapis.com/v1beta/models?key="+key+"&pageSize=1")
}

func checkOpenAICompat(client *http.Client, key, baseURL string) error {
	if key == "" {
		return fmt.Errorf("not set")
	}
	req, err := http.NewRequest("GET", baseURL+"/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func expectOK(client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var payload struct {
		OK bool `json:"ok"`
	}
	// Telegram wraps responses in {"ok": true}; other endpoints return 200.
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil && !payload.OK {
		return fmt.Errorf("api returned ok:false")
	}
	return nil
}

func envOr(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
