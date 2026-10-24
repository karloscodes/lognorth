package alerts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"lognorth/internal/settings"
)

// IsQuietHours reports whether the time is within the owner's quiet hours.
func IsQuietHours(s *settings.Settings) bool {
	if !s.QuietHoursEnabled {
		return false
	}
	hour := time.Now().Hour()
	if s.QuietStart > s.QuietEnd {
		// Crosses midnight (e.g., 23-7)
		return hour >= s.QuietStart || hour < s.QuietEnd
	}
	return hour >= s.QuietStart && hour < s.QuietEnd
}

// CanNotify reports whether a channel is configured and quiet hours are off.
func CanNotify(s *settings.Settings) bool {
	if IsQuietHours(s) {
		return false
	}
	switch s.NotifyChannel {
	case "ntfy":
		return s.IsNtfyConfigured()
	case "email":
		return s.IsEmailConfigured()
	case "telegram":
		return s.IsTelegramConfigured()
	case "webhook":
		return s.IsWebhookConfigured()
	default:
		return false
	}
}

// Send sends a notification through the configured channel.
func Send(s *settings.Settings, title, message string) error {
	// Titles carry event data: a path or error message from an SDK. Email and
	// ntfy put the title in a header, where a line break starts a new header.
	title = oneLine.Replace(title)

	switch s.NotifyChannel {
	case "ntfy":
		return sendViaNtfy(s, title, message)
	case "email":
		return sendViaSMTP(s, title, message)
	case "telegram":
		return sendViaTelegram(s, title, message)
	case "webhook":
		return sendViaWebhook(s, title, message)
	default:
		return fmt.Errorf("no notification channel configured")
	}
}

var oneLine = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")

// sendViaNtfy sends notification via ntfy.sh
func sendViaNtfy(s *settings.Settings, title, message string) error {
	if s.NtfyURL == "" {
		return fmt.Errorf("ntfy URL not configured")
	}

	req, err := http.NewRequest("POST", s.NtfyURL, strings.NewReader(message))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Title", title)
	req.Header.Set("Priority", "high")
	req.Header.Set("Tags", "warning")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("ntfy failed with status %d", resp.StatusCode)
	}

	return nil
}

// sendViaTelegram sends notification via Telegram bot
func sendViaTelegram(s *settings.Settings, title, message string) error {
	if !s.IsTelegramConfigured() {
		return fmt.Errorf("Telegram not configured")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.TelegramBotToken)

	// Plain text: Markdown mode rejects messages with an unbalanced _ or `,
	// which paths and error messages often contain.
	body, err := json.Marshal(map[string]string{
		"chat_id": s.TelegramChatID,
		"text":    title + "\n\n" + message,
	})
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return errors.New("failed to create telegram request")
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Unwrap drops the request URL, which holds the bot token.
		return fmt.Errorf("telegram request failed: %w", errors.Unwrap(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram failed with status %d", resp.StatusCode)
	}

	return nil
}

// sendViaWebhook POSTs the alert as JSON. text repeats title and message in
// one field, the shape Slack-style incoming webhooks read.
func sendViaWebhook(s *settings.Settings, title, message string) error {
	if !s.IsWebhookConfigured() {
		return fmt.Errorf("webhook URL not configured")
	}

	body, err := json.Marshal(map[string]string{
		"source":  "lognorth",
		"title":   title,
		"message": message,
		"text":    title + "\n\n" + message,
		"sent_at": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("failed to encode alert: %w", err)
	}

	req, err := http.NewRequest("POST", s.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LogNorth")
	if s.WebhookToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.WebhookToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("webhook failed with status %d", resp.StatusCode)
	}

	return nil
}
