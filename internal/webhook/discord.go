package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DiscordMessage represents a Discord webhook message payload.
type DiscordMessage struct {
	Content   string          `json:"content,omitempty"`
	Embeds    []DiscordEmbed  `json:"embeds,omitempty"`
	Username  string          `json:"username,omitempty"`
}

// DiscordEmbed represents a Discord embed object.
type DiscordEmbed struct {
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Color       int                 `json:"color,omitempty"`
	Fields      []DiscordEmbedField `json:"fields,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
	Footer      *DiscordFooter      `json:"footer,omitempty"`
}

// DiscordEmbedField represents a field in a Discord embed.
type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// DiscordFooter represents the footer of a Discord embed.
type DiscordFooter struct {
	Text string `json:"text"`
}

const (
	ColorGreen  = 0x00FF00
	ColorRed    = 0xFF0000
	ColorYellow = 0xFFFF00
	ColorBlue   = 0x3498DB
	ColorPurple = 0x9B59B6
)

// SendNotification sends a Discord webhook notification.
func SendNotification(webhookURL, title, description string, color int, fields []DiscordEmbedField) error {
	if webhookURL == "" {
		return nil
	}

	embed := DiscordEmbed{
		Title:       title,
		Description: description,
		Color:       color,
		Fields:      fields,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Footer: &DiscordFooter{
			Text: "K-12 Reverse",
		},
	}

	msg := DiscordMessage{
		Embeds:   []DiscordEmbed{embed},
		Username: "K-12 Bot",
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// SendAccountCreated sends a notification for a successfully created account.
func SendAccountCreated(webhookURL, email, planType, workspaceID string) error {
	fields := []DiscordEmbedField{
		{Name: "Email", Value: email, Inline: true},
		{Name: "Plan", Value: planType, Inline: true},
	}
	if workspaceID != "" {
		fields = append(fields, DiscordEmbedField{Name: "Workspace", Value: workspaceID[:8] + "...", Inline: true})
	}
	return SendNotification(webhookURL, "✅ Account Created", "New ChatGPT account registered successfully", ColorGreen, fields)
}

// SendAccountFailed sends a notification for a failed registration attempt.
func SendAccountFailed(webhookURL, email, reason string) error {
	fields := []DiscordEmbedField{
		{Name: "Email", Value: email, Inline: true},
		{Name: "Reason", Value: reason, Inline: false},
	}
	return SendNotification(webhookURL, "❌ Registration Failed", "Account creation failed", ColorRed, fields)
}

// SendTokenExpiring sends a notification when tokens are about to expire.
func SendTokenExpiring(webhookURL, email, expiresIn string) error {
	fields := []DiscordEmbedField{
		{Name: "Email", Value: email, Inline: true},
		{Name: "Expires In", Value: expiresIn, Inline: true},
	}
	return SendNotification(webhookURL, "⚠️ Token Expiring Soon", "A ChatGPT token is about to expire", ColorYellow, fields)
}

// SendSummary sends a batch summary notification.
func SendSummary(webhookURL string, success, failed, total int, duration string) error {
	fields := []DiscordEmbedField{
		{Name: "Success", Value: fmt.Sprintf("%d", success), Inline: true},
		{Name: "Failed", Value: fmt.Sprintf("%d", failed), Inline: true},
		{Name: "Total", Value: fmt.Sprintf("%d", total), Inline: true},
		{Name: "Duration", Value: duration, Inline: false},
	}
	return SendNotification(webhookURL, "📊 Batch Complete", "Registration batch finished", ColorBlue, fields)
}
