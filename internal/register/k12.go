package register

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	http "github.com/bogdanfinn/fhttp"
	"github.com/verssache/chatgpt-creator/internal/email"
)

// TokenResult holds the extracted tokens for bulk export.
type TokenResult struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	IdToken      string `json:"idToken"`
	Email        string `json:"email"`
	Password     string `json:"password"`
}

// sessionResponse represents the ChatGPT session API response.
type sessionResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	IdToken      string `json:"idToken"`
	Expires      string `json:"expires"`
	User         struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	} `json:"user"`
}

// getSession fetches the session from chatgpt.com/api/auth/session
func (c *Client) getSession() (*sessionResponse, error) {
	req, _ := http.NewRequest("GET", baseURL+"/api/auth/session", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", baseURL+"/")

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch session: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var session sessionResponse
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session: %w (body: %s)", err, string(body))
	}

	if session.AccessToken == "" {
		return nil, fmt.Errorf("no access token in session response")
	}

	c.log("Get Session", resp.StatusCode)
	return &session, nil
}

// requestK12Invite sends a K12 workspace invite request.
func (c *Client) requestK12Invite(accessToken, workspaceID string) (bool, error) {
	inviteURL := fmt.Sprintf("%s/backend-api/accounts/%s/invites/request", baseURL, workspaceID)

	req, _ := http.NewRequest("POST", inviteURL, nil)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Oai-Language", "en-US")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Referer", baseURL+"/k12-verification")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := c.do(req)
	if err != nil {
		return false, fmt.Errorf("k12 invite request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	c.log(fmt.Sprintf("K12 Invite [%s]", workspaceID[:8]), resp.StatusCode)

	if resp.StatusCode == 200 {
		if success, ok := result["success"].(bool); ok && success {
			return true, nil
		}
	}

	detail := ""
	if d, ok := result["detail"].(string); ok {
		detail = d
	}
	return false, fmt.Errorf("k12 invite failed (%d): %s", resp.StatusCode, detail)
}

// switchWorkspace switches the active ChatGPT workspace/account.
// After K12 invite, the account has 2 workspaces (personal + K12).
// We need to switch to K12 workspace before fetching session to get K12 plan tokens.
func (c *Client) switchWorkspace(accessToken, accountID string) error {
	// Method 1: Use the account check endpoint to activate the workspace
	switchURL := fmt.Sprintf("%s/backend-api/accounts/check/v4-2023-04-27", baseURL)

	req, _ := http.NewRequest("GET", switchURL, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Oai-Language", "en-US")
	req.Header.Set("Chatgpt-Account-Id", accountID)
	req.Header.Set("Referer", baseURL+"/")

	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("failed to switch workspace: %w", err)
	}
	defer resp.Body.Close()

	c.log(fmt.Sprintf("Switch Workspace [%s]", accountID[:8]), resp.StatusCode)

	// Method 2: Also set the active account via cookie/header for session endpoint
	// Visit the main page with the account ID header to activate the workspace session
	reqMain, _ := http.NewRequest("GET", baseURL+"/", nil)
	reqMain.Header.Set("Accept", "text/html")
	reqMain.Header.Set("Chatgpt-Account-Id", accountID)
	reqMain.Header.Set("Upgrade-Insecure-Requests", "1")

	respMain, err := c.do(reqMain)
	if err == nil {
		respMain.Body.Close()
	}

	return nil
}

// getSessionWithAccount fetches the session with a specific account/workspace ID active.
func (c *Client) getSessionWithAccount(accountID string) (*sessionResponse, error) {
	sessionURL := baseURL + "/api/auth/session"
	if accountID != "" {
		sessionURL = fmt.Sprintf("%s/api/auth/session?exchange_workspace_token=true&workspace_id=%s&reason=setCurrentAccount", baseURL, accountID)
	}
	req, _ := http.NewRequest("GET", sessionURL, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", baseURL+"/")
	if accountID != "" {
		req.Header.Set("Chatgpt-Account-Id", accountID)
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch session: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var session sessionResponse
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session: %w (body: %s)", err, string(body))
	}

	if session.AccessToken == "" {
		return nil, fmt.Errorf("no access token in session response")
	}

	c.log("Get Session (K12)", resp.StatusCode)
	return &session, nil
}

// RunK12Flow performs the K12 invite and token extraction after account creation.
func (c *Client) RunK12Flow(ctx context.Context, workspaceIDs []string, emailAddr string, gmailIMAP *email.GmailIMAPConfig) (*TokenResult, error) {
	c.print("Starting K12 invite flow...")
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	session, err := c.getSession()
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	invited := false
	invitedWsID := ""
	for _, wsID := range workspaceIDs {
		wsID = strings.TrimSpace(wsID)
		if wsID == "" {
			continue
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}
		success, err := c.requestK12Invite(session.AccessToken, wsID)
		if success {
			c.print(fmt.Sprintf("✓ K12 invite request SUCCESS: %s", wsID))
			invited = true
			invitedWsID = wsID
			break
		}
		if err != nil {
			c.print(fmt.Sprintf("✗ K12 invite failed [%s]: %v", wsID[:8], err))
		}
		if err := c.randomDelay(ctx, 0.3, 0.8); err != nil {
			return nil, err
		}
	}

	if !invited {
		c.print("⚠ All K12 invites failed, saving free-tier tokens")
	} else {
		c.print("✓ K12 invite successful, no email verification needed for K12.")
	}

	if invited {
		c.print("Switching to K12 workspace...")
		if err := c.randomDelay(ctx, 2.0, 2.0); err != nil {
			return nil, err
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.switchWorkspace(session.AccessToken, invitedWsID)
		if err := c.randomDelay(ctx, 1.0, 2.0); err != nil {
			return nil, err
		}

		newSession, err := c.getSessionWithAccount(invitedWsID)
		if err == nil {
			session = newSession
			c.print("✓ Switched to K12 workspace successfully")
		} else {
			c.print(fmt.Sprintf("⚠ Failed to switch workspace, trying regular session: %v", err))
			newSession, err := c.getSession()
			if err == nil {
				session = newSession
			}
		}
	}

	// Step 4: Build token result
	refreshToken := session.RefreshToken
	if refreshToken == "" {
		refreshToken = "not available"
	}

	idToken := session.IdToken
	if idToken == "" {
		idToken = session.AccessToken
	}

	result := &TokenResult{
		AccessToken:  session.AccessToken,
		RefreshToken: refreshToken,
		IdToken:      idToken,
		Email:        session.User.Email,
	}

	c.print(fmt.Sprintf("✓ Tokens extracted for %s", session.User.Email))
	return result, nil
}
