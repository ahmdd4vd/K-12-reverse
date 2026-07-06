package email

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/PuerkitoBio/goquery"
	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	"github.com/verssache/chatgpt-creator/internal/util"
)

var (
	blacklistedDomains sync.Map
	blacklistMutex     sync.Mutex
)

func init() {
	data, err := os.ReadFile("blacklist.json")
	if err != nil {
		return // File might not exist yet
	}

	var domains []string
	if err := json.Unmarshal(data, &domains); err != nil {
		return
	}

	for _, domain := range domains {
		blacklistedDomains.Store(domain, true)
	}
}

func saveBlacklist() {
	blacklistMutex.Lock()
	defer blacklistMutex.Unlock()

	var domains []string
	blacklistedDomains.Range(func(key, value any) bool {
		if domain, ok := key.(string); ok {
			domains = append(domains, domain)
		}
		return true
	})

	data, err := json.MarshalIndent(domains, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile("blacklist.json", data, 0644)
}

// AddBlacklistDomain adds a domain to the global blacklist.
func AddBlacklistDomain(domain string) {
	blacklistedDomains.Store(domain, true)
	saveBlacklist()
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// CreateTempEmail fetches a new temp email using a random profile and gofakeit names.
func CreateTempEmail(defaultDomain string) (string, error) {
	// If defaultDomain is set, skip fetching from generator.email
	if defaultDomain != "" {
		firstName := gofakeit.FirstName()
		lastName := gofakeit.LastName()
		email := strings.ToLower(firstName+lastName+util.RandStr(5)) + "@" + defaultDomain
		return email, nil
	}

	options := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(profiles.Chrome_131),
		tls_client.WithTimeoutSeconds(60),
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return "", fmt.Errorf("failed to create tls client: %w", err)
	}

	req, err := fhttp.NewRequest(http.MethodGet, "https://generator.email/", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch generator.email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("generator.email returned status: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	domains := []string{"smartmail.de", "enayu.com", "crazymailing.com"}
	doc.Find(".e7m.tt-suggestions div > p").Each(func(i int, s *goquery.Selection) {
		domain := strings.TrimSpace(s.Text())
		if domain != "" {
			if _, blacklisted := blacklistedDomains.Load(domain); !blacklisted {
				domains = append(domains, domain)
			}
		}
	})

	if len(domains) == 0 {
		return "", fmt.Errorf("all available domains are blacklisted")
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomDomain := domains[r.Intn(len(domains))]

	firstName := gofakeit.FirstName()
	lastName := gofakeit.LastName()
	email := strings.ToLower(firstName+lastName+util.RandStr(5)) + "@" + randomDomain

	return email, nil
}

// GetVerificationCode polls generator.email for the OTP using a custom cookie.
func GetVerificationCode(ctx context.Context, email string, maxRetries int, delay time.Duration) (string, error) {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid email format: %s", email)
	}
	username := parts[0]
	domain := parts[1]

	otpRegex := regexp.MustCompile(`\d{6}`)

	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		options := []tls_client.HttpClientOption{
			tls_client.WithClientProfile(profiles.Chrome_131),
			tls_client.WithTimeoutSeconds(60),
		}

		client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
		if err != nil {
			return "", fmt.Errorf("failed to create tls client: %w", err)
		}

		url := fmt.Sprintf("https://generator.email/%s/%s", domain, username)
		req, err := fhttp.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}

		// Critical: Set request header Cookie: surl={domain}/{username} explicitly.
		req.Header.Set("Cookie", fmt.Sprintf("surl=%s/%s", domain, username))

		resp, err := client.Do(req)
		if err != nil {
			// Log error and continue retrying
			if err := waitContext(ctx, delay); err != nil {
				return "", err
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if err := waitContext(ctx, delay); err != nil {
				return "", err
			}
			continue
		}

		doc, err := goquery.NewDocumentFromReader(resp.Body)
		resp.Body.Close()
		if err != nil {
			if err := waitContext(ctx, delay); err != nil {
				return "", err
			}
			continue
		}

		// Find OTP in #email-table > div.e7m.list-group-item.list-group-item-info > div.e7m.subj_div_45g45gg
		otp := ""
		doc.Find("#email-table > div.e7m.list-group-item.list-group-item-info > div.e7m.subj_div_45g45gg").EachWithBreak(func(i int, s *goquery.Selection) bool {
			text := s.Text()
			matches := otpRegex.FindStringSubmatch(text)
			if len(matches) > 0 {
				code := matches[0]
				// Skip code "177010" explicitly
				if code == "177010" {
					return true // continue to next if any, but we only expect one
				}
				otp = code
				return false // break
			}
			return true
		})

		if otp != "" {
			return otp, nil
		}

		if err := waitContext(ctx, delay); err != nil {
			return "", err
		}
	}

	return "", fmt.Errorf("failed to get verification code after %d retries", maxRetries)
}
