package register

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/verssache/chatgpt-creator/internal/config"
	"github.com/verssache/chatgpt-creator/internal/email"
	"github.com/verssache/chatgpt-creator/internal/ui"
	"github.com/verssache/chatgpt-creator/internal/util"
	"github.com/verssache/chatgpt-creator/internal/webhook"
)

// BatchConfig holds all batch registration settings.
type BatchConfig struct {
	TotalAccounts   int
	OutputFile      string
	MaxWorkers      int
	Proxy           string
	ProxyPool       *util.ProxyPool
	DefaultPassword string
	DefaultDomain   string
	K12WorkspaceIDs []string
	WebhookURL      string
	RateLimiter     *RateLimiter

	// Gmail mode
	GmailMode     bool
	GmailPool     *email.GmailDotPool
	GmailAccounts []config.GmailAccount
}

// Global mutex for file writing to prevent corruption
var fileMutex sync.Mutex

func getBaseEmail(emailAddr string) string {
	parts := strings.Split(emailAddr, "@")
	if len(parts) != 2 {
		return emailAddr
	}
	return strings.ReplaceAll(parts[0], ".", "") + "@" + parts[1]
}

// registerOne handles a single account registration.
func registerOne(workerID int, tag string, cfg *BatchConfig, registeredEmails map[string]bool, printMu *sync.Mutex) (bool, string, string, *TokenResult) {
	proxy := cfg.Proxy
	if cfg.ProxyPool != nil {
		proxy = cfg.ProxyPool.Next()
	}

	client, err := NewClient(proxy, tag, workerID, printMu, &fileMutex)
	if err != nil {
		if cfg.ProxyPool != nil && proxy != "" {
			cfg.ProxyPool.MarkFailure(proxy)
		}
		return false, "", fmt.Sprintf("failed to create client: %v", err), nil
	}

	// Attach rate limiter to client
	if cfg.RateLimiter != nil {
		client.rateLimiter = cfg.RateLimiter
	}

	var emailAddr string
	var baseEmail string
	var appPassword string

	if cfg.GmailMode && cfg.GmailPool != nil {
		// Gmail dot-trick mode: get next email from pool, skipping already registered ones
		for {
			emailAddr, err = cfg.GmailPool.Next()
			if err != nil {
				return false, "", fmt.Sprintf("no more Gmail addresses: %v", err), nil
			}
			if !registeredEmails[emailAddr] {
				break
			}
		}

		baseEmail = getBaseEmail(emailAddr)
		for _, acc := range cfg.GmailAccounts {
			if strings.EqualFold(acc.BaseEmail, baseEmail) {
				appPassword = acc.AppPassword
				break
			}
		}
	} else {
		// Original temp email mode
		emailAddr, err = email.CreateTempEmail(cfg.DefaultDomain)
		if err != nil {
			return false, "", fmt.Sprintf("failed to create temp email: %v", err), nil
		}
	}

	password := cfg.DefaultPassword
	if password == "" {
		password = util.GeneratePassword(14)
	}

	firstName, lastName := util.RandomName()
	birthdate := util.RandomBirthdate()

	printMu.Lock()
	if cfg.GmailMode {
		fmt.Printf("[%s] [W%d] 📧 %s | Starting registration for %s\n", time.Now().Format("15:04:05"), workerID, strings.Split(baseEmail, "@")[0], emailAddr)
	} else {
		fmt.Printf("[%s] [W%d] Starting registration for %s\n", time.Now().Format("15:04:05"), workerID, emailAddr)
	}
	printMu.Unlock()

	// Pass Gmail IMAP config for OTP reading if in Gmail mode
	var gmailIMAP *email.GmailIMAPConfig
	if cfg.GmailMode && appPassword != "" {
		gmailIMAP = &email.GmailIMAPConfig{
			Email:       baseEmail,
			AppPassword: appPassword,
		}
	}

	tokenResult, err := client.RunRegister(emailAddr, password, firstName+" "+lastName, birthdate, cfg.K12WorkspaceIDs, gmailIMAP)

	if tokenResult != nil {
		tokenResult.Password = password
	}

	if err != nil {
		// Mark proxy failure for registration errors
		if cfg.ProxyPool != nil && proxy != "" {
			cfg.ProxyPool.MarkFailure(proxy)
		}

		// Handle Zombie auto-purge & Rescue
		if cfg.GmailMode && (strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "profile") || strings.Contains(err.Error(), "log-in/password")) {
			printMu.Lock()
			fmt.Printf("[%s] 🔄 Zombie detected! Switching to Login Mode for %s...\n", time.Now().Format("15:04:05"), emailAddr)
			printMu.Unlock()

			tokenResult, err = client.RunLogin(emailAddr, password, cfg.K12WorkspaceIDs, gmailIMAP)
			if tokenResult != nil {
				tokenResult.Password = password
			}

			if err != nil {
				cfg.GmailPool.MarkConsumed(emailAddr) // Shrink list
				return false, emailAddr, "Zombie Login Failed: " + err.Error(), nil
			}

			// Zombie rescue succeeded, mark proxy success
			if cfg.ProxyPool != nil && proxy != "" {
				cfg.ProxyPool.MarkSuccess(proxy)
			}
		} else {
			return false, emailAddr, err.Error(), nil
		}
	}

	// Mark proxy success if using pool
	if cfg.ProxyPool != nil && proxy != "" {
		cfg.ProxyPool.MarkSuccess(proxy)
	}

	// Success! Mark consumed so it's removed from list
	if cfg.GmailMode {
		cfg.GmailPool.MarkConsumed(emailAddr)
	}

	if tokenResult != nil {
		saveTokensPerBase(emailAddr, tokenResult, cfg.GmailMode)
	}

	// Append to generic text output file
	fileMutex.Lock()
	f, fileErr := os.OpenFile(cfg.OutputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if fileErr == nil {
		var line string
		if tokenResult != nil {
			line = fmt.Sprintf("%s|%s|%s\n", emailAddr, password, tokenResult.AccessToken)
		} else {
			line = fmt.Sprintf("%s|%s\n", emailAddr, password)
		}
		f.WriteString(line)
		f.Close()
	}
	fileMutex.Unlock()

	return true, emailAddr, "", tokenResult
}

