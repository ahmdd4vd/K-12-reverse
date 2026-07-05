package register

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/verssache/chatgpt-creator/internal/email"
	"github.com/verssache/chatgpt-creator/internal/sentinel"
	"github.com/verssache/chatgpt-creator/internal/util"
)

// visitHomepage visits chatgpt.com to initialize session
func (c *Client) visitHomepage() error {
	var resp *http.Response
	var err error
	for retry := 0; retry < 3; retry++ {
		req, _ := http.NewRequest("GET", baseURL+"/", nil)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
		req.Header.Set("Upgrade-Insecure-Requests", "1")

		resp, err = c.do(req)
		if err != nil {
			return err
		}

		c.log(fmt.Sprintf("Visit Homepage (Try %d)", retry+1), resp.StatusCode)

		if resp.StatusCode == 200 || resp.StatusCode == 302 || resp.StatusCode == 307 {
			resp.Body.Close()
			return nil
		}
		resp.Body.Close()
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("failed to visit homepage after 3 retries (status: %d)", resp.StatusCode)
}

// getCSRF retrieves the CSRF token from chatgpt.com
func (c *Client) getCSRF() (string, error) {
	req, _ := http.NewRequest("GET", baseURL+"/api/auth/csrf", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", baseURL+"/")

	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var data struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}

	c.log("Get CSRF", resp.StatusCode)
	if data.CSRFToken == "" {
		return "", fmt.Errorf("csrf token not found")
	}
	return data.CSRFToken, nil
}

// signin initiates the signin process and returns the authorize URL
func (c *Client) signin(email, csrf, mode string) (string, error) {
	if mode == "" {
		mode = "login_or_signup"
	}
	signinURL := baseURL + "/api/auth/signin/openai"
	params := url.Values{}
	params.Set("prompt", "login")
	params.Set("ext-oai-did", c.deviceID)
	params.Set("auth_session_logging_id", util.GenerateUUID())
	params.Set("screen_hint", mode)
	params.Set("login_hint", email)

	fullURL := signinURL + "?" + params.Encode()

	formData := url.Values{}
	formData.Set("callbackUrl", baseURL+"/")
	formData.Set("csrfToken", csrf)
	formData.Set("json", "true")

	req, _ := http.NewRequest("POST", fullURL, strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", baseURL+"/")
	req.Header.Set("Origin", baseURL)

	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var data struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}

	c.log("Signin", resp.StatusCode)
	if data.URL == "" {
		return "", fmt.Errorf("authorize url not found")
	}
	return data.URL, nil
}

// authorize visits the authorize URL and returns the final redirect URL
func (c *Client) authorize(authURL string) (string, error) {
	req, _ := http.NewRequest("GET", authURL, nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", baseURL+"/")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()
	c.log("Authorize", resp.StatusCode)
	return finalURL, nil
}

// register registers the user with email and password
func (c *Client) register(email, password string) (int, map[string]interface{}, error) {
	regURL := authURL + "/api/accounts/user/register"
	payload := map[string]string{
		"username": email,
		"password": password,
	}
	jsonPayload, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", regURL, strings.NewReader(string(jsonPayload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", authURL+"/create-account/password")
	req.Header.Set("Origin", authURL)

	// Add trace headers if available in util
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

	c.log("Register", resp.StatusCode)
	return resp.StatusCode, data, nil
}

// sendOTP sends the OTP to the user's email
func (c *Client) sendOTP() (int, map[string]interface{}, error) {
	otpURL := authURL + "/api/accounts/email-otp/send"
	req, _ := http.NewRequest("GET", otpURL, nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", authURL+"/create-account/password")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := c.do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		data = map[string]interface{}{"text": string(body)}
	}

	c.log("Send OTP", resp.StatusCode)
	return resp.StatusCode, data, nil
}

// validateOTP validates the OTP code
func (c *Client) validateOTP(code string) (int, map[string]interface{}, error) {
	valURL := authURL + "/api/accounts/email-otp/validate"
	payload := map[string]string{"code": code}
	jsonPayload, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", valURL, strings.NewReader(string(jsonPayload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", authURL+"/email-verification")
	req.Header.Set("Origin", "https://auth.openai.com")

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

	c.log(fmt.Sprintf("Validate OTP [%s]", code), resp.StatusCode)
	return resp.StatusCode, data, nil
}

