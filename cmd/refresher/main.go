package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/verssache/chatgpt-creator/internal/config"
	"github.com/verssache/chatgpt-creator/internal/healthcheck"
	"github.com/verssache/chatgpt-creator/internal/ui"
	"github.com/verssache/chatgpt-creator/internal/util"
)

func main() {
	ui.ClearScreen()
	ui.PrintBanner()

	interval := flag.Duration("interval", 30*time.Minute, "Check interval (e.g., 30m, 1h)")
	once := flag.Bool("once", false, "Run once and exit (for cron)")
	dataDir := flag.String("data", "data", "Data directory")
	proxy := flag.String("proxy", "", "Proxy for health checks")
	expiryWarn := flag.Duration("expiry-warn", 24*time.Hour, "Warn when token expires within this duration")
	flag.Parse()

	cfg, _ := config.Load("config.json")
	if cfg != nil && *proxy == "" {
		*proxy = cfg.Proxy
	}

	proxyPool := buildProxyPool(cfg)

	fmt.Println("\n" + ui.C("🔄 TOKEN AUTO-REFRESH DAEMON", ui.Cyan))
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("  Interval     : %s\n", *interval)
	fmt.Printf("  Data dir     : %s\n", *dataDir)
	fmt.Printf("  Proxy        : %s\n", *proxy)
	fmt.Printf("  Expiry warn  : %s\n", *expiryWarn)
	fmt.Printf("  One-shot     : %v\n", *once)
	fmt.Println(strings.Repeat("─", 50))

	daemonCfg := healthcheck.DefaultDaemonConfig()
	daemonCfg.DataDir = *dataDir
	daemonCfg.Proxy = *proxy
	daemonCfg.ProxyPool = proxyPool
	daemonCfg.Interval = *interval
	daemonCfg.ExpiryWarn = *expiryWarn
	daemonCfg.Once = *once

	healthcheck.RunDaemon(daemonCfg)
}

func buildProxyPool(cfg *config.Config) *util.ProxyPool {
	if cfg == nil {
		return nil
	}
	var proxies []string
	if len(cfg.ProxyList) > 0 {
		proxies = cfg.ProxyList
	} else if cfg.Proxy != "" {
		proxies = []string{cfg.Proxy}
	}
	if len(proxies) == 0 {
		return nil
	}
	return util.NewProxyPool(proxies)
}
