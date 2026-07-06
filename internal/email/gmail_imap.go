package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
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

// GetLatestUID connects to Gmail IMAP and returns the highest UID currently in the inbox.
// This is used to establish a baseline before requesting a new OTP.
func GetLatestUID(cfg GmailIMAPConfig) (uint32, error) {
	c, err := client.DialTLS("imap.gmail.com:993", nil)
	if err != nil {
		return 0, fmt.Errorf("failed to connect to IMAP: %w", err)
	}
	defer c.Logout()

	if err := c.Login(cfg.Email, cfg.AppPassword); err != nil {
		return 0, fmt.Errorf("IMAP login failed: %w", err)
	}

	mbox, err := c.Select("INBOX", true)
	if err != nil {
		return 0, fmt.Errorf("failed to select INBOX: %w", err)
	}

	// The highest UID in the mailbox is mbox.UidNext - 1 (approximately)
	// But to be completely safe, we can search all messages and find the max UID.
	criteria := imap.NewSearchCriteria()
	criteria.Since = time.Now().Add(-1 * time.Hour) // only check last hour to be fast
	uids, err := c.Search(criteria)
	if err != nil || len(uids) == 0 {
		// Fallback to UidNext - 1 if search fails or inbox is empty in the last hour
		if mbox.UidNext > 1 {
			return mbox.UidNext - 1, nil
		}
		return 0, nil
	}

	var maxUID uint32
	for _, uid := range uids {
		if uid > maxUID {
			maxUID = uid
		}
	}
	return maxUID, nil
}

// GetVerificationCodeViaIMAP connects to Gmail IMAP and retrieves the OTP code from OpenAI emails.
// It only considers emails with a UID greater than startUID.
func GetVerificationCodeViaIMAP(ctx context.Context, cfg GmailIMAPConfig, targetEmail string, startUID uint32, maxRetries int, delay time.Duration) (string, error) {
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// The Mutex is now locked BEFORE c.sendOTP() in the worker flow,
		// so we DO NOT lock it here anymore to prevent deadlocks!
		code, err := fetchOTPFromGmail(cfg, targetEmail, startUID)

		if err == nil && code != "" {
			return code, nil
		}
		if err != nil {
			if strings.Contains(err.Error(), "login failed") || strings.Contains(err.Error(), "Login failed") || strings.Contains(err.Error(), "credential") {
				return "", err
			}
			fmt.Printf("[IMAP Check %d/%d] Waiting for OTP email to arrive in inbox...\n", i+1, maxRetries)
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}

	return "", fmt.Errorf("failed to get OTP from Gmail after %d retries", maxRetries)
}