// createAccount creates the user account with name and birthdate
func (c *Client) createAccount(name, birthdate string) (int, map[string]interface{}, error) {
	createURL := authURL + "/api/accounts/create_account"
	payload := map[string]string{
		"name":      name,
		"birthdate": birthdate,
	}
	jsonPayload, _ := json.Marshal(payload)

	sentinelCreateAccount, err := sentinel.BuildSentinelToken(c.session, c.deviceID, "create_account", c.ua, c.secChUA, c.impersonate)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get sentinel auth: %v", err)
	}

	req, _ := http.NewRequest("POST", createURL, strings.NewReader(string(jsonPayload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", authURL+"/about-you")
	req.Header.Set("Origin", authURL)
	req.Header.Set("openai-sentinel-token", sentinelCreateAccount)

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

	c.log("Create Account", resp.StatusCode)
	return resp.StatusCode, data, nil
}

// callback handles the callback URL
func (c *Client) callback(cbURL string) (int, map[string]interface{}, error) {
	if cbURL == "" {
		return 0, nil, fmt.Errorf("empty callback url")
	}

	req, _ := http.NewRequest("GET", cbURL, nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := c.do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	c.log("Callback", resp.StatusCode)
	return resp.StatusCode, map[string]interface{}{"final_url": resp.Request.URL.String()}, nil
}

func (c *Client) RunRegister(emailAddr, password, name, birthdate string, k12WorkspaceIDs []string, gmailIMAP *email.GmailIMAPConfig) (*TokenResult, error) {
	c.print("Starting registration flow...")

	if err := c.visitHomepage(); err != nil {
		return nil, err
	}
	c.randomDelay(0.3, 0.8)

	csrf, err := c.getCSRF()
	if err != nil {
		return nil, err
	}
	c.randomDelay(0.2, 0.5)

	authURL, err := c.signin(emailAddr, csrf, "signup")
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

	needOTP := false
	needsManualOTPRequest := false
	var otpCbURL string

	if strings.Contains(finalPath, "create-account/password") {
		c.randomDelay(0.5, 1.0)
		status, data, err := c.register(emailAddr, password)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, fmt.Errorf("register failed (%d): %v", status, data)
		}
		c.randomDelay(0.3, 0.8)
		// c.sendOTP() is handled in the block below instead
		needOTP = true
		needsManualOTPRequest = true
	} else if strings.Contains(finalPath, "email-verification") || strings.Contains(finalPath, "email-otp") {
		c.print("Jump to OTP verification stage")
		// c.sendOTP() // OpenAI already sends the OTP automatically when jumping here
		needOTP = true
	} else if strings.Contains(finalPath, "about-you") {
		c.print("Jump to fill information stage")
		c.randomDelay(0.5, 1.0)
		status, data, err := c.createAccount(name, birthdate)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, fmt.Errorf("create account failed (%d): %v", status, data)
		}
		c.randomDelay(0.3, 0.5)

		if u, ok := data["continue_url"].(string); ok {
			otpCbURL = u
		} else if u, ok := data["url"].(string); ok {
			otpCbURL = u
		} else if u, ok := data["redirect_url"].(string); ok {
			otpCbURL = u
		}
		c.callback(otpCbURL)

		// K12 invite + token extraction
		if len(k12WorkspaceIDs) > 0 {
			c.randomDelay(1.0, 2.0)
			return c.RunK12Flow(k12WorkspaceIDs, emailAddr, gmailIMAP)
		}
		return nil, nil
	} else if strings.Contains(finalPath, "callback") || strings.Contains(finalURL, "chatgpt.com") {
		c.print("Account registration completed")

		// K12 invite + token extraction
		if len(k12WorkspaceIDs) > 0 {
			c.randomDelay(1.0, 2.0)
			return c.RunK12Flow(k12WorkspaceIDs, emailAddr, gmailIMAP)
		}
		return nil, nil
	} else if strings.Contains(finalPath, "error") || strings.Contains(finalURL, "error") {
		c.print(fmt.Sprintf("Auth Error jump: %s", finalURL))
		return nil, fmt.Errorf("auth rate limit or error encountered: please change IP/Proxy or try again later")
	} else {
		c.print(fmt.Sprintf("Unknown jump: %s", finalURL))
		return nil, fmt.Errorf("unexpected register redirect: %s", finalPath)
	}

	if needOTP {
		var otpCode string
		var err error

		if gmailIMAP != nil {
			if needsManualOTPRequest {
				c.sendOTP()
			}
			
			c.print("Waiting for OTP via IMAP. Reading Gmail inbox...")
			otpCode, err = email.GetVerificationCodeViaIMAP(*gmailIMAP, emailAddr, 15, 4*time.Second)
			
			if err != nil {
				return nil, fmt.Errorf("failed to auto-read OTP: %v", err)
			}
			c.print(fmt.Sprintf("Received OTP automatically: %s", otpCode))
		} else {
			if needsManualOTPRequest {
				c.sendOTP()
			}
			// Temp email mode: scrape OTP from generator.email
			otpCode, err = email.GetVerificationCode(emailAddr, 20, 3*time.Second)
		}
		if err != nil {
			return nil, err
		}

		c.randomDelay(0.3, 0.8)
		status, data, err := c.validateOTP(otpCode)
		if err != nil {
			return nil, err
		}

		if status != 200 {
			c.print("Verification code failed, retrying...")
			if gmailIMAP != nil {
				c.sendOTP()
				c.randomDelay(1.0, 2.0)
				c.print("Waiting for OTP retry via IMAP...")
				otpCode, err = email.GetVerificationCodeViaIMAP(*gmailIMAP, emailAddr, 15, 4*time.Second)
			} else {
				c.sendOTP()
				c.randomDelay(1.0, 2.0)
				otpCode, err = email.GetVerificationCode(emailAddr, 10, 3*time.Second)
			}
			if err != nil {
				return nil, err
			}
			c.randomDelay(0.3, 0.8)
			status, data, err = c.validateOTP(otpCode)
			if err != nil {
				return nil, err
			}
			if status != 200 {
				return nil, fmt.Errorf("verification code failed after retry (%d): %v", status, data)
			}
		}
		
		c.print(fmt.Sprintf("Validate OTP Data: %v", data))

		if u, ok := data["redirect_url"].(string); ok {
			otpCbURL = u
		} else if u, ok := data["continue_url"].(string); ok {
			otpCbURL = u
		}
	}

	var cbURL string
	if otpCbURL != "" && strings.Contains(otpCbURL, "callback") {
		c.print("🚀 Account recovered from Zombie state via OTP!")
		cbURL = otpCbURL
	} else {
		// After OTP validation, proceed to createAccount
		c.randomDelay(0.5, 1.5)
		status, accountData, err := c.createAccount(name, birthdate)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			errStr := fmt.Sprintf("%v", accountData)
			if strings.Contains(errStr, "user_already_exists") {
				// This email was previously partially registered (email verified but profile never completed).
				// Even in a browser this is a dead-end - Auth0 can't create or login.
				// Skip this email and move to the next one.
				c.print("⚠ SKIP: Account is a zombie (partially registered, can't login or create). Trying next email...")
				return nil, fmt.Errorf("SKIP_EMAIL: account already exists for %s, cannot recover", emailAddr)
			}
			return nil, fmt.Errorf("create account failed (%d): %v", status, accountData)
		}

		if u, ok := accountData["continue_url"].(string); ok {
			cbURL = u
		} else if u, ok := accountData["url"].(string); ok {
			cbURL = u
		} else if u, ok := accountData["redirect_url"].(string); ok {
			cbURL = u
		}
	}

	if cbURL != "" && !strings.HasPrefix(cbURL, "http") {
		cbURL = "https://auth.openai.com" + cbURL
	}

	c.randomDelay(0.2, 0.5)
	c.callback(cbURL)

	// K12 invite + token extraction after full registration
	if len(k12WorkspaceIDs) > 0 {
		c.randomDelay(1.0, 2.0)
		return c.RunK12Flow(k12WorkspaceIDs, emailAddr, gmailIMAP)
	}

	return nil, nil
}

func (c *Client) randomDelay(low, high float64) {
	delay := low + rand.Float64()*(high-low)
	time.Sleep(time.Duration(delay * float64(time.Second)))
}

