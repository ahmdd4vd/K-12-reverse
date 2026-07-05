package register

import (
	"encoding/json"
	"fmt"
	"io"
	http "github.com/bogdanfinn/fhttp"
	"net/url"
	"strings"
	"time"

	"github.com/verssache/chatgpt-creator/internal/email"
	"github.com/verssache/chatgpt-creator/internal/util"
)

// RunLogin attempts to log in to an existing account and extract its access token.
func (c *Client) RunLogin(emailAddr, password string, k12WorkspaceIDs []string, gmailIMAP *email.GmailIMAPConfig) (*TokenResult, error) {
	c.print(fmt.Sprintf("Starting login flow for existing account: %s", emailAddr))

	if err := c.visitHomepage(); err != nil {
		return nil, err
	}
	c.randomDelay(0.3, 0.8)

	csrf, err := c.getCSRF()
	if err != nil {
		return nil, err
	}
	c.randomDelay(0.2, 0.5)

	authURL, err := c.signin(emailAddr, csrf, "login_or_signup")
	if err != nil {
		return nil, err
	}
	c.randomDelay(0.3, 0.8)

	finalURL, err := c.authorize(authURL)
	if err != nil {
		return nil, err
	}
	c.randomDelay(0.3, 0.8)

	u, _ := url.Parse(finalURL)
	finalPath := u.Path

	var cbURL string
	needOTP := false

	c.print(fmt.Sprintf("Final Login URL: %s", finalURL))
	
	if strings.Contains(finalPath, "login/password") || strings.Contains(finalPath, "create-account/password") || strings.Contains(finalPath, "log-in/password") {
		c.print("Entering password for login...")
		
		state := u.Query().Get("state")
		c.print(fmt.Sprintf("Login State Parameter: %s", state))
		status, data, err := c.loginPassword(emailAddr, password, state)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, fmt.Errorf("password login failed (%d): %v", status, data)
		}
		
		if u, ok := data["redirect_url"].(string); ok {
			cbURL = u
		} else if u, ok := data["continue_url"].(string); ok {
			cbURL = u
		}
		
		// Sometimes it might require OTP after password
		if cbURL != "" && (strings.Contains(cbURL, "email-verification") || strings.Contains(cbURL, "email-otp")) {
			c.print("Login requires 2FA / OTP verification.")
			needOTP = true
			cbURL = "" // reset
		}
	} else if strings.Contains(finalPath, "email-verification") || strings.Contains(finalPath, "email-otp") {
		c.print("Passwordless login detected. Jump to OTP verification stage")
		needOTP = true
	} else if strings.Contains(finalPath, "callback") || strings.Contains(finalURL, "chatgpt.com") {
		cbURL = finalURL
	} else {
		c.print(fmt.Sprintf("Unknown login jump: %s", finalURL))
		return nil, fmt.Errorf("unexpected login redirect: %s", finalPath)
	}

	if needOTP {
		var otpCode string
		var err error

		if gmailIMAP != nil {
			c.sendOTP()
			
			c.print("Waiting for Login OTP via IMAP. Reading Gmail inbox...")
			otpCode, err = email.GetVerificationCodeViaIMAP(*gmailIMAP, emailAddr, 15, 4*time.Second)
			
			if err != nil {
				return nil, fmt.Errorf("failed to auto-read OTP: %v", err)
			}
			c.print(fmt.Sprintf("Received OTP automatically: %s", otpCode))
		} else {
			c.sendOTP()
			otpCode, err = email.GetVerificationCode(emailAddr, 20, 3*time.Second)
			if err != nil {
				return nil, err
			}
		}

		c.randomDelay(0.3, 0.8)
		status, data, err := c.validateOTP(otpCode)
		if err != nil {
			return nil, err
		}

		if status != 200 {
			return nil, fmt.Errorf("login OTP verification failed (%d): %v", status, data)
		}
		
		c.print(fmt.Sprintf("Validate OTP Data: %v", data))
		if u, ok := data["redirect_url"].(string); ok {
			cbURL = u
		} else if u, ok := data["continue_url"].(string); ok {
			cbURL = u
		}

		// Let's see if we can extract a better callback URL by visiting about-you
		if cbURL != "" && strings.Contains(cbURL, "about-you") {
			c.print("Login redirected to about-you. Trying to bypass it...")
			finalURL, err := c.authorize(cbURL)
			if err == nil && strings.Contains(finalURL, "callback") {
				cbURL = finalURL
			}
		}
	}

	if cbURL == "" {
		return nil, fmt.Errorf("failed to extract callback url from login")
	}

	if !strings.HasPrefix(cbURL, "http") {
		cbURL = "https://auth.openai.com" + cbURL
	}

	c.randomDelay(0.2, 0.5)
	c.callback(cbURL)

	c.print("✅ Login completed! Extracting tokens...")
	if len(k12WorkspaceIDs) > 0 {
		c.randomDelay(1.0, 2.0)
		return c.RunK12Flow(k12WorkspaceIDs, emailAddr, gmailIMAP)
	}
	return nil, nil
}

// loginPassword submits the password for an existing account
func (c *Client) loginPassword(email, password, state string) (int, map[string]interface{}, error) {
	loginURL := authURL + "/u/login/password"
	if state != "" {
		loginURL += "?state=" + state
	}
	
	payload := map[string]string{
		"username": email,
		"password": password,
		"action":   "default",
	}
	jsonPayload, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", loginURL, strings.NewReader(string(jsonPayload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	refererURL := authURL + "/u/login/password"
	if state != "" {
		refererURL += "?state=" + state
	}
	req.Header.Set("Referer", refererURL)
	req.Header.Set("Origin", authURL)

	traceHeaders := util.MakeTraceHeaders()
	for k, v := range traceHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	json.Unmarshal(body, &data)

	c.log("Login Password", resp.StatusCode)
	c.print(fmt.Sprintf("Login Password Data: %v", data))
	return resp.StatusCode, data, nil
}