// fetchOTPFromGmail connects to Gmail IMAP and searches for OTP in recent OpenAI emails.
// Only emails with a UID greater than startUID are considered.
func fetchOTPFromGmail(cfg GmailIMAPConfig, targetEmail string, startUID uint32) (string, error) {
	// Connect to Gmail IMAP over TLS
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	c, err := client.DialWithDialerTLS(dialer, "imap.gmail.com:993", &tls.Config{ServerName: "imap.gmail.com"})
	if err != nil {
		return "", fmt.Errorf("failed to connect to IMAP: %w", err)
	}
	c.Timeout = 30 * time.Second
	defer c.Logout()

	// Login with base Gmail + App Password
	if err := c.Login(cfg.Email, cfg.AppPassword); err != nil {
		return "", fmt.Errorf("IMAP login failed: %w", err)
	}

	// Select INBOX (read-write so we can mark as Seen)
	_, err = c.Select("INBOX", false)
	if err != nil {
		return "", fmt.Errorf("failed to select INBOX: %w", err)
	}

	// Search for ALL emails from the last 2 hours
	criteria := imap.NewSearchCriteria()
	criteria.Since = time.Now().Add(-2 * time.Hour)
	uids, err := c.Search(criteria)
	if err != nil {
		return "", fmt.Errorf("search error: %w", err)
	}
	if len(uids) == 0 {
		return "", fmt.Errorf("no emails")
	}

	// Filter uids to only include those greater than startUID
	var filteredUIDs []uint32
	for _, uid := range uids {
		if uid > startUID {
			filteredUIDs = append(filteredUIDs, uid)
		}
	}

	if len(filteredUIDs) == 0 {
		return "", fmt.Errorf("no new emails since UID %d", startUID)
	}

	// Limit to prevent fetching thousands
	if len(filteredUIDs) > 30 {
		filteredUIDs = filteredUIDs[len(filteredUIDs)-30:]
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(filteredUIDs...)

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

	bodyOTPRegex := regexp.MustCompile(`(?:^|[^a-zA-Z0-9])([0-9]{6})(?:[^a-zA-Z0-9]|$)`)

	for i := len(allMessages) - 1; i >= 0; i-- {
		msg := allMessages[i]
		if msg == nil || msg.Envelope == nil {
			continue
		}

		// Double check UID constraint
		if msg.Uid <= startUID {
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

		// EXACT To-address matching: only match the specific dot-trick variant
		isForTarget := false
		for _, toAddr := range msg.Envelope.To {
			fullAddr := strings.ToLower(toAddr.MailboxName + "@" + toAddr.HostName)

			// Exact match (preserving dots)
			if strings.EqualFold(fullAddr, targetEmail) {
				isForTarget = true
				break
			}

			// For dot-tricks, OpenAI preserves the exact dots in the To header.
			// Only use normalized comparison as a fallback for edge cases.
			normalizedFullAddr := strings.ReplaceAll(fullAddr, ".", "")
			normalizedTargetEmail := strings.ReplaceAll(strings.ToLower(targetEmail), ".", "")
			if normalizedFullAddr == normalizedTargetEmail {
				isForTarget = true
				break
			}
		}

		if !isForTarget {
			continue
		}

		// Check subject for OTP
		matches := regexp.MustCompile(`\b([0-9]{6})\b`).FindStringSubmatch(subject)
		if len(matches) > 1 {
			otp := matches[1]
			if otp != "177010" {
				// Mark as read
				markAsRead(c, msg.SeqNum)
				return otp, nil
			}
		}

		// Check body for OTP
		for _, body := range msg.Body {
			if body == nil {
				continue
			}

			mr, err := mail.CreateReader(body)
			if err != nil {
				// Fallback: try to find OTP in raw body
				raw, readErr := io.ReadAll(body)
				if readErr == nil {
					rawStr := string(raw)
					code := extractOTPFromText(bodyOTPRegex, rawStr)
					if code != "" {
						markAsRead(c, msg.SeqNum)
						return code, nil
					}
				}
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
					b, err := io.ReadAll(p.Body)
					if err != nil {
						continue
					}
					bodyStr := string(b)

					code := extractOTPFromText(bodyOTPRegex, bodyStr)
					if code != "" {
						markAsRead(c, msg.SeqNum)
						return code, nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("no OTP found in recent emails")
}

// extractOTPFromText extracts a 6-digit OTP from text, filtering out known CSS color codes
func extractOTPFromText(re *regexp.Regexp, text string) string {
	// Known false positives: CSS color codes and font-related numbers in OpenAI HTML emails
	falsePositives := map[string]bool{
		"202123": true, "177010": true, "353740": true,
		"140626": true, "333333": true, "216706": true,
	}

	allMatches := re.FindAllStringSubmatch(text, -1)
	for _, m := range allMatches {
		if len(m) > 1 {
			code := m[1]
			if !falsePositives[code] {
				return code
			}
		}
	}
	return ""
}

// markAsRead marks a message as Seen
func markAsRead(c *client.Client, seqNum uint32) {
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.SeenFlag}
	seq := new(imap.SeqSet)
	seq.AddNum(seqNum)
	c.Store(seq, item, flags, nil)
}
