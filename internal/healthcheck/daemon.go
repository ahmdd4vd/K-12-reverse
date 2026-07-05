package healthcheck

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/verssache/chatgpt-creator/internal/register"
	"github.com/verssache/chatgpt-creator/internal/util"
)

// DaemonConfig holds configuration for the auto-refresh daemon.
type DaemonConfig struct {
	DataDir     string
	Proxy       string
	ProxyPool   *util.ProxyPool
	Interval    time.Duration
	ExpiryWarn  time.Duration // warn when token expires within this duration (default: 24h)
	Once        bool          // run once and exit (for cron)
	Webhook     func(string) // optional webhook callback for notifications
}

// DefaultDaemonConfig returns a default daemon configuration.
func DefaultDaemonConfig() DaemonConfig {
	return DaemonConfig{
		DataDir:    "data",
		Interval:   30 * time.Minute,
		ExpiryWarn: 24 * time.Hour,
		Once:       false,
	}
}

// RunDaemon starts the auto-refresh daemon loop.
func RunDaemon(cfg DaemonConfig) {
	fmt.Printf("🔄 Auto-Refresh Daemon started\n")
	fmt.Printf("   Interval    : %s\n", cfg.Interval)
	fmt.Printf("   Expiry Warn : %s\n", cfg.ExpiryWarn)
	fmt.Printf("   Data dir    : %s\n", cfg.DataDir)
	if cfg.Once {
		fmt.Println("   Mode        : One-shot")
	} else {
		fmt.Println("   Mode        : Continuous (Ctrl+C to stop)")
	}
	fmt.Println()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	runOnce := func(iteration int) {
		if iteration > 0 {
			fmt.Printf("\n--- Check #%d at %s ---\n", iteration, time.Now().Format("15:04:05"))
		} else {
			fmt.Printf("\n--- Initial check at %s ---\n", time.Now().Format("15:04:05"))
		}

		accounts, err := LoadAccounts(cfg.DataDir)
		if err != nil || len(accounts) == 0 {
			fmt.Printf("⚠ No accounts found, skipping check\n")
			return
		}

		proxy := cfg.Proxy
		if cfg.ProxyPool != nil {
			proxy = cfg.ProxyPool.Next()
		}

		checker, err := NewChecker(proxy)
		if err != nil {
			fmt.Printf("⚠ Checker init failed: %v\n", err)
			return
		}

		var mu sync.Mutex
		var wg sync.WaitGroup
		results := make([]*CheckResult, len(accounts))
		workerCh := make(chan int, 5)

		for i, token := range accounts {
			wg.Add(1)
			workerCh <- 1

			go func(idx int, tk *register.TokenResult) {
				defer wg.Done()
				defer func() { <-workerCh }()

				result := checker.CheckToken(tk)

				mu.Lock()
				results[idx] = result
				mu.Unlock()

				switch result.Status {
				case StatusValid:
					fmt.Printf("  ✅ %s — valid\n", tk.Email)
				case StatusExpired:
					fmt.Printf("  ⚠️ %s — expired\n", tk.Email)
				case StatusRefreshed:
					fmt.Printf("  🔄 %s — refreshed\n", tk.Email)
					if cfg.Webhook != nil {
						cfg.Webhook(fmt.Sprintf("🔄 Token refreshed: %s", tk.Email))
					}
				case StatusError:
					fmt.Printf("  ❌ %s — error: %s\n", tk.Email, result.Error)
				}
			}(i, token)
		}
		wg.Wait()

		// Save refreshed tokens
		expiringSoon := 0
		for _, r := range results {
			if r.Status == StatusExpired {
				expiringSoon++
			}
		}

		if err := SaveRefreshedTokens(cfg.DataDir, results); err != nil {
			fmt.Printf("⚠ Failed to save refreshed tokens: %v\n", err)
		} else {
			refreshed := 0
			for _, r := range results {
				if r.Status == StatusRefreshed {
					refreshed++
				}
			}
			if refreshed > 0 {
				fmt.Printf("✅ Refreshed %d tokens and saved\n", refreshed)
			}
		}

		// Webhook notification for expiring tokens
		if cfg.Webhook != nil && expiringSoon > 0 {
			cfg.Webhook(fmt.Sprintf("⚠️ %d token(s) expired and could not be refreshed", expiringSoon))
		}

		fmt.Printf("--- Check complete: %d accounts ---\n", len(accounts))
	}

	// Run first check immediately
	runOnce(0)

	if cfg.Once {
		return
	}

	// Continuous loop
	iteration := 1
	for {
		select {
		case <-ticker.C:
			runOnce(iteration)
			iteration++
		case <-sigCh:
			fmt.Println("\n🛑 Daemon stopped by user")
			return
		}
	}
}
