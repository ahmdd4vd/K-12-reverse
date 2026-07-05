package email

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
)

// GmailIMAPConfig holds the Gmail IMAP connection settings.
type GmailIMAPConfig struct {
	Email       string // The base Gmail address (e.g., ahmaddavd273@gmail.com)
	AppPassword string // Google App Password (16 chars)
}

var IMAPMutex sync.Mutex

// GetVerificationCodeViaIMAP connects to Gmail IMAP and retrieves the OTP code from OpenAI emails.
func GetVerificationCodeViaIMAP(cfg GmailIMAPConfig, targetEmail string, maxRetries int, delay time.Duration) (string, error) {
	otpRegex := regexp.MustCompile(`\b(\d{6})\b`)

	for i := 0; i < maxRetries; i++ {
		// The Mutex is now locked BEFORE c.sendOTP() in the worker flow, 
		// so we DO NOT lock it here anymore to prevent deadlocks!
		code, err := fetchOTPFromGmail(cfg, targetEmail, otpRegex)

		if err == nil && code != "" {
			return code, nil
		}
		if err != nil {
			fmt.Printf("[IMAP Check %d/%d] Waiting for OTP email to arrive in inbox...\n", i+1, maxRetries)
		}

		time.Sleep(delay)
	}

	return "", fmt.Errorf("failed to get OTP from Gmail after %d retries", maxRetries)
}

// fetchOTPFromGmail connects to Gmail IMAP and searches for OTP in recent OpenAI emails.
func fetchOTPFromGmail(cfg GmailIMAPConfig, targetEmail string, otpRegex *regexp.Regexp) (string, error) {
	// Connect to Gmail IMAP over TLS
	c, err := client.DialTLS("imap.gmail.com:993", nil)
	if err != nil {
		return "", fmt.Errorf("failed to connect to IMAP: %w", err)
	}
	defer c.Logout()

	// Login with base Gmail + App Password
	if err := c.Login(cfg.Email, cfg.AppPassword); err != nil {
		return "", fmt.Errorf("IMAP login failed: %w", err)
	}

	// Select INBOX
	_, err = c.Select("INBOX", false)
	if err != nil {
		return "", fmt.Errorf("failed to select INBOX: %w", err)
	}

	// Search for emails from the last 2 hours (we filter by 5 mins later, but this reduces search space)
	criteria := imap.NewSearchCriteria()
	criteria.Since = time.Now().Add(-2 * time.Hour)
	criteria.WithoutFlags = []string{imap.SeenFlag}
	uids, err := c.Search(criteria)
	if err != nil {
		return "", fmt.Errorf("search error: %w", err)
	}
	if len(uids) == 0 {
		return "", fmt.Errorf("no unread emails")
	}

	// Limit to the last 20 emails to prevent fetching thousands of unread emails
	if len(uids) > 20 {
		uids = uids[len(uids)-20:]
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uids...)

	// Fetch messages
	messages := make(chan *imap.Message, 100)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{section.FetchItem(), imap.FetchEnvelope, imap.FetchUid, imap.FetchFlags}

	go func() {
		c.Fetch(seqSet, items, messages)
	}()

	// Process messages in reverse order (newest first)
	var allMessages []*imap.Message
	for msg := range messages {
		allMessages = append(allMessages, msg)
	}

	for i := len(allMessages) - 1; i >= 0; i-- {
		msg := allMessages[i]
		if msg == nil || msg.Envelope == nil {
			continue
		}

		// Check if it's from OpenAI
		subject := msg.Envelope.Subject
		isOpenAI := false
		for _, addr := range msg.Envelope.From {
			if strings.Contains(strings.ToLower(addr.HostName), "openai") ||
				strings.Contains(strings.ToLower(addr.HostName), "chatgpt") {
				isOpenAI = true
				break
			}
		}

		subjectLower := strings.ToLower(subject)
		if !isOpenAI && !strings.Contains(subjectLower, "openai") {
			continue
		}

		// STRICT FILTER: Only parse OTP if it's actually an OTP email
		if !strings.Contains(subjectLower, "code") && !strings.Contains(subjectLower, "verify") && !strings.Contains(subjectLower, "verification") && subjectLower != "chatgpt" {
			continue
		}
		// Skip K12 invite emails which might have random 6-digit numbers in the HTML body
		if strings.Contains(subjectLower, "approved") || strings.Contains(subjectLower, "join") {
			continue
		}

		isForTarget := false
		for _, toAddr := range msg.Envelope.To {
			fullAddr := strings.ToLower(toAddr.MailboxName + "@" + toAddr.HostName)
			
			// EXACT MATCH ONLY: Ensures workers don't steal each other's emails
			if strings.EqualFold(fullAddr, targetEmail) {
				isForTarget = true
				break
			}
		}
		
		if !isForTarget {
			continue
		}

		// EXTRA PROTECTION: Only parse emails from the last 5 minutes
		if time.Since(msg.Envelope.Date) > 5*time.Minute {
			continue
		}

		// Check subject for OTP
		matches := regexp.MustCompile(`\b([0-9]{6})\b`).FindStringSubmatch(subject)
		if len(matches) > 1 {
			otp := matches[1]
			if otp != "177010" {
				// Mark as read
				item := imap.FormatFlagsOp(imap.AddFlags, true)
				flags := []interface{}{imap.SeenFlag}
				seq := new(imap.SeqSet)
				seq.AddNum(msg.SeqNum)
				c.Store(seq, item, flags, nil)
				return otp, nil
			}
		}

		// Check body for OTP
		for _, body := range msg.Body {
			if body == nil {
				continue
			}
			
			// We need a string of the body to parse MIME
			mr, err := mail.CreateReader(body)
			if err != nil {
				// Fallback to raw string if mail parser fails (rare)
				// Wait, CreateReader consumes the reader, so we can't easily fallback.
				// But CreateReader works on raw RFC822 messages.
				continue
			}

			// Process the message parts
			for {
				p, err := mr.NextPart()
				if err == io.EOF {
					break
				} else if err != nil {
					break
				}

				switch p.Header.(type) {
				case *mail.InlineHeader:
					// This is the message's text (can be plain-text or HTML)
					b, err := io.ReadAll(p.Body)
					if err != nil {
						continue
					}
					bodyStr := string(b)
					
					// Find all exact 6 digit numbers (we use [^a-zA-Z0-9] to match boundaries 
					// manually since HTML might not have \b where we expect)
					// Find all exact 6 digit numbers using word boundaries and also handling HTML
					re := regexp.MustCompile(`(?:^|[^a-zA-Z0-9])([0-9]{6})(?:[^a-zA-Z0-9]|$)`)
					allMatches := re.FindAllStringSubmatch(bodyStr, -1)
					
					for _, m := range allMatches {
						if len(m) > 1 {
							code := m[1]
							// Ensure it's not the color code or font weight or something from OpenAI HTML
							if code != "202123" && code != "177010" && code != "353740" && code != "140626" {
								// Mark as read
								item := imap.FormatFlagsOp(imap.AddFlags, true)
								flags := []interface{}{imap.SeenFlag}
								seq := new(imap.SeqSet)
								seq.AddNum(msg.SeqNum)
								c.Store(seq, item, flags, nil)
								return code, nil
							}
						}
					}
				}
			}
		}
	}

	return "", fmt.Errorf("no OTP found in recent emails")
}
