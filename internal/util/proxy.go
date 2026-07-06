package util

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"

	"github.com/verssache/chatgpt-creator/internal/chrome"
)

// FormatProxy parses various proxy string formats and normalizes them into protocol://username:password@host:port
// Supported formats:
// 1. Agreement://Host IP:Port:Username:Password (e.g. socks5://192.168.1.1:8080:user:pass)
// 2. Host IP:Port:Username:Password
// 3. Username:Password@Host IP:Port
// 4. Username:Password:Host IP:Port
// 5. Host IP:Port@Username:Password
func FormatProxy(rawProxy string) string {
	rawProxy = strings.TrimSpace(rawProxy)
	if rawProxy == "" {
		return ""
	}

	protocol := "http"
	parts := strings.SplitN(rawProxy, "://", 2)
	if len(parts) == 2 {
		protocol = parts[0]
		rawProxy = parts[1]
	}

	var user, pass, host, port string

	// Helper to check if a string looks like a port
	isPort := func(s string) bool {
		p, err := strconv.Atoi(s)
		return err == nil && p > 0 && p <= 65535
	}

	if strings.Contains(rawProxy, "@") {
		atParts := strings.SplitN(rawProxy, "@", 2)
		part1 := atParts[0]
		part2 := atParts[1]

		p1Parts := strings.SplitN(part1, ":", 2)
		if len(p1Parts) == 2 && isPort(p1Parts[1]) && (strings.Contains(p1Parts[0], ".") || p1Parts[0] == "localhost") {
			// Host:Port@Username:Password
			host = p1Parts[0]
			port = p1Parts[1]
			p2Parts := strings.SplitN(part2, ":", 2)
			if len(p2Parts) == 2 {
				user = p2Parts[0]
				pass = p2Parts[1]
			} else {
				user = part2
			}
		} else {
			// Username:Password@Host:Port (Standard)
			p1Parts := strings.SplitN(part1, ":", 2)
			if len(p1Parts) == 2 {
				user = p1Parts[0]
				pass = p1Parts[1]
			} else {
				user = part1
			}
			p2Parts := strings.SplitN(part2, ":", 2)
			if len(p2Parts) == 2 {
				host = p2Parts[0]
				port = p2Parts[1]
			} else {
				host = part2
			}
		}
	} else {
		colonParts := strings.Split(rawProxy, ":")
		if len(colonParts) == 4 {
			if isPort(colonParts[1]) && (strings.Contains(colonParts[0], ".") || colonParts[0] == "localhost") {
				// Host:Port:Username:Password
				host = colonParts[0]
				port = colonParts[1]
				user = colonParts[2]
				pass = colonParts[3]
			} else if isPort(colonParts[3]) && (strings.Contains(colonParts[2], ".") || colonParts[2] == "localhost") {
				// Username:Password:Host:Port
				user = colonParts[0]
				pass = colonParts[1]
				host = colonParts[2]
				port = colonParts[3]
			} else {
				// Fallback to Host:Port:Username:Password
				host = colonParts[0]
				port = colonParts[1]
				user = colonParts[2]
				pass = colonParts[3]
			}
		} else if len(colonParts) == 2 {
			// Host:Port
			host = colonParts[0]
			port = colonParts[1]
		} else {
			if !strings.Contains(rawProxy, "://") {
				return fmt.Sprintf("%s://%s", protocol, rawProxy)
			}
			return rawProxy
		}
	}

	if user != "" || pass != "" {
		return fmt.Sprintf("%s://%s:%s@%s:%s", protocol, user, pass, host, port)
	}
	if host != "" && port != "" {
		return fmt.Sprintf("%s://%s:%s", protocol, host, port)
	}

	return fmt.Sprintf("%s://%s", protocol, rawProxy)
}

func CheckProxyAccess(proxy string) (int, error) {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return 0, nil
	}

	formattedProxy := FormatProxy(proxy)
	profile, _, ua := chrome.RandomChromeVersion()
	mappedProfile := chrome.MapToTLSProfile(profile.Impersonate)

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(),
		tls_client.WithClientProfile(mappedProfile),
		tls_client.WithCookieJar(tls_client.NewCookieJar()),
		tls_client.WithTimeoutSeconds(15),
		tls_client.WithProxyUrl(formattedProxy),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create proxy client: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://chatgpt.com/", nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create proxy check request: %w", err)
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("sec-ch-ua", profile.SecChUA)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if err := proxyStatusError(resp.StatusCode); err != nil {
		return resp.StatusCode, err
	}

	req, err = http.NewRequest(http.MethodGet, "https://chatgpt.com/api/auth/csrf", nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create proxy CSRF check request: %w", err)
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://chatgpt.com/")
	req.Header.Set("sec-ch-ua", profile.SecChUA)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)

	resp, err = client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("proxy CSRF request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("failed to read CSRF response body: %w", err)
	}

	if err := proxyStatusError(resp.StatusCode); err != nil {
		return resp.StatusCode, err
	}
	if err := proxyCSRFBodyError(body); err != nil {
		return resp.StatusCode, err
	}
	return resp.StatusCode, nil
}

func proxyStatusError(status int) error {
	if status >= 200 && status < 400 {
		return nil
	}
	if status == http.StatusForbidden {
		return fmt.Errorf("ChatGPT/OpenAI returned 403")
	}
	return fmt.Errorf("unexpected HTTP status %d", status)
}

func proxyCSRFBodyError(body []byte) error {
	var data struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("CSRF endpoint returned non-JSON/invalid body: %w", err)
	}
	if strings.TrimSpace(data.CSRFToken) == "" {
		return fmt.Errorf("CSRF endpoint returned JSON without csrfToken")
	}
	return nil
}
