package jwtanalyzer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// OpenAIClaims represents the custom claims in a ChatGPT JWT.
type OpenAIClaims struct {
	Sub       string `json:"sub"`
	Email     string `json:"email"`
	Exp       int64  `json:"exp"`
	Iat       int64  `json:"iat"`
	Iss       string `json:"iss"`
	SessionID string `json:"session_id"`
	Auth      *struct {
		AccountID     string `json:"chatgpt_account_id"`
		PlanType      string `json:"chatgpt_plan_type"`
		UserID        string `json:"chatgpt_user_id"`
		ComputeRes    string `json:"chatgpt_compute_residency"`
		IsSignup      bool   `json:"is_signup"`
	} `json:"https://api.openai.com/auth"`
	Profile *struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	} `json:"https://api.openai.com/profile"`
}

// TokenInfo holds the decoded JWT information.
type TokenInfo struct {
	Email       string `json:"email"`
	PlanType    string `json:"planType"`
	UserID      string `json:"userId"`
	AccountID   string `json:"accountId"`
	WorkspaceID string `json:"workspaceId"`
	ExpiresAt   string `json:"expiresAt"`
	IssuedAt    string `json:"issuedAt"`
	IsExpired   bool   `json:"isExpired"`
	ExpiresIn   string `json:"expiresIn"`
	RawSub      string `json:"rawSub"`
	SessionID   string `json:"sessionId"`
}

// Decode decodes a JWT token and returns human-readable info.
func Decode(token string) (*TokenInfo, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	payload := parts[1]
	// Add padding if needed
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// Try standard base64 as fallback
		decoded, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
		}
	}

	var claims OpenAIClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	info := &TokenInfo{}

	info.Email = claims.Email
	if claims.Profile != nil && claims.Profile.Email != "" {
		info.Email = claims.Profile.Email
	}

	if claims.Auth != nil {
		info.PlanType = claims.Auth.PlanType
		info.UserID = claims.Auth.UserID
		info.AccountID = claims.Auth.AccountID
		info.WorkspaceID = claims.Auth.AccountID
	}

	info.RawSub = claims.Sub
	info.SessionID = claims.SessionID

	if claims.Exp > 0 {
		t := time.Unix(claims.Exp, 0)
		info.ExpiresAt = t.Format(time.RFC3339)
		info.IsExpired = time.Now().After(t)
		info.ExpiresIn = formatDuration(time.Until(t))
	}
	if info.ExpiresAt == "" {
		info.ExpiresAt = "unknown"
	}

	if claims.Iat > 0 {
		t := time.Unix(claims.Iat, 0)
		info.IssuedAt = t.Format(time.RFC3339)
	}
	if info.IssuedAt == "" {
		info.IssuedAt = "unknown"
	}

	return info, nil
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		return "expired"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 24 {
		days := h / 24
		h = h % 24
		return fmt.Sprintf("%dd %dh", days, h)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// PrintInfo displays token info in a formatted table.
func PrintInfo(info *TokenInfo) {
	statusIcon := "✅"
	statusText := "Valid"
	if info.IsExpired {
		statusIcon = "❌"
		statusText = "EXPIRED"
	}

	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  %s Token Status: %s\n", statusIcon, statusText)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  Email      : %s\n", info.Email)
	fmt.Printf("  Plan Type  : %s\n", info.PlanType)
	fmt.Printf("  User ID    : %s\n", info.UserID)
	fmt.Printf("  Workspace  : %s\n", info.WorkspaceID)
	fmt.Printf("  Expires At : %s (%s)\n", info.ExpiresAt, info.ExpiresIn)
	fmt.Printf("  Issued At  : %s\n", info.IssuedAt)
	fmt.Printf("  Session ID : %s\n", info.SessionID)
	fmt.Println(strings.Repeat("─", 50))
}