// RunBatch runs concurrent registration tasks with retry until target success count is reached.
func RunBatch(cfg *BatchConfig) {
	var printMu sync.Mutex

	var remaining int64 = int64(cfg.TotalAccounts)
	var successCount int64
	var failureCount int64
	var attemptNum int64

	registeredEmails := make(map[string]bool)

	// Initialize shared rate limiter if not provided
	if cfg.RateLimiter == nil {
		cfg.RateLimiter = NewRateLimiter()
	}

	// In Gmail mode, read all existing account.json files to skip registered emails
	if cfg.GmailMode {
		for _, acc := range cfg.GmailAccounts {
			username := strings.Split(acc.BaseEmail, "@")[0]
			tokenFile := filepath.Join("data", fmt.Sprintf("accounts_%s.json", username))
			existingData, err := os.ReadFile(tokenFile)
			if err == nil && len(existingData) > 0 {
				var tokens []*TokenResult
				if json.Unmarshal(existingData, &tokens) == nil {
					for _, t := range tokens {
						registeredEmails[t.Email] = true
					}
				}
			}
		}
	}

	startTime := time.Now()

	// Graceful Exit Handler
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println(ui.C("\n[!] Menerima sinyal berhenti (Ctrl+C). Menunggu worker yang sedang jalan selesai (Graceful Exit)...", ui.Yellow))
		atomic.StoreInt64(&remaining, 0) // Stop accepting new tasks
	}()

	// Session check and save
	type SessionData struct {
		TotalAccounts int64 `json:"totalAccounts"`
		MaxWorkers    int   `json:"maxWorkers"`
		SuccessCount  int64 `json:"successCount"`
		FailCount     int64 `json:"failCount"`
		Remaining     int64 `json:"remaining"`
	}

	// Goroutine to periodically save state
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for {
			select {
			case <-sigCh:
				return // stop saving if graceful exit triggered
			case <-ticker.C:
				rem := atomic.LoadInt64(&remaining)
				if rem <= 0 {
					os.Remove(filepath.Join("data", "session.json"))
					return
				}

				sess := SessionData{
					TotalAccounts: int64(cfg.TotalAccounts),
					MaxWorkers:    cfg.MaxWorkers,
					SuccessCount:  atomic.LoadInt64(&successCount),
					FailCount:     atomic.LoadInt64(&failureCount),
					Remaining:     rem,
				}

				data, _ := json.MarshalIndent(sess, "", "  ")
				os.WriteFile(filepath.Join("data", "session.json"), data, 0644)
			}
		}
	}()

	if cfg.GmailMode && cfg.GmailPool != nil {
		fmt.Printf("📧 Multi-Gmail Mode: %d email addresses available in pool\n\n", cfg.GmailPool.Remaining())
	}

	if cfg.ProxyPool != nil && cfg.ProxyPool.Size() > 0 {
		fmt.Printf("🌐 Proxy Pool: %d proxies loaded (%d healthy)\n\n", cfg.ProxyPool.Size(), cfg.ProxyPool.HealthyCount())
	}

	var wg sync.WaitGroup

	for w := 1; w <= cfg.MaxWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				// Claim a slot before starting work
				if atomic.AddInt64(&remaining, -1) < 0 {
					atomic.AddInt64(&remaining, 1)
					return
				}

				attempt := atomic.AddInt64(&attemptNum, 1)
				tag := fmt.Sprintf("%d/%d", attempt, cfg.TotalAccounts)

				success, emailAddr, errStr, tokenResult := registerOne(workerID, tag, cfg, registeredEmails, &printMu)
				if success {
					atomic.AddInt64(&successCount, 1)
					ts := time.Now().Format("15:04:05")
					
					printMu.Lock()
					if cfg.GmailMode {
						base := getBaseEmail(emailAddr)
						fmt.Printf("[%s] [W%d] 📧 %s | %s\n", ts, workerID, ui.C(strings.Split(base, "@")[0], ui.Cyan), ui.C("✓ SUCCESS: "+emailAddr, ui.Green))
					} else {
						fmt.Printf("[%s] [W%d] %s\n", ts, workerID, ui.C("✓ SUCCESS: "+emailAddr, ui.Green))
					}
					printMu.Unlock()

					// Discord webhook notification
					if cfg.WebhookURL != "" && tokenResult != nil {
						planType := "free"
						if len(tokenResult.WorkspaceTokens) > 0 {
							planType = "k12"
						}
						wsID := ""
						for id := range tokenResult.WorkspaceTokens {
							wsID = id
							break
						}
						go webhook.SendAccountCreated(cfg.WebhookURL, emailAddr, planType, wsID)
					}

					// Collect token result and save to specific JSON
					if tokenResult != nil {
						saveTokensPerBase(emailAddr, tokenResult, cfg.GmailMode)

						printMu.Lock()
						fmt.Printf("[%s] [W%d] %s\n", ts, workerID, ui.C("🔑 Token saved for "+emailAddr, ui.Yellow))
						printMu.Unlock()
					}
				} else {
					atomic.AddInt64(&failureCount, 1)

					// Discord webhook for failure
					if cfg.WebhookURL != "" {
						go webhook.SendAccountFailed(cfg.WebhookURL, emailAddr, errStr)
					}

					// In Gmail mode, don't retry with same email (it's consumed or failed)
					if !cfg.GmailMode {
						atomic.AddInt64(&remaining, 1)
					} else {
						if cfg.GmailPool != nil && cfg.GmailPool.Remaining() > 0 {
							atomic.AddInt64(&remaining, 1) // Retry with next email
						}
					}

					ts := time.Now().Format("15:04:05")

					if !cfg.GmailMode && strings.Contains(errStr, "unsupported_email") {
						parts := strings.Split(emailAddr, "@")
						if len(parts) == 2 {
							domain := parts[1]
							email.AddBlacklistDomain(domain)
							printMu.Lock()
							fmt.Printf("[%s] [W%d] %s\n", ts, workerID, ui.C("⚠ Blacklisted domain: "+domain, ui.Yellow))
							printMu.Unlock()
						}
					}

					printMu.Lock()
					if cfg.GmailMode {
						base := getBaseEmail(emailAddr)
						fmt.Printf("[%s] [W%d] 📧 %s | %s | %s\n", ts, workerID, ui.C(strings.Split(base, "@")[0], ui.Cyan), ui.C("✗ FAILURE: "+emailAddr, ui.Red), errStr)
					} else {
						fmt.Printf("[%s] [W%d] %s | %s\n", ts, workerID, ui.C("✗ FAILURE: "+emailAddr, ui.Red), errStr)
					}
					printMu.Unlock()
				}
			}
		}(w)
	}

	wg.Wait()

	elapsed := time.Since(startTime)
	elapsedStr := formatDuration(elapsed)

	fmt.Printf(ui.C("\n--- Batch Registration Summary ---\n", ui.Cyan))
	fmt.Printf("Target:    %d\n", cfg.TotalAccounts)
	fmt.Printf("Success:   %s\n", ui.C(fmt.Sprintf("%d", successCount), ui.Green))
	fmt.Printf("Attempts:  %d\n", attemptNum)
	fmt.Printf("Failures:  %s\n", ui.C(fmt.Sprintf("%d", failureCount), ui.Red))
	fmt.Printf("Elapsed:   %s\n", elapsedStr)
	fmt.Println(ui.C("──────────────────────────────────", ui.Cyan))
	fmt.Printf("Results:   %s\n", cfg.OutputFile)
	if cfg.GmailMode {
		fmt.Printf("Tokens:    data/accounts_*.json\n")
	} else {
		fmt.Printf("Tokens:    accounts.json\n")
	}
	if cfg.WebhookURL != "" {
		go webhook.SendSummary(cfg.WebhookURL, int(successCount), int(failureCount), cfg.TotalAccounts, elapsedStr)
	}
	fmt.Println(ui.C("──────────────────────────────────", ui.Cyan))
}

// saveTokensPerBase saves a token result to a specific JSON file based on the email.
func saveTokensPerBase(emailAddr string, token *TokenResult, isGmail bool) {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	var tokenFile string
	if isGmail {
		baseEmail := getBaseEmail(emailAddr)
		username := strings.Split(baseEmail, "@")[0]
		tokenFile = filepath.Join("data", fmt.Sprintf("accounts_%s.json", username))
	} else {
		tokenFile = "accounts.json"
	}

	var tokens []*TokenResult
	existingData, err := os.ReadFile(tokenFile)
	if err == nil && len(existingData) > 0 {
		_ = json.Unmarshal(existingData, &tokens)
	}

	tokens = append(tokens, token)
	data, err := json.MarshalIndent(tokens, "", "  ")
	if err == nil {
		_ = os.WriteFile(tokenFile, data, 0644)
	}
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
